/*
CDEvents to OpenTelemetry Bridge

Entry point for the bridge service. It loads configuration, picks the
storage backend, optionally enables OpenTelemetry tracing for the bridge's
own HTTP requests, starts the API server and shuts everything down in order
on SIGINT or SIGTERM.

Usage:

	bridge [-config path/to/config.yaml] [-version]

When -config is not given, configs/config.yaml is used if it exists and the
built-in defaults otherwise. Environment variables (see internal/config)
override both. The Version variable is set at build time with
-ldflags "-X main.Version=...".
*/
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/iamrajiv/cdevents-otel-bridge/internal/api"
	"github.com/iamrajiv/cdevents-otel-bridge/internal/config"
	"github.com/iamrajiv/cdevents-otel-bridge/internal/correlator"
	"github.com/iamrajiv/cdevents-otel-bridge/internal/metrics"
	bridgeotel "github.com/iamrajiv/cdevents-otel-bridge/internal/otel"
	"github.com/iamrajiv/cdevents-otel-bridge/internal/storage"
	"github.com/iamrajiv/cdevents-otel-bridge/pkg/otelbridge"
)

const (
	defaultConfigPath = "configs/config.yaml"
	shutdownTimeout   = 15 * time.Second
)

var Version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "bridge: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", "", "Path to configuration file (default: "+defaultConfigPath+" if present)")
	showVersion := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(Version)
		return nil
	}

	cfg, err := loadConfig(*configPath)
	if err != nil {
		return err
	}

	logger := newLogger(cfg.Log)
	logger.Info("starting CDEvents to OpenTelemetry bridge",
		"version", Version,
		"address", fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		"storage", cfg.Storage.Type,
		"otel_enabled", cfg.OTel.Enabled,
	)

	store, err := newStorage(cfg.Storage)
	if err != nil {
		return err
	}
	defer func() {
		if err := store.Close(); err != nil {
			logger.Error("failed to close storage", "error", err)
		}
	}()
	logger.Info("storage ready", "type", cfg.Storage.Type)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var tracerProvider *sdktrace.TracerProvider
	if cfg.OTel.Enabled {
		processor := otelbridge.NewSpanProcessor(
			otelbridge.NewCachedResolver(bridgeotel.NewStorageResolver(store)),
			otelbridge.WithLogger(logger),
		)
		tracerProvider, err = bridgeotel.NewTracerProvider(ctx, bridgeotel.Config{
			Endpoint:       cfg.OTel.Endpoint,
			Insecure:       cfg.OTel.Insecure,
			ServiceName:    cfg.OTel.ServiceName,
			ServiceVersion: Version,
		}, processor)
		if err != nil {
			return fmt.Errorf("failed to initialize OpenTelemetry: %w", err)
		}
		logger.Info("OpenTelemetry tracing enabled", "endpoint", cfg.OTel.Endpoint, "service", cfg.OTel.ServiceName)
	}

	server := api.NewServer(api.Options{
		Host:        cfg.Server.Host,
		Port:        cfg.Server.Port,
		Storage:     store,
		Correlator:  correlator.NewCorrelator(store),
		Metrics:     metrics.New(),
		Logger:      logger,
		Version:     Version,
		Tracing:     cfg.OTel.Enabled,
		ServiceName: cfg.OTel.ServiceName,
	})

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("HTTP server listening", "address", server.Addr())
		serverErr <- server.Start()
	}()

	select {
	case err := <-serverErr:
		if err != nil {
			return fmt.Errorf("server failed: %w", err)
		}
		return nil
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("HTTP server shutdown failed", "error", err)
	}
	if tracerProvider != nil {
		if err := tracerProvider.Shutdown(shutdownCtx); err != nil {
			logger.Error("tracer provider shutdown failed", "error", err)
		}
	}

	logger.Info("shutdown complete")
	return nil
}

func loadConfig(path string) (*config.Config, error) {
	if path == "" {
		if env := os.Getenv("BRIDGE_CONFIG"); env != "" {
			path = env
		} else if _, err := os.Stat(defaultConfigPath); err == nil {
			path = defaultConfigPath
		}
	}

	cfg, err := config.Load(path)
	if err != nil {
		if path != "" && errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("config file %s does not exist", path)
		}
		return nil, err
	}
	return cfg, nil
}

func newStorage(cfg config.StorageConfig) (storage.Storage, error) {
	switch cfg.Type {
	case config.StorageRedis:
		store, err := storage.NewRedisStorage(cfg.RedisURL, cfg.TTL())
		if err != nil {
			return nil, fmt.Errorf("failed to connect to redis: %w", err)
		}
		return store, nil
	default:
		return storage.NewMemoryStorage(), nil
	}
}

func newLogger(cfg config.LogConfig) *slog.Logger {
	var level slog.Level
	switch cfg.Level {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: level}
	if cfg.Format == config.LogFormatText {
		return slog.New(slog.NewTextHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, opts))
}
