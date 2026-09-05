/*
Package config provides configuration management for the CDEvents-OTel Bridge.

Configuration is resolved in three layers, each overriding the previous one:

 1. Built-in defaults (Default).

 2. An optional YAML file. Keys that are absent keep their default value.

 3. Environment variables:

    BRIDGE_HOST               server.host
    BRIDGE_PORT               server.port
    BRIDGE_STORAGE_TYPE       storage.type        ("memory" or "redis")
    BRIDGE_REDIS_URL          storage.redisURL    (redis://[:password@]host:port[/db])
    BRIDGE_REDIS_TTL          storage.redisTTL    (seconds, 0 = keep forever)
    BRIDGE_OTEL_ENABLED       otel.enabled
    BRIDGE_OTEL_ENDPOINT      otel.endpoint       (host:port or http(s)://host:port)
    BRIDGE_OTEL_INSECURE      otel.insecure       (plain gRPC without TLS)
    BRIDGE_OTEL_SERVICE_NAME  otel.serviceName
    BRIDGE_LOG_LEVEL          log.level           (debug, info, warn, error)
    BRIDGE_LOG_FORMAT         log.format          (json or text)

The result is validated before it is returned so the rest of the program can
trust every field.
*/
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	StorageMemory = "memory"
	StorageRedis  = "redis"

	LogFormatJSON = "json"
	LogFormatText = "text"
)

type Config struct {
	Server  ServerConfig  `yaml:"server"`
	Storage StorageConfig `yaml:"storage"`
	OTel    OTelConfig    `yaml:"otel"`
	Log     LogConfig     `yaml:"log"`
}

type ServerConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

type StorageConfig struct {
	Type     string `yaml:"type"`
	RedisURL string `yaml:"redisURL,omitempty"`
	RedisTTL int    `yaml:"redisTTL,omitempty"`
}

type OTelConfig struct {
	Enabled     bool   `yaml:"enabled"`
	Endpoint    string `yaml:"endpoint"`
	Insecure    bool   `yaml:"insecure"`
	ServiceName string `yaml:"serviceName"`
}

type LogConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

func Default() *Config {
	return &Config{
		Server: ServerConfig{
			Host: "0.0.0.0",
			Port: 8080,
		},
		Storage: StorageConfig{
			Type:     StorageMemory,
			RedisTTL: 86400,
		},
		OTel: OTelConfig{
			Enabled:     false,
			Endpoint:    "localhost:4317",
			Insecure:    true,
			ServiceName: "cdevents-otel-bridge",
		},
		Log: LogConfig{
			Level:  "info",
			Format: LogFormatJSON,
		},
	}
}

func Load(path string) (*Config, error) {
	cfg := Default()

	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("failed to parse config file %s: %w", path, err)
		}
	}

	if err := applyEnvOverrides(cfg); err != nil {
		return nil, err
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}

func (c *Config) Validate() error {
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", c.Server.Port)
	}

	switch c.Storage.Type {
	case StorageMemory:
	case StorageRedis:
		if c.Storage.RedisURL == "" {
			return errors.New("redis storage requires storage.redisURL to be set")
		}
	default:
		return fmt.Errorf("invalid storage type: %q (must be %q or %q)", c.Storage.Type, StorageMemory, StorageRedis)
	}

	if c.Storage.RedisTTL < 0 {
		return fmt.Errorf("invalid redis TTL: %d (must be >= 0)", c.Storage.RedisTTL)
	}

	switch c.Log.Level {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("invalid log level: %q (must be debug, info, warn, or error)", c.Log.Level)
	}

	switch c.Log.Format {
	case LogFormatJSON, LogFormatText:
	default:
		return fmt.Errorf("invalid log format: %q (must be json or text)", c.Log.Format)
	}

	if c.OTel.Enabled {
		if c.OTel.Endpoint == "" {
			return errors.New("otel.endpoint is required when otel is enabled")
		}
		if c.OTel.ServiceName == "" {
			return errors.New("otel.serviceName is required when otel is enabled")
		}
	}

	return nil
}

func (s StorageConfig) TTL() time.Duration {
	return time.Duration(s.RedisTTL) * time.Second
}

func applyEnvOverrides(cfg *Config) error {
	if v := os.Getenv("BRIDGE_HOST"); v != "" {
		cfg.Server.Host = v
	}

	if v := os.Getenv("BRIDGE_PORT"); v != "" {
		port, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("invalid BRIDGE_PORT %q: %w", v, err)
		}
		cfg.Server.Port = port
	}

	if v := os.Getenv("BRIDGE_STORAGE_TYPE"); v != "" {
		cfg.Storage.Type = v
	}

	if v := os.Getenv("BRIDGE_REDIS_URL"); v != "" {
		cfg.Storage.RedisURL = v
	}

	if v := os.Getenv("BRIDGE_REDIS_TTL"); v != "" {
		ttl, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("invalid BRIDGE_REDIS_TTL %q: %w", v, err)
		}
		cfg.Storage.RedisTTL = ttl
	}

	if v := os.Getenv("BRIDGE_OTEL_ENABLED"); v != "" {
		enabled, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("invalid BRIDGE_OTEL_ENABLED %q: %w", v, err)
		}
		cfg.OTel.Enabled = enabled
	}

	if v := os.Getenv("BRIDGE_OTEL_ENDPOINT"); v != "" {
		cfg.OTel.Endpoint = v
	}

	if v := os.Getenv("BRIDGE_OTEL_INSECURE"); v != "" {
		insecure, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("invalid BRIDGE_OTEL_INSECURE %q: %w", v, err)
		}
		cfg.OTel.Insecure = insecure
	}

	if v := os.Getenv("BRIDGE_OTEL_SERVICE_NAME"); v != "" {
		cfg.OTel.ServiceName = v
	}

	if v := os.Getenv("BRIDGE_LOG_LEVEL"); v != "" {
		cfg.Log.Level = v
	}

	if v := os.Getenv("BRIDGE_LOG_FORMAT"); v != "" {
		cfg.Log.Format = v
	}

	return nil
}
