/*
Package storage provides the Redis implementation of the Storage interface.

RedisStorage serializes records as JSON strings and applies a configurable
TTL to everything it writes, so old deployments and events expire on their
own. Key layout:

  - deployment:{service}:{environment}  JSON models.Deployment
  - event:{eventId}                     JSON cdevents.StoredEvent
  - chain:{chainId}                     SET of event ids in that chain

Listing deployments and finding the latest deployment for a service use SCAN
with a MATCH pattern rather than a separate index; the number of tracked
deployments is expected to be small (one per service and environment), so
this stays cheap and avoids stale index entries when keys expire.
*/
package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/iamrajiv/cdevents-otel-bridge/internal/cdevents"
	"github.com/iamrajiv/cdevents-otel-bridge/pkg/models"
)

const (
	deploymentKeyPrefix = "deployment:"
	eventKeyPrefix      = "event:"
	chainKeyPrefix      = "chain:"
	scanBatchSize       = 100
	connectTimeout      = 5 * time.Second
)

type RedisStorage struct {
	client *redis.Client
	ttl    time.Duration
}

func NewRedisStorage(url string, ttl time.Duration) (*RedisStorage, error) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("failed to parse redis URL: %w", err)
	}

	client := redis.NewClient(opts)

	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("failed to ping redis: %w", err)
	}

	return &RedisStorage{client: client, ttl: ttl}, nil
}

func redisDeploymentKey(service, environment string) string {
	return deploymentKeyPrefix + escapeKeyPart(service) + ":" + escapeKeyPart(environment)
}

func escapeKeyPart(s string) string {
	return strings.NewReplacer("*", "%2A", "?", "%3F", "[", "%5B", "]", "%5D", "\\", "%5C").Replace(s)
}

func (r *RedisStorage) SaveDeployment(ctx context.Context, d *models.Deployment) error {
	if d == nil {
		return errors.New("deployment is nil")
	}

	data, err := json.Marshal(d)
	if err != nil {
		return fmt.Errorf("failed to marshal deployment: %w", err)
	}

	if err := r.client.Set(ctx, redisDeploymentKey(d.Service, d.Environment), data, r.ttl).Err(); err != nil {
		return fmt.Errorf("failed to save deployment: %w", err)
	}
	return nil
}

func (r *RedisStorage) GetDeployment(ctx context.Context, service, environment string) (*models.Deployment, error) {
	return r.readDeployment(ctx, redisDeploymentKey(service, environment))
}

func (r *RedisStorage) readDeployment(ctx context.Context, key string) (*models.Deployment, error) {
	data, err := r.client.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment: %w", err)
	}

	var d models.Deployment
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, fmt.Errorf("failed to unmarshal deployment: %w", err)
	}
	return &d, nil
}

func (r *RedisStorage) GetLatestDeployment(ctx context.Context, service string) (*models.Deployment, error) {
	deployments, err := r.scanDeployments(ctx, deploymentKeyPrefix+escapeKeyPart(service)+":*")
	if err != nil {
		return nil, err
	}

	candidates := deployments[:0]
	for _, d := range deployments {
		if d.Service == service {
			candidates = append(candidates, d)
		}
	}
	return latestDeployment(candidates)
}

func (r *RedisStorage) ListDeployments(ctx context.Context, filter ListFilter) ([]*models.Deployment, error) {
	deployments, err := r.scanDeployments(ctx, deploymentKeyPrefix+"*")
	if err != nil {
		return nil, err
	}
	return applyFilter(deployments, filter), nil
}

func (r *RedisStorage) scanDeployments(ctx context.Context, pattern string) ([]*models.Deployment, error) {
	var (
		cursor      uint64
		deployments []*models.Deployment
	)

	for {
		keys, next, err := r.client.Scan(ctx, cursor, pattern, scanBatchSize).Result()
		if err != nil {
			return nil, fmt.Errorf("failed to scan deployments: %w", err)
		}

		for _, key := range keys {
			d, err := r.readDeployment(ctx, key)
			if errors.Is(err, ErrNotFound) {
				continue
			}
			if err != nil {
				return nil, err
			}
			deployments = append(deployments, d)
		}

		if next == 0 {
			break
		}
		cursor = next
	}

	return deployments, nil
}

func (r *RedisStorage) SaveEvent(ctx context.Context, event *cdevents.StoredEvent) error {
	if event == nil {
		return errors.New("event is nil")
	}

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	pipe := r.client.TxPipeline()
	pipe.Set(ctx, eventKeyPrefix+event.ID, data, r.ttl)
	if event.ChainID != "" {
		chainKey := chainKeyPrefix + event.ChainID
		pipe.SAdd(ctx, chainKey, event.ID)
		if r.ttl > 0 {
			pipe.Expire(ctx, chainKey, r.ttl)
		}
	}

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to save event: %w", err)
	}
	return nil
}

func (r *RedisStorage) GetEvent(ctx context.Context, eventID string) (*cdevents.StoredEvent, error) {
	data, err := r.client.Get(ctx, eventKeyPrefix+eventID).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get event: %w", err)
	}

	var event cdevents.StoredEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return nil, fmt.Errorf("failed to unmarshal event: %w", err)
	}
	return &event, nil
}

func (r *RedisStorage) GetEventsByChain(ctx context.Context, chainID string) ([]*cdevents.StoredEvent, error) {
	if chainID == "" {
		return []*cdevents.StoredEvent{}, nil
	}

	eventIDs, err := r.client.SMembers(ctx, chainKeyPrefix+chainID).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get chain members: %w", err)
	}

	events := make([]*cdevents.StoredEvent, 0, len(eventIDs))
	for _, eventID := range eventIDs {
		event, err := r.GetEvent(ctx, eventID)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}

	sortEventsNewestFirst(events)
	return events, nil
}

func (r *RedisStorage) Ping(ctx context.Context) error {
	if err := r.client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis ping failed: %w", err)
	}
	return nil
}

func (r *RedisStorage) Close() error {
	return r.client.Close()
}
