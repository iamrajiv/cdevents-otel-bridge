/*
Package otelbridge enriches OpenTelemetry spans with deployment context.

This file implements Client, a small HTTP client for the bridge's query API.
GetDeployment calls GET /api/v1/deployments/{service}, optionally scoped to
an environment; without an environment the bridge returns the most recent
deployment of the service across all environments. Client implements
DeploymentResolver so it can be wrapped in a CachedResolver and handed to
the span processor.

Use a plain http.Client here. An OpenTelemetry-instrumented transport would
create a span for every lookup, and because lookups happen from inside span
creation that would recurse.
*/
package otelbridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/iamrajiv/cdevents-otel-bridge/pkg/models"
)

const (
	DefaultRequestTimeout = 2 * time.Second
	maxResponseBytes      = 1 << 20
)

type Client struct {
	baseURL     string
	environment string
	httpClient  *http.Client
}

type ClientOption func(*Client)

func WithHTTPClient(httpClient *http.Client) ClientOption {
	return func(c *Client) {
		if httpClient != nil {
			c.httpClient = httpClient
		}
	}
}

func WithEnvironment(environment string) ClientOption {
	return func(c *Client) {
		c.environment = environment
	}
}

func WithRequestTimeout(timeout time.Duration) ClientOption {
	return func(c *Client) {
		if timeout > 0 {
			c.httpClient.Timeout = timeout
		}
	}
}

func NewClient(baseURL string, opts ...ClientOption) *Client {
	c := &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: DefaultRequestTimeout},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *Client) ResolveDeployment(ctx context.Context, service string) (*models.Deployment, error) {
	return c.GetDeployment(ctx, service, c.environment)
}

func (c *Client) GetDeployment(ctx context.Context, service, environment string) (*models.Deployment, error) {
	if service == "" {
		return nil, errors.New("otelbridge: service name is required")
	}

	endpoint := c.baseURL + "/api/v1/deployments/" + url.PathEscape(service)
	if environment != "" {
		endpoint += "?environment=" + url.QueryEscape(environment)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("otelbridge: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("otelbridge: request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("otelbridge: read response: %w", err)
	}

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return nil, ErrNotFound
	default:
		return nil, fmt.Errorf("otelbridge: bridge returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var payload struct {
		Deployment *models.Deployment `json:"deployment"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("otelbridge: decode response: %w", err)
	}
	if payload.Deployment == nil {
		return nil, ErrNotFound
	}

	return payload.Deployment, nil
}
