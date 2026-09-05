/*
Sample application instrumented with OpenTelemetry and the otelbridge span
processor.

It is a small HTTP service whose traces carry deployment.* attributes
resolved from the CDEvents-OTel Bridge, which is exactly what a real service
would do to link production traces back to the deployment, commit and
pipeline that produced them.

Endpoints:

	GET /hello        returns a greeting and the current trace id
	GET /api/data     simulates work in a child span
	GET /deployment   shows what the bridge knows about this service
	GET /healthz      liveness probe (not traced)

Environment variables:

	OTEL_EXPORTER_OTLP_ENDPOINT  OTLP gRPC endpoint (default localhost:4317).
	                             host:port is dialed without TLS; use a
	                             full URL such as https://host:4317 for TLS.
	BRIDGE_URL                   bridge base URL (default http://localhost:8080)
	SERVICE_NAME                 service.name resource attribute and the
	                             service looked up in the bridge
	                             (default sample-app)
	ENVIRONMENT                  environment to resolve deployments for
	                             (default production)
	PORT                         listen port (default 8081)
	JAEGER_UI_URL                base URL used to build trace links in the
	                             /hello response (default
	                             http://localhost:16686)
*/
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/iamrajiv/cdevents-otel-bridge/pkg/otelbridge"
)

const shutdownTimeout = 5 * time.Second

type app struct {
	serviceName string
	environment string
	jaegerURL   string
	client      *otelbridge.Client
	tracer      trace.Tracer
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	otlpEndpoint := getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317")
	bridgeURL := getEnv("BRIDGE_URL", "http://localhost:8080")
	serviceName := getEnv("SERVICE_NAME", "sample-app")
	environment := getEnv("ENVIRONMENT", "production")
	port := getEnv("PORT", "8081")
	jaegerURL := strings.TrimRight(getEnv("JAEGER_UI_URL", "http://localhost:16686"), "/")

	client := otelbridge.NewClient(bridgeURL, otelbridge.WithEnvironment(environment))

	tp, err := initTracerProvider(ctx, otlpEndpoint, serviceName, client)
	if err != nil {
		log.Fatalf("failed to initialize tracer provider: %v", err)
	}

	a := &app{
		serviceName: serviceName,
		environment: environment,
		jaegerURL:   jaegerURL,
		client:      client,
		tracer:      otel.Tracer(serviceName),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/hello", a.handleHello)
	mux.HandleFunc("/api/data", a.handleData)
	mux.HandleFunc("/deployment", a.handleDeployment)
	mux.HandleFunc("/healthz", handleHealthz)

	handler := otelhttp.NewHandler(mux, serviceName,
		otelhttp.WithFilter(func(r *http.Request) bool { return r.URL.Path != "/healthz" }),
		otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string { return r.Method + " " + r.URL.Path }),
	)

	server := &http.Server{
		Addr:              ":" + port,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("sample app %q listening on :%s (otlp=%s bridge=%s environment=%s)",
			serviceName, port, otlpEndpoint, bridgeURL, environment)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("server shutdown error: %v", err)
	}
	if err := tp.Shutdown(shutdownCtx); err != nil {
		log.Printf("tracer provider shutdown error: %v", err)
	}
}

func initTracerProvider(ctx context.Context, endpoint, serviceName string, client *otelbridge.Client) (*sdktrace.TracerProvider, error) {
	var exporterOpts []otlptracegrpc.Option
	if strings.Contains(endpoint, "://") {
		exporterOpts = append(exporterOpts, otlptracegrpc.WithEndpointURL(endpoint))
	} else {
		exporterOpts = append(exporterOpts, otlptracegrpc.WithEndpoint(endpoint), otlptracegrpc.WithInsecure())
	}

	exporter, err := otlptracegrpc.New(ctx, exporterOpts...)
	if err != nil {
		return nil, fmt.Errorf("create OTLP exporter: %w", err)
	}

	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(semconv.SchemaURL,
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion("demo"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("create resource: %w", err)
	}

	processor := otelbridge.NewSpanProcessor(
		otelbridge.NewCachedResolver(client, otelbridge.WithCacheTTL(15*time.Second)),
	)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSpanProcessor(processor),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	return tp, nil
}

func (a *app) handleHello(w http.ResponseWriter, r *http.Request) {
	span := trace.SpanFromContext(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"message":  "Hello from " + a.serviceName,
		"time":     time.Now().UTC().Format(time.RFC3339),
		"traceId":  span.SpanContext().TraceID().String(),
		"jaegerUI": a.jaegerURL + "/trace/" + span.SpanContext().TraceID().String(),
	})
}

func (a *app) handleData(w http.ResponseWriter, r *http.Request) {
	ctx, span := a.tracer.Start(r.Context(), "load-data")
	defer span.End()

	items := []string{"item1", "item2", "item3"}
	span.SetAttributes(attribute.Int("data.count", len(items)))

	_, dbSpan := a.tracer.Start(ctx, "fake-db-query")
	time.Sleep(25 * time.Millisecond)
	dbSpan.End()

	writeJSON(w, http.StatusOK, map[string]any{
		"data":      items,
		"count":     len(items),
		"timestamp": time.Now().Unix(),
		"traceId":   span.SpanContext().TraceID().String(),
	})
}

func (a *app) handleDeployment(w http.ResponseWriter, r *http.Request) {
	service := r.URL.Query().Get("service")
	if service == "" {
		service = a.serviceName
	}
	environment := r.URL.Query().Get("environment")
	if environment == "" {
		environment = a.environment
	}

	deployment, err := a.client.GetDeployment(r.Context(), service, environment)
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, otelbridge.ErrNotFound) {
			status = http.StatusNotFound
		}
		writeJSON(w, status, map[string]any{"error": err.Error(), "service": service, "environment": environment})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"service":     service,
		"environment": environment,
		"deployment":  deployment,
	})
}

func handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("failed to write response: %v", err)
	}
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
