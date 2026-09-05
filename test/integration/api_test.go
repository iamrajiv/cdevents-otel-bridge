//go:build integration

/*
Integration tests for the CDEvents-OTel Bridge API.

These tests start a real HTTP server (httptest.NewServer) with the full
middleware chain, in-memory storage and correlator, and exercise the
end-to-end flow a CI tool and an observability client would follow: post a
chain of events, query the current deployment, walk the chain back to the
commit, read metrics. The JSON fixtures under test/fixtures are posted as
well so they stay valid inputs.

Build with: go test -tags=integration ./test/integration/...
*/

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iamrajiv/cdevents-otel-bridge/internal/api"
	"github.com/iamrajiv/cdevents-otel-bridge/internal/correlator"
	"github.com/iamrajiv/cdevents-otel-bridge/internal/storage"
	"github.com/iamrajiv/cdevents-otel-bridge/pkg/models"
	"github.com/iamrajiv/cdevents-otel-bridge/pkg/otelbridge"
)

type testServer struct {
	server  *httptest.Server
	storage storage.Storage
}

func setupTestServer(t *testing.T) *testServer {
	t.Helper()

	store := storage.NewMemoryStorage()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	apiServer := api.NewServer(api.Options{
		Host:       "127.0.0.1",
		Port:       0,
		Storage:    store,
		Correlator: correlator.NewCorrelator(store),
		Logger:     logger,
		Version:    "integration",
	})
	srv := httptest.NewServer(apiServer.Handler())
	t.Cleanup(srv.Close)

	return &testServer{server: srv, storage: store}
}

func (ts *testServer) post(t *testing.T, body string) *http.Response {
	t.Helper()
	resp, err := http.Post(ts.server.URL+"/api/v1/events", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("failed to post event: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func (ts *testServer) get(t *testing.T, path string) *http.Response {
	t.Helper()
	resp, err := http.Get(ts.server.URL + path)
	if err != nil {
		t.Fatalf("failed to GET %s: %v", path, err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}
	return string(body)
}

func decodeJSON(t *testing.T, resp *http.Response, into any) {
	t.Helper()
	body := readBody(t, resp)
	if err := json.Unmarshal([]byte(body), into); err != nil {
		t.Fatalf("failed to decode %q: %v", body, err)
	}
}

func fixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "fixtures", name))
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}
	return string(data)
}

func TestFullEventFlow(t *testing.T) {
	ts := setupTestServer(t)

	t.Run("post valid service deployed event", func(t *testing.T) {
		resp := ts.post(t, `{
			"context": {
				"version": "0.4.1",
				"id": "test-event-001",
				"source": "https://github.com/test/app",
				"type": "dev.cdevents.service.deployed.0.1.2",
				"timestamp": "2026-01-29T10:30:00Z",
				"chainId": "chain-test-001"
			},
			"subject": {
				"id": "svc-test-app-production",
				"source": "https://github.com/test/app",
				"type": "service",
				"content": {
					"environment": {"id": "production"},
					"service": {"name": "test-app", "version": "1.0.0"},
					"repository": "https://github.com/test/app",
					"commitSha": "abc123",
					"deployedBy": "test-user"
				}
			}
		}`)

		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("expected status 201, got %d: %s", resp.StatusCode, readBody(t, resp))
		}

		var postResp map[string]any
		decodeJSON(t, resp, &postResp)
		if postResp["status"] != "accepted" || postResp["eventId"] != "test-event-001" {
			t.Errorf("unexpected response: %v", postResp)
		}
	})

	t.Run("get deployment by environment", func(t *testing.T) {
		resp := ts.get(t, "/api/v1/deployments/test-app?environment=production")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", resp.StatusCode, readBody(t, resp))
		}

		var deploymentResp struct {
			Service    string             `json:"service"`
			Deployment *models.Deployment `json:"deployment"`
			Links      map[string]string  `json:"links"`
		}
		decodeJSON(t, resp, &deploymentResp)

		if deploymentResp.Service != "test-app" || deploymentResp.Deployment == nil {
			t.Fatalf("unexpected response: %+v", deploymentResp)
		}
		d := deploymentResp.Deployment
		if d.Service != "test-app" || d.Environment != "production" || d.Version != "1.0.0" || d.CommitSha != "abc123" || d.DeployedBy != "test-user" {
			t.Errorf("unexpected deployment: %+v", d)
		}
		if deploymentResp.Links["chain"] != "/api/v1/chain/test-event-001" {
			t.Errorf("unexpected links: %v", deploymentResp.Links)
		}
	})

	t.Run("get latest deployment without environment", func(t *testing.T) {
		resp := ts.get(t, "/api/v1/deployments/test-app")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", resp.StatusCode, readBody(t, resp))
		}
	})

	t.Run("get event by id", func(t *testing.T) {
		resp := ts.get(t, "/api/v1/events/test-event-001")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", resp.StatusCode, readBody(t, resp))
		}

		var eventResp struct {
			Event map[string]any `json:"event"`
		}
		decodeJSON(t, resp, &eventResp)
		if eventResp.Event["id"] != "test-event-001" {
			t.Errorf("expected event id 'test-event-001', got %v", eventResp.Event["id"])
		}
		if eventResp.Event["summary"] != "Deployed test-app 1.0.0 to production" {
			t.Errorf("unexpected summary %v", eventResp.Event["summary"])
		}
	})

	t.Run("list deployments", func(t *testing.T) {
		resp := ts.get(t, "/api/v1/deployments?environment=production")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", resp.StatusCode, readBody(t, resp))
		}

		var listResp struct {
			Deployments []*models.Deployment `json:"deployments"`
			Total       int                  `json:"total"`
			Limit       int                  `json:"limit"`
		}
		decodeJSON(t, resp, &listResp)

		found := false
		for _, d := range listResp.Deployments {
			if d.Service == "test-app" && d.Environment == "production" {
				found = true
			}
		}
		if !found || listResp.Total < 1 || listResp.Limit != 50 {
			t.Errorf("test-app deployment not found in list: %+v", listResp)
		}
	})

	t.Run("get non-existent deployment returns 404", func(t *testing.T) {
		resp := ts.get(t, "/api/v1/deployments/non-existent-service")
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected status 404, got %d", resp.StatusCode)
		}
	})

	t.Run("post invalid event returns 400", func(t *testing.T) {
		resp := ts.post(t, `{"context": {"id": "invalid-event"}}`)
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected status 400, got %d", resp.StatusCode)
		}
	})

	t.Run("verify event stored in chain", func(t *testing.T) {
		events, err := ts.storage.GetEventsByChain(context.Background(), "chain-test-001")
		if err != nil {
			t.Fatalf("failed to get events by chain: %v", err)
		}
		if len(events) != 1 || events[0].ID != "test-event-001" {
			t.Errorf("unexpected chain events: %v", events)
		}
	})

	t.Run("health check returns 200", func(t *testing.T) {
		resp := ts.get(t, "/api/v1/health")
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got %d", resp.StatusCode)
		}

		var healthResp map[string]any
		decodeJSON(t, resp, &healthResp)
		if healthResp["status"] != "healthy" || healthResp["version"] != "integration" {
			t.Errorf("unexpected health response: %v", healthResp)
		}
	})
}

