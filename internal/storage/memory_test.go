/*
Unit tests for MemoryStorage.

Runs the shared storage conformance suite and adds memory-specific checks
for context cancellation handling.
*/
package storage

import (
	"context"
	"errors"
	"testing"

	"github.com/iamrajiv/cdevents-otel-bridge/internal/cdevents"
	"github.com/iamrajiv/cdevents-otel-bridge/pkg/models"
)

func TestMemoryStorage(t *testing.T) {
	runStorageSuite(t, func(t *testing.T) Storage {
		t.Helper()
		return NewMemoryStorage()
	})
}

func TestMemoryStorageHonorsCanceledContext(t *testing.T) {
	s := NewMemoryStorage()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := s.SaveDeployment(ctx, &models.Deployment{Service: "svc"}); !errors.Is(err, context.Canceled) {
		t.Errorf("SaveDeployment: expected context.Canceled, got %v", err)
	}
	if _, err := s.GetDeployment(ctx, "svc", "prod"); !errors.Is(err, context.Canceled) {
		t.Errorf("GetDeployment: expected context.Canceled, got %v", err)
	}
	if _, err := s.GetLatestDeployment(ctx, "svc"); !errors.Is(err, context.Canceled) {
		t.Errorf("GetLatestDeployment: expected context.Canceled, got %v", err)
	}
	if _, err := s.ListDeployments(ctx, ListFilter{}); !errors.Is(err, context.Canceled) {
		t.Errorf("ListDeployments: expected context.Canceled, got %v", err)
	}
	if err := s.SaveEvent(ctx, &cdevents.StoredEvent{ID: "e"}); !errors.Is(err, context.Canceled) {
		t.Errorf("SaveEvent: expected context.Canceled, got %v", err)
	}
	if _, err := s.GetEvent(ctx, "e"); !errors.Is(err, context.Canceled) {
		t.Errorf("GetEvent: expected context.Canceled, got %v", err)
	}
	if _, err := s.GetEventsByChain(ctx, "c"); !errors.Is(err, context.Canceled) {
		t.Errorf("GetEventsByChain: expected context.Canceled, got %v", err)
	}
	if err := s.Ping(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("Ping: expected context.Canceled, got %v", err)
	}
}
