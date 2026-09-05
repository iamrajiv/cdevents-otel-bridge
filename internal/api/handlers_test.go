/*
Unit tests for the HTTP API.

Every test drives the full router (middleware included) through httptest
with the real in-memory storage and correlator, so status codes, JSON
shapes and error codes are verified exactly as clients see them. Storage
failures are simulated with a wrapper that fails selected operations.
*/
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/iamrajiv/cdevents-otel-bridge/internal/cdevents"
	"github.com/iamrajiv/cdevents-otel-bridge/internal/correlator"
	"github.com/iamrajiv/cdevents-otel-bridge/internal/storage"
	"github.com/iamrajiv/cdevents-otel-bridge/pkg/models"
)

type failingStorage struct {
	storage.Storage
	failSaveDeployment bool
	failSaveEvent      bool
	failGet            bool
	failPing           bool
}

var errBoom = errors.New("boom")

func (f *failingStorage) SaveDeployment(ctx context.Context, d *models.Deployment) error {
	if f.failSaveDeployment {
		return errBoom
	}
	return f.Storage.SaveDeployment(ctx, d)
}

func (f *failingStorage) SaveEvent(ctx context.Context, e *cdevents.StoredEvent) error {
	if f.failSaveEvent {
		return errBoom
	}
	return f.Storage.SaveEvent(ctx, e)
}

func (f *failingStorage) GetDeployment(ctx context.Context, s, e string) (*models.Deployment, error) {
	if f.failGet {
		return nil, errBoom
	}
	return f.Storage.GetDeployment(ctx, s, e)
}

func (f *failingStorage) GetLatestDeployment(ctx context.Context, s string) (*models.Deployment, error) {
	if f.failGet {
		return nil, errBoom
	}
	return f.Storage.GetLatestDeployment(ctx, s)
}

func (f *failingStorage) ListDeployments(ctx context.Context, filter storage.ListFilter) ([]*models.Deployment, error) {
	if f.failGet {
		return nil, errBoom
	}
	return f.Storage.ListDeployments(ctx, filter)
}

func (f *failingStorage) GetEvent(ctx context.Context, id string) (*cdevents.StoredEvent, error) {
	if f.failGet {
		return nil, errBoom
	}
	return f.Storage.GetEvent(ctx, id)
}

func (f *failingStorage) Ping(ctx context.Context) error {
	if f.failPing {
		return errBoom
	}
	return f.Storage.Ping(ctx)
}

func newTestHandler(t *testing.T, store storage.Storage) http.Handler {
	t.Helper()
	if store == nil {
		store = storage.NewMemoryStorage()
	}
	server := NewServer(Options{
		Host:       "127.0.0.1",
		Port:       0,
		Storage:    store,
		Correlator: correlator.NewCorrelator(store),
		Logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		Version:    "test-version",
	})
	return server.Handler()
}

func do(t *testing.T, h http.Handler, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, target, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decode(t *testing.T, rec *httptest.ResponseRecorder, into any) {
	t.Helper()
	if err := json.NewDecoder(rec.Body).Decode(into); err != nil {
		t.Fatalf("failed to decode response %q: %v", rec.Body.String(), err)
	}
}

func assertError(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d, want %d (body %s)", rec.Code, status, rec.Body.String())
	}
	var resp ErrorResponse
	decode(t, rec, &resp)
	if resp.Error != code {
		t.Errorf("error code = %q, want %q", resp.Error, code)
	}
}

func deployedEvent(id, service, environment, version, ts string) string {
	return fmt.Sprintf(`{
		"context": {"version": "0.4.1", "id": %q, "source": "https://ci.example.com/run/1",
			"type": "dev.cdevents.service.deployed.0.1.1", "timestamp": %q},
		"subject": {"id": %q, "type": "service", "content": {
			"artifactId": "%s:%s", "environment": {"id": %q}}},
		"customData": {"commitSha": "abc123", "repository": "github.com/org/app", "deployer": "ci"}
	}`, id, ts, service, service, version, environment)
}