func TestFixturesAreAccepted(t *testing.T) {
	ts := setupTestServer(t)

	for _, name := range []string{"valid_service_deployed.json", "valid_pipelinerun_finished.json"} {
		resp := ts.post(t, fixture(t, name))
		if resp.StatusCode != http.StatusCreated {
			t.Errorf("%s: expected 201, got %d: %s", name, resp.StatusCode, readBody(t, resp))
		}
	}

	resp := ts.post(t, fixture(t, "invalid_event.json"))
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("invalid_event.json: expected 400, got %d", resp.StatusCode)
	}

	deployment := ts.get(t, "/api/v1/deployments/sample-app")
	if deployment.StatusCode != http.StatusOK {
		t.Fatalf("expected deployment from fixture, got %d: %s", deployment.StatusCode, readBody(t, deployment))
	}
}

func TestIncidentToCommitChain(t *testing.T) {
	ts := setupTestServer(t)

	events := []string{
		`{"context": {"version": "0.4.1", "id": "evt-change", "source": "https://github.com/demo/my-app", "type": "dev.cdevents.change.merged.0.4.1",
			"timestamp": "2024-01-15T10:00:00Z", "chainId": "chain-demo"},
		  "subject": {"id": "pr-42", "type": "change", "content": {"repository": {"id": "github.com/demo/my-app"}}},
		  "customData": {"commitSha": "abc123def456", "author": "developer@example.com", "message": "Add new feature X"}}`,
		`{"context": {"version": "0.4.1", "id": "evt-pipeline", "source": "https://github.com/demo/my-app/actions", "type": "dev.cdevents.pipelinerun.finished.0.2.0",
			"timestamp": "2024-01-15T10:25:00Z", "chainId": "chain-demo", "links": [{"linkType": "triggeredBy", "linkId": "evt-change"}]},
		  "subject": {"id": "build-456", "type": "pipelineRun", "content": {"pipelineName": "build-and-deploy", "outcome": "success"}}}`,
		`{"context": {"version": "0.4.1", "id": "evt-deploy", "source": "https://github.com/demo/my-app/actions", "type": "dev.cdevents.service.deployed.0.1.1",
			"timestamp": "2024-01-15T10:30:00Z", "chainId": "chain-demo", "links": [{"linkType": "triggeredBy", "linkId": "evt-pipeline"}]},
		  "subject": {"id": "my-app", "type": "service", "content": {"artifactId": "my-app:v1.2.3", "environment": {"id": "production"}}},
		  "customData": {"commitSha": "abc123def456", "pipelineId": "build-456", "repository": "github.com/demo/my-app", "deployer": "github-actions"}}`,
		`{"context": {"version": "0.4.1", "id": "evt-incident", "source": "https://alerts.example.com", "type": "dev.cdevents.incident.detected.0.2.0",
			"timestamp": "2024-01-15T11:00:00Z", "chainId": "chain-demo", "links": [{"linkType": "causedBy", "linkId": "evt-deploy"}]},
		  "subject": {"id": "incident-101", "type": "incident", "content": {"description": "High latency detected", "severity": "critical", "service": "my-app"}}}`,
	}

	for _, body := range events {
		resp := ts.post(t, body)
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", resp.StatusCode, readBody(t, resp))
		}
	}

	resp := ts.get(t, "/api/v1/chain/evt-incident")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, readBody(t, resp))
	}

	var chain models.EventChain
	decodeJSON(t, resp, &chain)

	if chain.ChainID != "chain-demo" || chain.StartEvent != "evt-incident" {
		t.Errorf("unexpected chain header: %+v", chain)
	}

	wantOrder := []string{"evt-incident", "evt-deploy", "evt-pipeline", "evt-change"}
	if len(chain.Events) != len(wantOrder) {
		t.Fatalf("expected %d events, got %d", len(wantOrder), len(chain.Events))
	}
	for i, want := range wantOrder {
		if chain.Events[i].ID != want {
			t.Errorf("position %d: got %s, want %s", i, chain.Events[i].ID, want)
		}
	}

	if chain.RootCause == nil {
		t.Fatal("expected root cause")
	}
	if chain.RootCause.Commit != "abc123def456" || chain.RootCause.Author != "developer@example.com" ||
		chain.RootCause.Message != "Add new feature X" || chain.RootCause.Repository != "github.com/demo/my-app" {
		t.Errorf("unexpected root cause: %+v", chain.RootCause)
	}

	summaries := []string{
		"Incident detected: High latency detected",
		"Deployed my-app v1.2.3 to production",
		"Pipeline build-and-deploy finished (success)",
		"Merged change pr-42 by developer@example.com",
	}
	for i, want := range summaries {
		if chain.Events[i].Summary != want {
			t.Errorf("summary %d = %q, want %q", i, chain.Events[i].Summary, want)
		}
	}

	t.Run("chain from the middle still reaches the commit", func(t *testing.T) {
		resp := ts.get(t, "/api/v1/chain/evt-pipeline")
		var partial models.EventChain
		decodeJSON(t, resp, &partial)
		if partial.RootCause == nil || partial.RootCause.Commit != "abc123def456" {
			t.Errorf("expected root cause from pipeline start, got %+v", partial.RootCause)
		}
		if len(partial.Events) != 4 {
			t.Errorf("chainId union should include all 4 events, got %d", len(partial.Events))
		}
	})

	t.Run("otelbridge client resolves the deployment", func(t *testing.T) {
		client := otelbridge.NewClient(ts.server.URL, otelbridge.WithEnvironment("production"))
		d, err := client.ResolveDeployment(context.Background(), "my-app")
		if err != nil {
			t.Fatalf("client failed: %v", err)
		}
		if d.Version != "v1.2.3" || d.CommitSha != "abc123def456" || d.PipelineID != "build-456" {
			t.Errorf("unexpected deployment via client: %+v", d)
		}
	})

	t.Run("metrics reflect the ingested events", func(t *testing.T) {
		body := readBody(t, ts.get(t, "/metrics"))
		for _, want := range []string{
			`cdevents_received_total{type="change.merged"} 1`,
			`cdevents_received_total{type="incident.detected"} 1`,
			`cdevents_received_total{type="pipelinerun.finished"} 1`,
			`cdevents_received_total{type="service.deployed"} 1`,
		} {
			if !strings.Contains(body, want) {
				t.Errorf("metrics missing %q", want)
			}
		}
	})
}

