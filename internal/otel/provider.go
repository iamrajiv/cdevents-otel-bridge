/*
Package otel wires OpenTelemetry into the bridge itself.

The bridge exports its own HTTP request traces over OTLP/gRPC so it shows
up next to the applications it serves. NewTracerProvider builds the
exporter, the resource (service name and version) and the provider,
registers the provider and the W3C propagators globally, and attaches any
extra span processors, such as the otelbridge processor that stamps the
bridge's own deployment metadata onto its spans.
*/
package otel

import (
	"context"
	"fmt"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
)

type Config struct {
	Endpoint       string
	Insecure       bool
	ServiceName    string
	ServiceVersion string
}

func NewTracerProvider(ctx context.Context, cfg Config, processors ...sdktrace.SpanProcessor) (*sdktrace.TracerProvider, error) {
	exporter, err := otlptracegrpc.New(ctx, exporterOptions(cfg)...)
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP exporter: %w", err)
	}

	attrs := []attribute.KeyValue{semconv.ServiceName(cfg.ServiceName)}
	if cfg.ServiceVersion != "" {
		attrs = append(attrs, semconv.ServiceVersion(cfg.ServiceVersion))
	}

	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(semconv.SchemaURL, attrs...),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	opts := []sdktrace.TracerProviderOption{
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	}
	for _, processor := range processors {
		if processor != nil {
			opts = append(opts, sdktrace.WithSpanProcessor(processor))
		}
	}

	tp := sdktrace.NewTracerProvider(opts...)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return tp, nil
}

func exporterOptions(cfg Config) []otlptracegrpc.Option {
	endpoint := strings.TrimSpace(cfg.Endpoint)
	if strings.Contains(endpoint, "://") {
		return []otlptracegrpc.Option{otlptracegrpc.WithEndpointURL(endpoint)}
	}

	opts := []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(endpoint)}
	if cfg.Insecure {
		opts = append(opts, otlptracegrpc.WithInsecure())
	}
	return opts
}
