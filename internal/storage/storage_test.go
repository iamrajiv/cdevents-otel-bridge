/*
Shared conformance tests for Storage implementations.

Every backend must behave identically for the operations the bridge relies
on, so the same suite runs against MemoryStorage and RedisStorage (backed by
miniredis). Backend-specific behavior such as TTL expiry lives in the
backend's own test file.
*/
package storage

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/iamrajiv/cdevents-otel-bridge/internal/cdevents"
	"github.com/iamrajiv/cdevents-otel-bridge/pkg/models"
)

type storageFactory func(t *testing.T) Storage

func runStorageSuite(t *testing.T, newStorage storageFactory) {
	t.Helper()

	t.Run("save and get deployment", func(t *testing.T) {
		s := newStorage(t)
		ctx := context.Background()

		d := &models.Deployment{
			Service: "api-service", Environment: "production", Version: "v1.2.3",
			DeployedAt: time.Now().UTC().Truncate(time.Second), EventID: "event-123",
		}
		if err := s.SaveDeployment(ctx, d); err != nil {
			t.Fatalf("SaveDeployment failed: %v", err)
		}

		got, err := s.GetDeployment(ctx, "api-service", "production")
		if err != nil {
			t.Fatalf("GetDeployment failed: %v", err)
		}
		if got.Service != d.Service || got.Environment != d.Environment || got.Version != d.Version || got.EventID != d.EventID {
			t.Errorf("GetDeployment returned %+v, want %+v", got, d)
		}
	})

	t.Run("save deployment replaces previous for same service and environment", func(t *testing.T) {
		s := newStorage(t)
		ctx := context.Background()
		now := time.Now().UTC().Truncate(time.Second)

		for _, version := range []string{"v1", "v2"} {
			if err := s.SaveDeployment(ctx, &models.Deployment{Service: "svc", Environment: "prod", Version: version, DeployedAt: now}); err != nil {
				t.Fatal(err)
			}
		}

		got, err := s.GetDeployment(ctx, "svc", "prod")
		if err != nil {
			t.Fatal(err)
		}
		if got.Version != "v2" {
			t.Errorf("Version = %q, want v2", got.Version)
		}

		all, err := s.ListDeployments(ctx, ListFilter{})
		if err != nil {
			t.Fatal(err)
		}
		if len(all) != 1 {
			t.Errorf("expected 1 deployment, got %d", len(all))
		}
	})

	t.Run("get deployment not found", func(t *testing.T) {
		s := newStorage(t)
		_, err := s.GetDeployment(context.Background(), "nonexistent", "production")
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("nil deployment and event are rejected", func(t *testing.T) {
		s := newStorage(t)
		if err := s.SaveDeployment(context.Background(), nil); err == nil {
			t.Error("SaveDeployment(nil) should fail")
		}
		if err := s.SaveEvent(context.Background(), nil); err == nil {
			t.Error("SaveEvent(nil) should fail")
		}
	})

	t.Run("get latest deployment across environments", func(t *testing.T) {
		s := newStorage(t)
		ctx := context.Background()
		base := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)

		deployments := []*models.Deployment{
			{Service: "svc", Environment: "staging", Version: "v3", DeployedAt: base.Add(2 * time.Hour)},
			{Service: "svc", Environment: "production", Version: "v2", DeployedAt: base.Add(time.Hour)},
			{Service: "svc", Environment: "dev", Version: "v1", DeployedAt: base},
			{Service: "other", Environment: "production", Version: "v9", DeployedAt: base.Add(5 * time.Hour)},
		}
		for _, d := range deployments {
			if err := s.SaveDeployment(ctx, d); err != nil {
				t.Fatal(err)
			}
		}

		got, err := s.GetLatestDeployment(ctx, "svc")
		if err != nil {
			t.Fatalf("GetLatestDeployment failed: %v", err)
		}
		if got.Version != "v3" || got.Environment != "staging" {
			t.Errorf("GetLatestDeployment = %s/%s, want v3/staging", got.Version, got.Environment)
		}

		if _, err := s.GetLatestDeployment(ctx, "missing"); !errors.Is(err, ErrNotFound) {
			t.Errorf("expected ErrNotFound for unknown service, got %v", err)
		}
	})

	t.Run("get latest deployment does not match service prefixes", func(t *testing.T) {
		s := newStorage(t)
		ctx := context.Background()
		if err := s.SaveDeployment(ctx, &models.Deployment{Service: "svc-extended", Environment: "prod", DeployedAt: time.Now()}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.GetLatestDeployment(ctx, "svc"); !errors.Is(err, ErrNotFound) {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("list deployments filters and orders newest first", func(t *testing.T) {
		s := newStorage(t)
		ctx := context.Background()
		base := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)

		deployments := []*models.Deployment{
			{Service: "service-a", Environment: "production", Version: "v1.0.0", DeployedAt: base, EventID: "event-1"},
			{Service: "service-b", Environment: "production", Version: "v2.0.0", DeployedAt: base.Add(2 * time.Hour), EventID: "event-2"},
			{Service: "service-c", Environment: "staging", Version: "v3.0.0", DeployedAt: base.Add(time.Hour), EventID: "event-3"},
		}
		for _, d := range deployments {
			if err := s.SaveDeployment(ctx, d); err != nil {
				t.Fatalf("SaveDeployment failed: %v", err)
			}
		}

		tests := []struct {
			name      string
			filter    ListFilter
			wantOrder []string
		}{
			{name: "all", filter: ListFilter{}, wantOrder: []string{"service-b", "service-c", "service-a"}},
			{name: "production only", filter: ListFilter{Environment: "production"}, wantOrder: []string{"service-b", "service-a"}},
			{name: "staging only", filter: ListFilter{Environment: "staging"}, wantOrder: []string{"service-c"}},
			{name: "with limit", filter: ListFilter{Limit: 2}, wantOrder: []string{"service-b", "service-c"}},
			{name: "since", filter: ListFilter{Since: base.Add(time.Hour)}, wantOrder: []string{"service-b", "service-c"}},
			{name: "since and environment", filter: ListFilter{Since: base.Add(time.Hour), Environment: "production"}, wantOrder: []string{"service-b"}},
			{name: "no match", filter: ListFilter{Environment: "qa"}, wantOrder: []string{}},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result, err := s.ListDeployments(ctx, tt.filter)
				if err != nil {
					t.Fatalf("ListDeployments failed: %v", err)
				}
				if len(result) != len(tt.wantOrder) {
					t.Fatalf("got %d deployments, want %d", len(result), len(tt.wantOrder))
				}
				for i, want := range tt.wantOrder {
					if result[i].Service != want {
						t.Errorf("position %d: got %s, want %s", i, result[i].Service, want)
					}
				}
			})
		}
	})

	t.Run("save and get event", func(t *testing.T) {
		s := newStorage(t)
		ctx := context.Background()

		event := &cdevents.StoredEvent{
			ID: "event-123", Type: "dev.cdevents.service.deployed.0.1.0", Source: "github.com/myorg/myrepo",
			Timestamp: time.Now().UTC().Truncate(time.Second), SubjectID: "api-service", SubjectType: "service",
			ChainID: "chain-abc", Summary: "Deployed api-service",
			Links:   []models.Link{{LinkType: "triggeredBy", LinkID: "p-1"}},
			RawData: map[string]any{"customData": map[string]any{"commitSha": "abc"}},
		}
		if err := s.SaveEvent(ctx, event); err != nil {
			t.Fatalf("SaveEvent failed: %v", err)
		}

		got, err := s.GetEvent(ctx, "event-123")
		if err != nil {
			t.Fatalf("GetEvent failed: %v", err)
		}
		if got.ID != event.ID || got.Type != event.Type || got.ChainID != event.ChainID || got.Summary != event.Summary {
			t.Errorf("GetEvent returned %+v, want %+v", got, event)
		}
		if len(got.Links) != 1 || got.Links[0] != event.Links[0] {
			t.Errorf("Links = %v, want %v", got.Links, event.Links)
		}
		if cdevents.ExtractChangeInfo(got).Commit != "abc" {
			t.Errorf("RawData did not survive storage: %v", got.RawData)
		}
	})

	t.Run("get event not found", func(t *testing.T) {
		s := newStorage(t)
		_, err := s.GetEvent(context.Background(), "nonexistent-event")
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("get events by chain newest first", func(t *testing.T) {
		s := newStorage(t)
		ctx := context.Background()
		base := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)

		events := []*cdevents.StoredEvent{
			{ID: "event-1", Type: "dev.cdevents.pipelinerun.started.0.1.0", Timestamp: base, ChainID: "chain-abc"},
			{ID: "event-2", Type: "dev.cdevents.pipelinerun.finished.0.1.0", Timestamp: base.Add(time.Minute), ChainID: "chain-abc"},
			{ID: "event-3", Type: "dev.cdevents.service.deployed.0.1.0", Timestamp: base.Add(2 * time.Minute), ChainID: "chain-xyz"},
			{ID: "event-4", Type: "dev.cdevents.service.deployed.0.1.0", Timestamp: base.Add(3 * time.Minute)},
		}
		for _, e := range events {
			if err := s.SaveEvent(ctx, e); err != nil {
				t.Fatalf("SaveEvent failed: %v", err)
			}
		}

		chainEvents, err := s.GetEventsByChain(ctx, "chain-abc")
		if err != nil {
			t.Fatalf("GetEventsByChain failed: %v", err)
		}
		if len(chainEvents) != 2 || chainEvents[0].ID != "event-2" || chainEvents[1].ID != "event-1" {
			t.Errorf("unexpected chain events: %v", ids(chainEvents))
		}

		empty, err := s.GetEventsByChain(ctx, "nonexistent-chain")
		if err != nil {
			t.Fatalf("GetEventsByChain failed: %v", err)
		}
		if len(empty) != 0 {
			t.Errorf("expected 0 events for nonexistent chain, got %d", len(empty))
		}

		none, err := s.GetEventsByChain(ctx, "")
		if err != nil || len(none) != 0 {
			t.Errorf("empty chain id should yield no events, got %v, %v", none, err)
		}
	})

	t.Run("ping and close", func(t *testing.T) {
		s := newStorage(t)
		if err := s.Ping(context.Background()); err != nil {
			t.Errorf("Ping failed: %v", err)
		}
		if err := s.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	})
}

func ids(events []*cdevents.StoredEvent) []string {
	result := make([]string, 0, len(events))
	for _, e := range events {
		result = append(result, e.ID)
	}
	return result
}
