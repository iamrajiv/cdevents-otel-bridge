/*
Unit tests for RedisStorage.

Uses miniredis, an in-process Redis server, so the tests need no external
service. Runs the shared conformance suite plus Redis-specific checks for
TTL expiry, key escaping and connection failures.
*/
package storage

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"github.com/iamrajiv/cdevents-otel-bridge/internal/cdevents"
	"github.com/iamrajiv/cdevents-otel-bridge/pkg/models"
)

func newTestRedis(t *testing.T, ttl time.Duration) (*RedisStorage, *miniredis.Miniredis) {
	t.Helper()

	server := miniredis.RunT(t)
	s, err := NewRedisStorage("redis://"+server.Addr(), ttl)
	if err != nil {
		t.Fatalf("NewRedisStorage failed: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, server
}

func TestRedisStorage(t *testing.T) {
	runStorageSuite(t, func(t *testing.T) Storage {
		t.Helper()
		s, _ := newTestRedis(t, time.Hour)
		return s
	})
}

func TestRedisStorageTTL(t *testing.T) {
	s, server := newTestRedis(t, time.Minute)
	ctx := context.Background()

	if err := s.SaveDeployment(ctx, &models.Deployment{Service: "svc", Environment: "prod", DeployedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveEvent(ctx, &cdevents.StoredEvent{ID: "e1", ChainID: "c1", Timestamp: time.Now()}); err != nil {
		t.Fatal(err)
	}

	if ttl := server.TTL("deployment:svc:prod"); ttl != time.Minute {
		t.Errorf("deployment TTL = %v, want 1m", ttl)
	}
	if ttl := server.TTL("event:e1"); ttl != time.Minute {
		t.Errorf("event TTL = %v, want 1m", ttl)
	}
	if ttl := server.TTL("chain:c1"); ttl != time.Minute {
		t.Errorf("chain TTL = %v, want 1m", ttl)
	}

	server.FastForward(2 * time.Minute)

	if _, err := s.GetDeployment(ctx, "svc", "prod"); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected deployment to expire, got %v", err)
	}
	if _, err := s.GetEvent(ctx, "e1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected event to expire, got %v", err)
	}
	events, err := s.GetEventsByChain(ctx, "c1")
	if err != nil || len(events) != 0 {
		t.Errorf("expected empty chain after expiry, got %v, %v", events, err)
	}
}

func TestRedisStorageNoTTL(t *testing.T) {
	s, server := newTestRedis(t, 0)
	ctx := context.Background()

	if err := s.SaveEvent(ctx, &cdevents.StoredEvent{ID: "e1", ChainID: "c1", Timestamp: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if ttl := server.TTL("event:e1"); ttl != 0 {
		t.Errorf("event TTL = %v, want none", ttl)
	}
	if ttl := server.TTL("chain:c1"); ttl != 0 {
		t.Errorf("chain TTL = %v, want none", ttl)
	}
}

func TestRedisStorageEscapesGlobCharacters(t *testing.T) {
	s, _ := newTestRedis(t, time.Hour)
	ctx := context.Background()

	if err := s.SaveDeployment(ctx, &models.Deployment{Service: "svc*", Environment: "prod", Version: "wild", DeployedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveDeployment(ctx, &models.Deployment{Service: "svcX", Environment: "prod", Version: "plain", DeployedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetLatestDeployment(ctx, "svc*")
	if err != nil {
		t.Fatalf("GetLatestDeployment failed: %v", err)
	}
	if got.Version != "wild" {
		t.Errorf("Version = %q, want wild", got.Version)
	}
}

func TestRedisStorageCorruptRecord(t *testing.T) {
	s, server := newTestRedis(t, time.Hour)
	ctx := context.Background()

	server.Set("deployment:svc:prod", "{not json")
	if _, err := s.GetDeployment(ctx, "svc", "prod"); err == nil || errors.Is(err, ErrNotFound) {
		t.Errorf("expected unmarshal error, got %v", err)
	}

	server.Set("event:bad", "{not json")
	if _, err := s.GetEvent(ctx, "bad"); err == nil || errors.Is(err, ErrNotFound) {
		t.Errorf("expected unmarshal error, got %v", err)
	}
}

func TestRedisStorageConnectionFailures(t *testing.T) {
	if _, err := NewRedisStorage("not a url", time.Hour); err == nil {
		t.Error("expected error for invalid URL")
	}
	if _, err := NewRedisStorage("redis://127.0.0.1:1", time.Hour); err == nil {
		t.Error("expected error when redis is unreachable")
	}

	s, server := newTestRedis(t, time.Hour)
	server.Close()

	if err := s.Ping(context.Background()); err == nil {
		t.Error("expected Ping to fail after server closed")
	}
	if _, err := s.ListDeployments(context.Background(), ListFilter{}); err == nil {
		t.Error("expected ListDeployments to fail after server closed")
	}
}
