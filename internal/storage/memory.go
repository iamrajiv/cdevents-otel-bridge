/*
Package storage provides the in-memory implementation of the Storage
interface.

MemoryStorage keeps deployments and events in plain maps guarded by a
read/write mutex. It is suitable for development, tests and single-instance
deployments where losing data on restart is acceptable. Nothing is ever
evicted, so long-running instances that receive many events should use the
Redis backend instead.
*/
package storage

import (
	"context"
	"errors"
	"sync"

	"github.com/iamrajiv/cdevents-otel-bridge/internal/cdevents"
	"github.com/iamrajiv/cdevents-otel-bridge/pkg/models"
)

type MemoryStorage struct {
	mu          sync.RWMutex
	deployments map[string]*models.Deployment
	events      map[string]*cdevents.StoredEvent
}

func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{
		deployments: make(map[string]*models.Deployment),
		events:      make(map[string]*cdevents.StoredEvent),
	}
}

func deploymentKey(service, environment string) string {
	return service + "\x00" + environment
}

func (m *MemoryStorage) SaveDeployment(ctx context.Context, d *models.Deployment) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if d == nil {
		return errors.New("deployment is nil")
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.deployments[deploymentKey(d.Service, d.Environment)] = d
	return nil
}

func (m *MemoryStorage) GetDeployment(ctx context.Context, service, environment string) (*models.Deployment, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	d, ok := m.deployments[deploymentKey(service, environment)]
	if !ok {
		return nil, ErrNotFound
	}
	return d, nil
}

func (m *MemoryStorage) GetLatestDeployment(ctx context.Context, service string) (*models.Deployment, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	var candidates []*models.Deployment
	for _, d := range m.deployments {
		if d.Service == service {
			candidates = append(candidates, d)
		}
	}
	return latestDeployment(candidates)
}

func (m *MemoryStorage) ListDeployments(ctx context.Context, filter ListFilter) ([]*models.Deployment, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	m.mu.RLock()
	all := make([]*models.Deployment, 0, len(m.deployments))
	for _, d := range m.deployments {
		all = append(all, d)
	}
	m.mu.RUnlock()

	return applyFilter(all, filter), nil
}

func (m *MemoryStorage) SaveEvent(ctx context.Context, event *cdevents.StoredEvent) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if event == nil {
		return errors.New("event is nil")
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.events[event.ID] = event
	return nil
}

func (m *MemoryStorage) GetEvent(ctx context.Context, eventID string) (*cdevents.StoredEvent, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	event, ok := m.events[eventID]
	if !ok {
		return nil, ErrNotFound
	}
	return event, nil
}

func (m *MemoryStorage) GetEventsByChain(ctx context.Context, chainID string) ([]*cdevents.StoredEvent, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if chainID == "" {
		return []*cdevents.StoredEvent{}, nil
	}

	m.mu.RLock()
	events := make([]*cdevents.StoredEvent, 0)
	for _, event := range m.events {
		if event.ChainID == chainID {
			events = append(events, event)
		}
	}
	m.mu.RUnlock()

	sortEventsNewestFirst(events)
	return events, nil
}

func (m *MemoryStorage) Ping(ctx context.Context) error {
	return ctx.Err()
}

func (m *MemoryStorage) Close() error {
	return nil
}
