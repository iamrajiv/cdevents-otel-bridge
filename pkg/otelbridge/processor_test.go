/*
Unit tests for SpanProcessor using a real tracer provider and the SDK's
in-memory exporter, so the attributes are checked on exported spans exactly
as a backend would see them.
*/
package otelbridge

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"

	"github.com/iamrajiv/cdevents-otel-bridge/pkg/models"
)

func newProvider(t *testing.T, serviceName string, processor sdktrace.SpanProcessor) (*sdktrace.TracerProvider, *tracetest.InMemoryExporter) {
	t.Helper()

	exporter := tracetest.NewInMemoryExporter()
	attrs := []attribute.KeyValue{}
	if serviceName != "" {
		attrs = append(attrs, semconv.ServiceName(serviceName))
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithResource(resource.NewWithAttributes(semconv.SchemaURL, attrs...)),
		sdktrace.WithSpanProcessor(processor),
		sdktrace.WithSyncer(exporter),
	)
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	return tp, exporter
}

func exportedAttributes(t *testing.T, exporter *tracetest.InMemoryExporter) map[attribute.Key]string {
	t.Helper()
	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 exported span, got %d", len(spans))
	}
	result := make(map[attribute.Key]string)
	for _, attr := range spans[0].Attributes {
		result[attr.Key] = attr.Value.AsString()
	}
	return result
}

func TestSpanProcessorAddsDeploymentAttributes(t *testing.T) {
	deployedAt := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	resolver := ResolverFunc(func(_ context.Context, service string) (*models.Deployment, error) {
		if service != "checkout" {
			t.Errorf("resolver asked for %q, want checkout", service)
		}
		return &models.Deployment{
			ID: "dep-1", Version: "v1.2.3", CommitSha: "abc123", Environment: "production",
			PipelineID: "build-42", Repository: "github.com/org/checkout", Branch: "main",
			DeployedAt: deployedAt, DeployedBy: "jenkins", EventID: "evt-1",
		}, nil
	})

	tp, exporter := newProvider(t, "checkout", NewSpanProcessor(resolver))
	_, span := tp.Tracer("test").Start(context.Background(), "op")
	span.End()

	attrs := exportedAttributes(t, exporter)
	want := map[attribute.Key]string{
		DeploymentIDKey:          "dep-1",
		DeploymentVersionKey:     "v1.2.3",
		DeploymentCommitKey:      "abc123",
		DeploymentEnvironmentKey: "production",
		DeploymentPipelineIDKey:  "build-42",
		DeploymentRepositoryKey:  "github.com/org/checkout",
		DeploymentBranchKey:      "main",
		DeploymentDeployedAtKey:  "2024-01-15T10:30:00Z",
		DeploymentDeployedByKey:  "jenkins",
		DeploymentEventIDKey:     "evt-1",
	}
	for key, value := range want {
		if attrs[key] != value {
			t.Errorf("%s = %q, want %q", key, attrs[key], value)
		}
	}
}

func TestSpanProcessorServiceNameOverride(t *testing.T) {
	var asked string
	resolver := ResolverFunc(func(_ context.Context, service string) (*models.Deployment, error) {
		asked = service
		return &models.Deployment{Version: "v1"}, nil
	})

	tp, exporter := newProvider(t, "resource-name", NewSpanProcessor(resolver, WithServiceName("override")))
	_, span := tp.Tracer("test").Start(context.Background(), "op")
	span.End()

	if asked != "override" {
		t.Errorf("resolver asked for %q, want override", asked)
	}
	if exportedAttributes(t, exporter)[DeploymentVersionKey] != "v1" {
		t.Error("expected deployment.version attribute")
	}
}

func TestSpanProcessorSkipsWithoutServiceName(t *testing.T) {
	called := false
	resolver := ResolverFunc(func(context.Context, string) (*models.Deployment, error) {
		called = true
		return &models.Deployment{Version: "v1"}, nil
	})

	tp, exporter := newProvider(t, "", NewSpanProcessor(resolver))
	_, span := tp.Tracer("test").Start(context.Background(), "op")
	span.End()

	if called {
		t.Error("resolver should not be called without a service name")
	}
	if len(exportedAttributes(t, exporter)) != 0 {
		t.Error("expected no attributes")
	}
}

func TestSpanProcessorNoAttributesOnErrors(t *testing.T) {
	for name, err := range map[string]error{"not found": ErrNotFound, "other": errors.New("boom")} {
		t.Run(name, func(t *testing.T) {
			resolver := ResolverFunc(func(context.Context, string) (*models.Deployment, error) { return nil, err })
			tp, exporter := newProvider(t, "svc", NewSpanProcessor(resolver))
			_, span := tp.Tracer("test").Start(context.Background(), "op")
			span.End()

			if attrs := exportedAttributes(t, exporter); len(attrs) != 0 {
				t.Errorf("expected no attributes, got %v", attrs)
			}
		})
	}
}

func TestSpanProcessorLookupTimeout(t *testing.T) {
	resolver := ResolverFunc(func(ctx context.Context, _ string) (*models.Deployment, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})

	tp, exporter := newProvider(t, "svc", NewSpanProcessor(resolver, WithLookupTimeout(30*time.Millisecond)))

	start := time.Now()
	_, span := tp.Tracer("test").Start(context.Background(), "op")
	span.End()

	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Errorf("span start blocked for %v", elapsed)
	}
	if attrs := exportedAttributes(t, exporter); len(attrs) != 0 {
		t.Errorf("expected no attributes, got %v", attrs)
	}
}

func TestSpanProcessorNilSafety(t *testing.T) {
	p := NewSpanProcessor(nil)
	p.OnStart(context.Background(), nil)
	p.OnEnd(nil)
	if err := p.Shutdown(context.Background()); err != nil {
		t.Error(err)
	}
	if err := p.ForceFlush(context.Background()); err != nil {
		t.Error(err)
	}
}

func TestAttributesOmitsEmptyAndFallsBackToEventID(t *testing.T) {
	if Attributes(nil) != nil {
		t.Error("nil deployment should give nil attributes")
	}

	attrs := Attributes(&models.Deployment{EventID: "evt-9", Version: "v1"})
	got := make(map[attribute.Key]string)
	for _, a := range attrs {
		got[a.Key] = a.Value.AsString()
	}
	if got[DeploymentIDKey] != "evt-9" {
		t.Errorf("deployment.id should fall back to event id, got %q", got[DeploymentIDKey])
	}
	if _, ok := got[DeploymentCommitKey]; ok {
		t.Error("empty commit should be omitted")
	}
	if len(attrs) != 3 {
		t.Errorf("expected 3 attributes (id, version, event_id), got %d: %v", len(attrs), attrs)
	}
}
