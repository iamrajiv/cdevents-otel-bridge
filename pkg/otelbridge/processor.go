/*
Package otelbridge enriches OpenTelemetry spans with deployment context.

This file implements SpanProcessor, an sdktrace.SpanProcessor that runs on
span start. It determines the service the span belongs to (from the
service.name resource attribute, or a configured override), asks the
resolver for that service's current deployment and copies the deployment
attributes onto the span. Lookups are bounded by LookupTimeout and detached
from the span's own context, so a slow or unreachable bridge can delay a
span by at most that timeout and never fails the traced operation.
*/
package otelbridge

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"time"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
)

const DefaultLookupTimeout = 500 * time.Millisecond

type SpanProcessor struct {
	resolver    DeploymentResolver
	serviceName string
	timeout     time.Duration
	logger      *slog.Logger
}

type ProcessorOption func(*SpanProcessor)

func WithServiceName(name string) ProcessorOption {
	return func(p *SpanProcessor) {
		p.serviceName = name
	}
}

func WithLookupTimeout(timeout time.Duration) ProcessorOption {
	return func(p *SpanProcessor) {
		if timeout > 0 {
			p.timeout = timeout
		}
	}
}

func WithLogger(logger *slog.Logger) ProcessorOption {
	return func(p *SpanProcessor) {
		if logger != nil {
			p.logger = logger
		}
	}
}

func NewSpanProcessor(resolver DeploymentResolver, opts ...ProcessorOption) *SpanProcessor {
	p := &SpanProcessor{
		resolver: resolver,
		timeout:  DefaultLookupTimeout,
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

func (p *SpanProcessor) OnStart(_ context.Context, span sdktrace.ReadWriteSpan) {
	if p.resolver == nil || span == nil {
		return
	}

	service := p.serviceName
	if service == "" {
		service = serviceNameFromResource(span)
	}
	if service == "" {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
	defer cancel()

	deployment, err := p.resolver.ResolveDeployment(ctx, service)
	if err != nil {
		if !errors.Is(err, ErrNotFound) {
			p.logger.Debug("deployment lookup failed", "service", service, "error", err)
		}
		return
	}

	span.SetAttributes(Attributes(deployment)...)
}

func (p *SpanProcessor) OnEnd(sdktrace.ReadOnlySpan) {}

func (p *SpanProcessor) Shutdown(context.Context) error { return nil }

func (p *SpanProcessor) ForceFlush(context.Context) error { return nil }

func serviceNameFromResource(span sdktrace.ReadWriteSpan) string {
	res := span.Resource()
	if res == nil {
		return ""
	}
	for _, attr := range res.Attributes() {
		if attr.Key == semconv.ServiceNameKey {
			return attr.Value.AsString()
		}
	}
	return ""
}
