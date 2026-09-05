/*
Unit tests for the storage-backed deployment resolver and exporter option
selection.
*/
package otel

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/iamrajiv/cdevents-otel-bridge/internal/storage"
	"github.com/iamrajiv/cdevents-otel-bridge/pkg/models"
	"github.com/iamrajiv/cdevents-otel-bridge/pkg/otelbridge"
)

func TestStorageResolver(t *testing.T) {
	store := storage.NewMemoryStorage()
	ctx := context.Background()
	base := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)

	_ = store.SaveDeployment(ctx, &models.Deployment{Service: "svc", Environment: "staging", Version: "v2", DeployedAt: base.Add(time.Hour)})
	_ = store.SaveDeployment(ctx, &models.Deployment{Service: "svc", Environment: "production", Version: "v1", DeployedAt: base})

	resolver := NewStorageResolver(store)

	d, err := resolver.ResolveDeployment(ctx, "svc")
	if err != nil {
		t.Fatalf("ResolveDeployment failed: %v", err)
	}
	if d.Version != "v2" {
		t.Errorf("Version = %q, want v2 (latest across environments)", d.Version)
	}

	_, err = resolver.ResolveDeployment(ctx, "missing")
	if !errors.Is(err, otelbridge.ErrNotFound) {
		t.Errorf("expected otelbridge.ErrNotFound, got %v", err)
	}

	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := resolver.ResolveDeployment(canceled, "svc"); err == nil || errors.Is(err, otelbridge.ErrNotFound) {
		t.Errorf("expected propagated storage error, got %v", err)
	}
}

func TestExporterOptions(t *testing.T) {
	if opts := exporterOptions(Config{Endpoint: "jaeger:4317", Insecure: true}); len(opts) != 2 {
		t.Errorf("host:port with insecure should give 2 options, got %d", len(opts))
	}
	if opts := exporterOptions(Config{Endpoint: "jaeger:4317", Insecure: false}); len(opts) != 1 {
		t.Errorf("host:port with TLS should give 1 option, got %d", len(opts))
	}
	if opts := exporterOptions(Config{Endpoint: "http://jaeger:4317", Insecure: false}); len(opts) != 1 {
		t.Errorf("URL endpoint should give 1 option, got %d", len(opts))
	}
}

func TestNewTracerProviderCreatesProvider(t *testing.T) {
	tp, err := NewTracerProvider(context.Background(), Config{
		Endpoint: "127.0.0.1:1", Insecure: true, ServiceName: "test", ServiceVersion: "v0",
	}, nil)
	if err != nil {
		t.Fatalf("NewTracerProvider failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = tp.Shutdown(ctx)
}
