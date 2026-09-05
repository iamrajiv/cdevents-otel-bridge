/*
Unit tests for configuration loading, layering and validation.
*/
package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDefaultIsValid(t *testing.T) {
	if err := Default().Validate(); err != nil {
		t.Errorf("Default() should validate: %v", err)
	}
}

func TestLoadWithoutFileUsesDefaults(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load(\"\") failed: %v", err)
	}
	if cfg.Server.Port != 8080 || cfg.Storage.Type != StorageMemory || cfg.Log.Format != LogFormatJSON {
		t.Errorf("unexpected defaults: %+v", cfg)
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "missing.yaml")); err == nil {
		t.Error("expected error for missing file")
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	path := writeConfig(t, "server: [not a map")
	if _, err := Load(path); err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestLoadPartialFileKeepsDefaults(t *testing.T) {
	path := writeConfig(t, "server:\n  port: 9090\nlog:\n  level: debug\n")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("Port = %d, want 9090", cfg.Server.Port)
	}
	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("Host = %q, want default", cfg.Server.Host)
	}
	if cfg.Log.Level != "debug" || cfg.Log.Format != LogFormatJSON {
		t.Errorf("Log = %+v, want debug/json", cfg.Log)
	}
	if cfg.Storage.RedisTTL != 86400 {
		t.Errorf("RedisTTL = %d, want default 86400", cfg.Storage.RedisTTL)
	}
	if cfg.OTel.ServiceName != "cdevents-otel-bridge" || !cfg.OTel.Insecure {
		t.Errorf("OTel = %+v, want defaults", cfg.OTel)
	}
}

func TestLoadFullFile(t *testing.T) {
	path := writeConfig(t, `
server:
  host: "127.0.0.1"
  port: 8181
storage:
  type: redis
  redisURL: redis://localhost:6379/1
  redisTTL: 3600
otel:
  enabled: true
  endpoint: collector:4317
  insecure: false
  serviceName: bridge-test
log:
  level: warn
  format: text
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Server.Host != "127.0.0.1" || cfg.Server.Port != 8181 {
		t.Errorf("Server = %+v", cfg.Server)
	}
	if cfg.Storage.Type != StorageRedis || cfg.Storage.RedisURL != "redis://localhost:6379/1" || cfg.Storage.TTL() != time.Hour {
		t.Errorf("Storage = %+v", cfg.Storage)
	}
	if !cfg.OTel.Enabled || cfg.OTel.Endpoint != "collector:4317" || cfg.OTel.Insecure || cfg.OTel.ServiceName != "bridge-test" {
		t.Errorf("OTel = %+v", cfg.OTel)
	}
	if cfg.Log.Level != "warn" || cfg.Log.Format != LogFormatText {
		t.Errorf("Log = %+v", cfg.Log)
	}
}

func TestEnvOverridesFile(t *testing.T) {
	path := writeConfig(t, "server:\n  port: 9090\nstorage:\n  type: memory\n")

	t.Setenv("BRIDGE_HOST", "localhost")
	t.Setenv("BRIDGE_PORT", "7070")
	t.Setenv("BRIDGE_STORAGE_TYPE", "redis")
	t.Setenv("BRIDGE_REDIS_URL", "redis://redis:6379")
	t.Setenv("BRIDGE_REDIS_TTL", "120")
	t.Setenv("BRIDGE_OTEL_ENABLED", "true")
	t.Setenv("BRIDGE_OTEL_ENDPOINT", "jaeger:4317")
	t.Setenv("BRIDGE_OTEL_INSECURE", "false")
	t.Setenv("BRIDGE_OTEL_SERVICE_NAME", "custom")
	t.Setenv("BRIDGE_LOG_LEVEL", "error")
	t.Setenv("BRIDGE_LOG_FORMAT", "text")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Server.Host != "localhost" || cfg.Server.Port != 7070 {
		t.Errorf("Server = %+v", cfg.Server)
	}
	if cfg.Storage.Type != StorageRedis || cfg.Storage.RedisURL != "redis://redis:6379" || cfg.Storage.RedisTTL != 120 {
		t.Errorf("Storage = %+v", cfg.Storage)
	}
	if !cfg.OTel.Enabled || cfg.OTel.Endpoint != "jaeger:4317" || cfg.OTel.Insecure || cfg.OTel.ServiceName != "custom" {
		t.Errorf("OTel = %+v", cfg.OTel)
	}
	if cfg.Log.Level != "error" || cfg.Log.Format != LogFormatText {
		t.Errorf("Log = %+v", cfg.Log)
	}
}

func TestEnvOverridesApplyWithoutFile(t *testing.T) {
	t.Setenv("BRIDGE_PORT", "1234")
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Port != 1234 {
		t.Errorf("Port = %d, want 1234", cfg.Server.Port)
	}
}

func TestInvalidEnvValues(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
	}{
		{"port", "BRIDGE_PORT", "eighty"},
		{"ttl", "BRIDGE_REDIS_TTL", "1h"},
		{"otel enabled", "BRIDGE_OTEL_ENABLED", "yes please"},
		{"otel insecure", "BRIDGE_OTEL_INSECURE", "maybe"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(tt.key, tt.value)
			if _, err := Load(""); err == nil || !strings.Contains(err.Error(), tt.key) {
				t.Errorf("expected error mentioning %s, got %v", tt.key, err)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr string
	}{
		{name: "port too low", mutate: func(c *Config) { c.Server.Port = 0 }, wantErr: "server port"},
		{name: "port too high", mutate: func(c *Config) { c.Server.Port = 70000 }, wantErr: "server port"},
		{name: "unknown storage", mutate: func(c *Config) { c.Storage.Type = "postgres" }, wantErr: "storage type"},
		{name: "redis without url", mutate: func(c *Config) { c.Storage.Type = StorageRedis }, wantErr: "redisURL"},
		{name: "negative ttl", mutate: func(c *Config) { c.Storage.RedisTTL = -1 }, wantErr: "redis TTL"},
		{name: "bad log level", mutate: func(c *Config) { c.Log.Level = "verbose" }, wantErr: "log level"},
		{name: "bad log format", mutate: func(c *Config) { c.Log.Format = "xml" }, wantErr: "log format"},
		{name: "otel without endpoint", mutate: func(c *Config) { c.OTel.Enabled = true; c.OTel.Endpoint = "" }, wantErr: "otel.endpoint"},
		{name: "otel without service name", mutate: func(c *Config) { c.OTel.Enabled = true; c.OTel.ServiceName = "" }, wantErr: "otel.serviceName"},
		{name: "redis with url is valid", mutate: func(c *Config) { c.Storage.Type = StorageRedis; c.Storage.RedisURL = "redis://x" }},
		{name: "otel disabled ignores endpoint", mutate: func(c *Config) { c.OTel.Endpoint = "" }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Default()
			tt.mutate(cfg)
			err := cfg.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}