func TestHealth(t *testing.T) {
	t.Run("healthy", func(t *testing.T) {
		rec := do(t, newTestHandler(t, nil), http.MethodGet, "/api/v1/health", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
		var resp struct {
			Status  string            `json:"status"`
			Version string            `json:"version"`
			Uptime  string            `json:"uptime"`
			Checks  map[string]string `json:"checks"`
		}
		decode(t, rec, &resp)
		if resp.Status != "healthy" || resp.Version != "test-version" || resp.Uptime == "" || resp.Checks["storage"] != "ok" {
			t.Errorf("unexpected health response: %+v", resp)
		}
	})

	t.Run("storage failure", func(t *testing.T) {
		h := newTestHandler(t, &failingStorage{Storage: storage.NewMemoryStorage(), failPing: true})
		rec := do(t, h, http.MethodGet, "/api/v1/health", "")
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503", rec.Code)
		}
		var resp struct {
			Status string            `json:"status"`
			Checks map[string]string `json:"checks"`
		}
		decode(t, rec, &resp)
		if resp.Status != "unhealthy" || !strings.Contains(resp.Checks["storage"], "boom") {
			t.Errorf("unexpected health response: %+v", resp)
		}
	})
}

func TestPostEvent(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		store      storage.Storage
		wantStatus int
		wantCode   string
	}{
		{
			name:       "valid deployment",
			body:       deployedEvent("evt-1", "api", "production", "v1.2.3", "2024-01-15T10:30:00Z"),
			wantStatus: http.StatusCreated,
		},
		{
			name: "valid pipeline run",
			body: `{"context": {"id": "p-1", "source": "ci", "type": "dev.cdevents.pipelinerun.finished.0.2.0", "timestamp": "2024-01-15T10:00:00Z"},
				"subject": {"id": "run-1", "type": "pipelineRun", "content": {"pipelineName": "build", "outcome": "success"}}}`,
			wantStatus: http.StatusCreated,
		},
		{
			name: "valid incident",
			body: `{"context": {"id": "i-1", "source": "alerts", "type": "dev.cdevents.incident.detected.0.2.0", "timestamp": "2024-01-15T10:00:00Z"},
				"subject": {"id": "inc-1", "type": "incident", "content": {"severity": "high", "service": "api"}}}`,
			wantStatus: http.StatusCreated,
		},
		{
			name: "valid generic event",
			body: `{"context": {"id": "a-1", "source": "ci", "type": "dev.cdevents.artifact.published.0.1.1", "timestamp": "2024-01-15T10:00:00Z"},
				"subject": {"id": "pkg", "type": "artifact"}}`,
			wantStatus: http.StatusCreated,
		},
		{
			name:       "invalid JSON",
			body:       `{invalid json`,
			wantStatus: http.StatusBadRequest,
			wantCode:   "invalid_event",
		},
		{
			name: "missing required field",
			body: `{"context": {"source": "ci", "type": "dev.cdevents.service.deployed.0.1.1", "timestamp": "2024-01-15T10:30:00Z"},
				"subject": {"id": "api", "type": "service"}}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   "validation_error",
		},
		{
			name:       "payload too large",
			body:       `{"context": {"id": "big"}, "pad": "` + strings.Repeat("x", maxBodyBytes) + `"}`,
			wantStatus: http.StatusRequestEntityTooLarge,
			wantCode:   "payload_too_large",
		},
		{
			name:       "deployment storage failure",
			body:       deployedEvent("evt-2", "api", "production", "v1", "2024-01-15T10:30:00Z"),
			store:      &failingStorage{Storage: storage.NewMemoryStorage(), failSaveDeployment: true},
			wantStatus: http.StatusInternalServerError,
			wantCode:   "storage_error",
		},
		{
			name:       "event storage failure",
			body:       deployedEvent("evt-3", "api", "production", "v1", "2024-01-15T10:30:00Z"),
			store:      &failingStorage{Storage: storage.NewMemoryStorage(), failSaveEvent: true},
			wantStatus: http.StatusInternalServerError,
			wantCode:   "storage_error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(t, newTestHandler(t, tt.store), http.MethodPost, "/api/v1/events", tt.body)
			if tt.wantCode != "" {
				assertError(t, rec, tt.wantStatus, tt.wantCode)
				return
			}
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			var resp SuccessResponse
			decode(t, rec, &resp)
			if resp.Status != "accepted" || resp.EventID == "" || resp.EventType == "" || resp.Timestamp == "" {
				t.Errorf("unexpected success response: %+v", resp)
			}
		})
	}
}

func TestPostEventResponseFields(t *testing.T) {
	rec := do(t, newTestHandler(t, nil), http.MethodPost, "/api/v1/events",
		deployedEvent("event-123", "api-service", "production", "v1.2.3", "2024-01-15T10:30:00+02:00"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var resp SuccessResponse
	decode(t, rec, &resp)
	if resp.EventID != "event-123" || resp.EventType != "dev.cdevents.service.deployed.0.1.1" || resp.Timestamp != "2024-01-15T08:30:00Z" {
		t.Errorf("unexpected response: %+v", resp)
	}
}

func TestGetDeployment(t *testing.T) {
	h := newTestHandler(t, nil)
	for _, body := range []string{
		deployedEvent("evt-prod", "api", "production", "v1.0.0", "2024-01-15T10:00:00Z"),
		deployedEvent("evt-staging", "api", "staging", "v1.1.0", "2024-01-15T11:00:00Z"),
	} {
		if rec := do(t, h, http.MethodPost, "/api/v1/events", body); rec.Code != http.StatusCreated {
			t.Fatalf("post failed: %s", rec.Body.String())
		}
	}

	type deploymentResponse struct {
		Service    string             `json:"service"`
		Deployment *models.Deployment `json:"deployment"`
		Links      map[string]string  `json:"links"`
	}

	t.Run("by environment", func(t *testing.T) {
		rec := do(t, h, http.MethodGet, "/api/v1/deployments/api?environment=production", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
		}
		var resp deploymentResponse
		decode(t, rec, &resp)
		if resp.Service != "api" || resp.Deployment.Version != "v1.0.0" || resp.Deployment.Environment != "production" {
			t.Errorf("unexpected response: %+v", resp.Deployment)
		}
		if resp.Links["event"] != "/api/v1/events/evt-prod" || resp.Links["chain"] != "/api/v1/chain/evt-prod" || resp.Links["self"] == "" {
			t.Errorf("unexpected links: %v", resp.Links)
		}
	})

	t.Run("latest without environment", func(t *testing.T) {
		rec := do(t, h, http.MethodGet, "/api/v1/deployments/api", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
		}
		var resp deploymentResponse
		decode(t, rec, &resp)
		if resp.Deployment.Version != "v1.1.0" || resp.Deployment.Environment != "staging" {
			t.Errorf("expected latest (staging v1.1.0), got %+v", resp.Deployment)
		}
	})

	t.Run("not found", func(t *testing.T) {
		assertError(t, do(t, h, http.MethodGet, "/api/v1/deployments/nope", ""), http.StatusNotFound, "not_found")
		assertError(t, do(t, h, http.MethodGet, "/api/v1/deployments/api?environment=qa", ""), http.StatusNotFound, "not_found")
	})

	t.Run("storage failure", func(t *testing.T) {
		failing := newTestHandler(t, &failingStorage{Storage: storage.NewMemoryStorage(), failGet: true})
		assertError(t, do(t, failing, http.MethodGet, "/api/v1/deployments/api", ""), http.StatusInternalServerError, "storage_error")
		assertError(t, do(t, failing, http.MethodGet, "/api/v1/deployments/api?environment=x", ""), http.StatusInternalServerError, "storage_error")
	})
}

func TestListDeployments(t *testing.T) {
	h := newTestHandler(t, nil)
	for _, body := range []string{
		deployedEvent("e1", "service-a", "production", "v1", "2024-01-15T10:00:00Z"),
		deployedEvent("e2", "service-b", "staging", "v2", "2024-01-15T11:00:00Z"),
		deployedEvent("e3", "service-c", "production", "v3", "2024-01-15T12:00:00Z"),
	} {
		if rec := do(t, h, http.MethodPost, "/api/v1/events", body); rec.Code != http.StatusCreated {
			t.Fatalf("post failed: %s", rec.Body.String())
		}
	}

	type listResponse struct {
		Deployments []*models.Deployment `json:"deployments"`
		Total       int                  `json:"total"`
		Limit       int                  `json:"limit"`
	}

	tests := []struct {
		name      string
		url       string
		wantOrder []string
		wantLimit int
	}{
		{name: "all newest first", url: "/api/v1/deployments", wantOrder: []string{"service-c", "service-b", "service-a"}, wantLimit: 50},
		{name: "environment filter", url: "/api/v1/deployments?environment=production", wantOrder: []string{"service-c", "service-a"}, wantLimit: 50},
		{name: "limit", url: "/api/v1/deployments?limit=1", wantOrder: []string{"service-c"}, wantLimit: 1},
		{name: "limit clamped", url: "/api/v1/deployments?limit=500", wantOrder: []string{"service-c", "service-b", "service-a"}, wantLimit: 100},
		{name: "since", url: "/api/v1/deployments?since=2024-01-15T11:00:00Z", wantOrder: []string{"service-c", "service-b"}, wantLimit: 50},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(t, h, http.MethodGet, tt.url, "")
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
			}
			var resp listResponse
			decode(t, rec, &resp)
			if resp.Total != len(tt.wantOrder) || resp.Limit != tt.wantLimit {
				t.Errorf("total/limit = %d/%d, want %d/%d", resp.Total, resp.Limit, len(tt.wantOrder), tt.wantLimit)
			}
			for i, want := range tt.wantOrder {
				if i >= len(resp.Deployments) || resp.Deployments[i].Service != want {
					t.Errorf("position %d: want %s, got %+v", i, want, resp.Deployments)
					break
				}
			}
		})
	}

	t.Run("invalid parameters", func(t *testing.T) {
		assertError(t, do(t, h, http.MethodGet, "/api/v1/deployments?limit=abc", ""), http.StatusBadRequest, "invalid_request")
		assertError(t, do(t, h, http.MethodGet, "/api/v1/deployments?limit=0", ""), http.StatusBadRequest, "invalid_request")
		assertError(t, do(t, h, http.MethodGet, "/api/v1/deployments?since=yesterday", ""), http.StatusBadRequest, "invalid_request")
	})

	t.Run("storage failure", func(t *testing.T) {
		failing := newTestHandler(t, &failingStorage{Storage: storage.NewMemoryStorage(), failGet: true})
		assertError(t, do(t, failing, http.MethodGet, "/api/v1/deployments", ""), http.StatusInternalServerError, "storage_error")
	})
}

func TestGetEvent(t *testing.T) {
	h := newTestHandler(t, nil)
	if rec := do(t, h, http.MethodPost, "/api/v1/events", deployedEvent("event-abc", "api", "production", "v1", "2024-01-15T10:00:00Z")); rec.Code != http.StatusCreated {
		t.Fatalf("post failed: %s", rec.Body.String())
	}

	rec := do(t, h, http.MethodGet, "/api/v1/events/event-abc", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Event *cdevents.StoredEvent `json:"event"`
	}
	decode(t, rec, &resp)
	if resp.Event == nil || resp.Event.ID != "event-abc" || resp.Event.Summary != "Deployed api v1 to production" {
		t.Errorf("unexpected event: %+v", resp.Event)
	}

	assertError(t, do(t, h, http.MethodGet, "/api/v1/events/missing", ""), http.StatusNotFound, "not_found")

	failing := newTestHandler(t, &failingStorage{Storage: storage.NewMemoryStorage(), failGet: true})
	assertError(t, do(t, failing, http.MethodGet, "/api/v1/events/event-abc", ""), http.StatusInternalServerError, "storage_error")
}

func TestGetChain(t *testing.T) {
	h := newTestHandler(t, nil)
	events := []string{
		`{"context": {"id": "change-1", "source": "github", "type": "dev.cdevents.change.merged.0.4.1", "timestamp": "2024-01-15T10:00:00Z", "chainId": "chain-1"},
		  "subject": {"id": "pr-42", "type": "change"},
		  "customData": {"commitSha": "abc123", "author": "dev@example.com", "message": "Add feature X", "repository": "github.com/org/app"}}`,
		`{"context": {"id": "pipeline-1", "source": "ci", "type": "dev.cdevents.pipelinerun.finished.0.2.0", "timestamp": "2024-01-15T10:25:00Z", "chainId": "chain-1",
		    "links": [{"linkType": "triggeredBy", "linkId": "change-1"}]},
		  "subject": {"id": "build-456", "type": "pipelineRun", "content": {"pipelineName": "build", "outcome": "success"}}}`,
		`{"context": {"id": "deploy-1", "source": "ci", "type": "dev.cdevents.service.deployed.0.1.1", "timestamp": "2024-01-15T10:30:00Z", "chainId": "chain-1",
		    "links": [{"linkType": "triggeredBy", "linkId": "pipeline-1"}]},
		  "subject": {"id": "api", "type": "service", "content": {"artifactId": "api:v1.2.3", "environment": {"id": "production"}}},
		  "customData": {"commitSha": "abc123"}}`,
		`{"context": {"id": "incident-1", "source": "alerts", "type": "dev.cdevents.incident.detected.0.2.0", "timestamp": "2024-01-15T11:00:00Z",
		    "links": [{"linkType": "causedBy", "linkId": "deploy-1"}]},
		  "subject": {"id": "inc-101", "type": "incident", "content": {"description": "High latency detected", "severity": "critical"}}}`,
	}
	for _, body := range events {
		if rec := do(t, h, http.MethodPost, "/api/v1/events", body); rec.Code != http.StatusCreated {
			t.Fatalf("post failed: %s", rec.Body.String())
		}
	}

	rec := do(t, h, http.MethodGet, "/api/v1/chain/incident-1", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var chain models.EventChain
	decode(t, rec, &chain)

	if chain.StartEvent != "incident-1" {
		t.Errorf("StartEvent = %q", chain.StartEvent)
	}
	wantOrder := []string{"incident-1", "deploy-1", "pipeline-1", "change-1"}
	if len(chain.Events) != len(wantOrder) {
		t.Fatalf("expected %d events, got %d: %+v", len(wantOrder), len(chain.Events), chain.Events)
	}
	for i, want := range wantOrder {
		if chain.Events[i].ID != want {
			t.Errorf("position %d: got %s, want %s", i, chain.Events[i].ID, want)
		}
	}
	if chain.RootCause == nil || chain.RootCause.Commit != "abc123" || chain.RootCause.Author != "dev@example.com" || chain.RootCause.Message != "Add feature X" {
		t.Errorf("unexpected root cause: %+v", chain.RootCause)
	}
	if chain.Events[0].Summary != "Incident detected: High latency detected" {
		t.Errorf("unexpected incident summary: %q", chain.Events[0].Summary)
	}

	assertError(t, do(t, h, http.MethodGet, "/api/v1/chain/missing", ""), http.StatusNotFound, "not_found")

	failing := newTestHandler(t, &failingStorage{Storage: storage.NewMemoryStorage(), failGet: true})
	assertError(t, do(t, failing, http.MethodGet, "/api/v1/chain/incident-1", ""), http.StatusInternalServerError, "correlation_error")
}

func TestMetricsEndpoint(t *testing.T) {
	h := newTestHandler(t, nil)
	do(t, h, http.MethodPost, "/api/v1/events", deployedEvent("m1", "api", "production", "v1", "2024-01-15T10:00:00Z"))
	do(t, h, http.MethodPost, "/api/v1/events", `{bad`)

	rec := do(t, h, http.MethodGet, "/metrics", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`cdevents_received_total{type="service.deployed"} 1`,
		`cdevents_rejected_total{reason="parse"} 1`,
		`cdevents_processing_duration_seconds_count 1`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics missing %q", want)
		}
	}
}

func TestCORSPreflight(t *testing.T) {
	rec := do(t, newTestHandler(t, nil), http.MethodOptions, "/api/v1/events", "")
	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("missing CORS header")
	}
}

func TestRecoveryMiddleware(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := RecoveryMiddleware(logger)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("kaboom")
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	assertError(t, rec, http.StatusInternalServerError, "internal_error")
}

func TestLoggingMiddlewareRecordsRoute(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	store := storage.NewMemoryStorage()
	server := NewServer(Options{Storage: store, Logger: logger})
	rec := do(t, server.Handler(), http.MethodGet, "/api/v1/deployments/svc", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("log line is not JSON: %s", buf.String())
	}
	if entry["route"] != "/api/v1/deployments/{service}" || entry["status"] != float64(404) || entry["method"] != "GET" {
		t.Errorf("unexpected log entry: %v", entry)
	}
}

func TestServerStartAndShutdown(t *testing.T) {
	server := NewServer(Options{Host: "127.0.0.1", Port: 0, Storage: storage.NewMemoryStorage(), Tracing: true})
	if server.Addr() != "127.0.0.1:0" {
		t.Errorf("Addr = %q", server.Addr())
	}

	rec := do(t, server.Handler(), http.MethodGet, "/api/v1/health", "")
	if rec.Code != http.StatusOK {
		t.Errorf("traced handler health status = %d", rec.Code)
	}

	if err := server.Shutdown(context.Background()); err != nil {
		t.Errorf("Shutdown before Start should succeed: %v", err)
	}
	if err := server.Start(); err != nil {
		t.Errorf("Start after Shutdown should return nil, got %v", err)
	}
}

func TestRespondJSONEncodingFailure(t *testing.T) {
	rec := httptest.NewRecorder()
	respondJSON(rec, http.StatusOK, map[string]any{"bad": make(chan int)})
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
}