func TestLegacyLinkShapeStillBuildsChain(t *testing.T) {
	ts := setupTestServer(t)

	ts.post(t, `{"context": {"id": "old-change", "source": "s", "type": "dev.cdevents.change.merged.0.4.1", "timestamp": "2024-01-15T10:00:00Z"},
		"subject": {"id": "pr", "type": "change"}, "customData": {"commitSha": "legacy123"}}`)
	resp := ts.post(t, `{"context": {"id": "old-deploy", "source": "s", "type": "dev.cdevents.service.deployed.0.1.1", "timestamp": "2024-01-15T10:30:00Z"},
		"subject": {"id": "svc", "type": "service"}, "links": [{"type": "TRIGGERED_BY", "target": "old-change"}]}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, readBody(t, resp))
	}

	var chain models.EventChain
	decodeJSON(t, ts.get(t, "/api/v1/chain/old-deploy"), &chain)
	if len(chain.Events) != 2 || chain.RootCause == nil || chain.RootCause.Commit != "legacy123" {
		t.Errorf("legacy links should still build a chain: %+v", chain)
	}
}

func TestRequestIDHeaderIsNotRequired(t *testing.T) {
	ts := setupTestServer(t)
	resp := ts.get(t, fmt.Sprintf("/api/v1/events/%s", "missing"))
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}
