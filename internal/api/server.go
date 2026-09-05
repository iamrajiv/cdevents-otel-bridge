/*
Package api provides the HTTP API for the CDEvents-OTel Bridge.

This file assembles the router, middleware and http.Server. The server owns
its listener lifecycle: Start blocks until the listener closes and Shutdown
drains in-flight requests. When tracing is enabled the whole router is
wrapped with otelhttp so each request becomes a span, except health and
metrics probes, which would only add noise.
*/
package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/iamrajiv/cdevents-otel-bridge/internal/correlator"
	"github.com/iamrajiv/cdevents-otel-bridge/internal/metrics"
	"github.com/iamrajiv/cdevents-otel-bridge/internal/storage"
)

const (
	readHeaderTimeout = 10 * time.Second
	readTimeout       = 30 * time.Second
	writeTimeout      = 30 * time.Second
	idleTimeout       = 120 * time.Second
	healthPath        = "/api/v1/health"
	metricsPath       = "/metrics"
)

type Options struct {
	Host        string
	Port        int
	Storage     storage.Storage
	Correlator  *correlator.Correlator
	Metrics     *metrics.Metrics
	Logger      *slog.Logger
	Version     string
	Tracing     bool
	ServiceName string
}

type Server struct {
	httpServer *http.Server
	handler    http.Handler
	addr       string
}

func NewServer(opts Options) *Server {
	if opts.Logger == nil {
		opts.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if opts.Metrics == nil {
		opts.Metrics = metrics.New()
	}
	if opts.Correlator == nil {
		opts.Correlator = correlator.NewCorrelator(opts.Storage)
	}
	if opts.Version == "" {
		opts.Version = "dev"
	}
	if opts.ServiceName == "" {
		opts.ServiceName = "cdevents-otel-bridge"
	}

	h := NewHandler(opts.Storage, opts.Correlator, opts.Metrics, opts.Logger, opts.Version)

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(RecoveryMiddleware(opts.Logger))
	r.Use(LoggingMiddleware(opts.Logger))
	r.Use(CORSMiddleware)

	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/events", h.HandlePostEvent)
		r.Get("/events/{eventId}", h.HandleGetEvent)
		r.Get("/deployments", h.HandleListDeployments)
		r.Get("/deployments/{service}", h.HandleGetDeployment)
		r.Get("/chain/{eventId}", h.HandleGetChain)
		r.Get("/health", h.HandleHealth)
	})
	r.Handle(metricsPath, opts.Metrics.Handler())

	var handler http.Handler = r
	if opts.Tracing {
		handler = otelhttp.NewHandler(r, opts.ServiceName,
			otelhttp.WithFilter(func(req *http.Request) bool {
				return req.URL.Path != healthPath && req.URL.Path != metricsPath
			}),
			otelhttp.WithSpanNameFormatter(func(_ string, req *http.Request) string {
				return req.Method + " " + req.URL.Path
			}),
		)
	}

	addr := net.JoinHostPort(opts.Host, fmt.Sprint(opts.Port))

	return &Server{
		handler: handler,
		addr:    addr,
		httpServer: &http.Server{
			Addr:              addr,
			Handler:           handler,
			ReadHeaderTimeout: readHeaderTimeout,
			ReadTimeout:       readTimeout,
			WriteTimeout:      writeTimeout,
			IdleTimeout:       idleTimeout,
		},
	}
}

func (s *Server) Start() error {
	err := s.httpServer.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) Handler() http.Handler {
	return s.handler
}

func (s *Server) Addr() string {
	return s.addr
}
