/*
Unit tests for CDEvent parsing and model conversion.

Tests cover parsing JSON into CDEvent structures (including the different
link shapes producers send), converting events to Deployment, PipelineRun and
Incident models from the various field layouts seen in the wild, and the
summary and change-info helpers used by storage and the correlator. The JSON
fixtures under test/fixtures are parsed as well so the fixtures and the parser
cannot drift apart.
*/
package cdevents

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/iamrajiv/cdevents-otel-bridge/pkg/models"
)

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "test", "fixtures", name))
	if err != nil {
		t.Fatalf("failed to read fixture %s: %v", name, err)
	}
	return data
}

func mustParse(t *testing.T, input string) *CDEvent {
	t.Helper()
	event, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse() unexpected error: %v", err)
	}
	return event
}

func TestParse(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantErr   bool
		wantLinks []models.Link
	}{
		{
			name: "valid service.deployed event",
			input: `{
				"context": {
					"version": "0.4.0",
					"id": "event-123",
					"source": "github.com/myorg/myrepo",
					"type": "dev.cdevents.service.deployed.0.1.0",
					"timestamp": "2024-01-15T10:30:00Z"
				},
				"subject": {"id": "api-service", "type": "service"}
			}`,
		},
		{
			name:    "invalid JSON",
			input:   `{"context": {invalid json`,
			wantErr: true,
		},
		{
			name:    "malformed timestamp",
			input:   `{"context": {"id": "x", "timestamp": "yesterday"}, "subject": {"id": "s", "type": "service"}}`,
			wantErr: true,
		},
		{
			name: "context links in spec shape",
			input: `{
				"context": {"id": "e", "source": "s", "type": "dev.cdevents.service.deployed.0.1.0",
					"timestamp": "2024-01-15T10:30:00Z",
					"links": [{"linkType": "triggeredBy", "linkId": "pipeline-1"}]},
				"subject": {"id": "svc", "type": "service"}
			}`,
			wantLinks: []models.Link{{LinkType: "triggeredBy", LinkID: "pipeline-1"}},
		},
		{
			name: "top-level links in legacy shape are merged into context",
			input: `{
				"context": {"id": "e", "source": "s", "type": "dev.cdevents.service.deployed.0.1.0",
					"timestamp": "2024-01-15T10:30:00Z"},
				"subject": {"id": "svc", "type": "service"},
				"links": [{"type": "TRIGGERED_BY", "target": "pipeline-1"}]
			}`,
			wantLinks: []models.Link{{LinkType: "TRIGGERED_BY", LinkID: "pipeline-1"}},
		},
		{
			name: "cdevents native link shape",
			input: `{
				"context": {"id": "e", "source": "s", "type": "dev.cdevents.service.deployed.0.1.0",
					"timestamp": "2024-01-15T10:30:00Z",
					"links": [{"linkType": "RELATION", "linkKind": "TRIGGER", "from": {"contextId": "pipeline-1"}}]},
				"subject": {"id": "svc", "type": "service"}
			}`,
			wantLinks: []models.Link{{LinkType: "TRIGGER", LinkID: "pipeline-1"}},
		},
		{
			name: "duplicate and empty links are dropped",
			input: `{
				"context": {"id": "e", "source": "s", "type": "dev.cdevents.service.deployed.0.1.0",
					"timestamp": "2024-01-15T10:30:00Z",
					"links": [{"linkType": "a", "linkId": "x"}, {"linkType": "a"}]},
				"subject": {"id": "svc", "type": "service"},
				"links": [{"linkType": "a", "linkId": "x"}, {"linkType": "b", "linkId": "y"}]
			}`,
			wantLinks: []models.Link{{LinkType: "a", LinkID: "x"}, {LinkType: "b", LinkID: "y"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event, err := Parse([]byte(tt.input))
			if tt.wantErr {
				if err == nil {
					t.Fatal("Parse() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse() unexpected error: %v", err)
			}
			if event == nil {
				t.Fatal("Parse() returned nil event")
			}
			if event.Links != nil {
				t.Error("top-level links should be cleared after normalization")
			}
			if len(tt.wantLinks) == 0 && len(event.Context.Links) != 0 {
				t.Errorf("expected no links, got %v", event.Context.Links)
			}
			if len(tt.wantLinks) != len(event.Context.Links) {
				t.Fatalf("expected %d links, got %d: %v", len(tt.wantLinks), len(event.Context.Links), event.Context.Links)
			}
			for i, want := range tt.wantLinks {
				if event.Context.Links[i] != want {
					t.Errorf("link %d: got %+v, want %+v", i, event.Context.Links[i], want)
				}
			}
		})
	}
}

func TestParseDoesNotValidate(t *testing.T) {
	event := mustParse(t, `{"context": {"id": "only-id"}, "subject": {}}`)
	if err := Validate(event); err == nil {
		t.Error("Validate() should reject an event that Parse() accepted without required fields")
	}
}

func TestToDeployment(t *testing.T) {
	timestamp, _ := time.Parse(time.RFC3339, "2024-01-15T10:30:00Z")

	t.Run("nil event", func(t *testing.T) {
		if ToDeployment(nil) != nil {
			t.Error("ToDeployment should return nil for nil event")
		}
	})

	t.Run("customData layout", func(t *testing.T) {
		event := &CDEvent{
			Context: EventContext{
				ID:        "event-123",
				Type:      "dev.cdevents.service.deployed.0.1.0",
				Source:    "github.com/myorg/myrepo",
				Timestamp: timestamp,
				ChainID:   "chain-abc",
				Links:     []models.Link{{LinkType: "triggeredBy", LinkID: "pipeline-789"}},
			},
			Subject: EventSubject{
				ID:   "api-service",
				Type: "service",
				Content: map[string]any{
					"artifactId":  "myorg/api-service:v1.2.3",
					"environment": map[string]any{"id": "production"},
				},
			},
			CustomData: map[string]any{
				"commitSha":  "abc123def456",
				"repository": "github.com/myorg/myrepo",
				"author":     "john.doe@example.com",
				"deployer":   "deploy-bot",
				"pipelineId": "pipeline-789",
				"branch":     "main",
			},
		}

		d := ToDeployment(event)
		want := &models.Deployment{
			ID:          "api-service",
			Service:     "api-service",
			Version:     "v1.2.3",
			Environment: "production",
			CommitSha:   "abc123def456",
			Repository:  "github.com/myorg/myrepo",
			Branch:      "main",
			Author:      "john.doe@example.com",
			PipelineID:  "pipeline-789",
			DeployedAt:  timestamp,
			DeployedBy:  "deploy-bot",
			EventID:     "event-123",
			ChainID:     "chain-abc",
			Links:       event.Context.Links,
		}
		assertDeployment(t, d, want)
	})

	t.Run("subject.content layout from fixture", func(t *testing.T) {
		event, err := Parse(loadFixture(t, "valid_service_deployed.json"))
		if err != nil {
			t.Fatalf("Parse() fixture: %v", err)
		}
		if err := Validate(event); err != nil {
			t.Fatalf("fixture should be valid: %v", err)
		}

		d := ToDeployment(event)
		if d.Service != "sample-app" {
			t.Errorf("Service = %q, want sample-app", d.Service)
		}
		if d.Version != "1.2.3" {
			t.Errorf("Version = %q, want 1.2.3", d.Version)
		}
		if d.Environment != "production" {
			t.Errorf("Environment = %q, want production", d.Environment)
		}
		if d.CommitSha != "a1b2c3d4e5f6789012345678901234567890abcd" {
			t.Errorf("CommitSha = %q", d.CommitSha)
		}
		if d.Repository != "https://github.com/iamrajiv/sample-app" {
			t.Errorf("Repository = %q", d.Repository)
		}
		if d.Branch != "main" {
			t.Errorf("Branch = %q, want main", d.Branch)
		}
		if d.PipelineID != "pipeline-123" {
			t.Errorf("PipelineID = %q, want pipeline-123", d.PipelineID)
		}
		if d.PipelineURL != "https://github.com/iamrajiv/sample-app/actions/runs/123456" {
			t.Errorf("PipelineURL = %q", d.PipelineURL)
		}
		if d.DeployedBy != "github-actions" {
			t.Errorf("DeployedBy = %q, want github-actions", d.DeployedBy)
		}
		if d.ID != "svc-sample-app-production" {
			t.Errorf("ID = %q, want subject id", d.ID)
		}
		if d.ChainID != "chain-12345" {
			t.Errorf("ChainID = %q, want chain-12345", d.ChainID)
		}
	})

	t.Run("repository as object and purl artifact", func(t *testing.T) {
		event := mustParse(t, `{
			"context": {"id": "e", "source": "s", "type": "dev.cdevents.service.deployed.0.1.0",
				"timestamp": "2024-01-15T10:30:00Z"},
			"subject": {"id": "svc", "type": "service", "content": {
				"artifactId": "pkg:docker/myapp@sha256:abc123",
				"environment": "staging",
				"repository": {"id": "github.com/org/repo", "url": "https://github.com/org/repo"}
			}}
		}`)
		d := ToDeployment(event)
		if d.Version != "sha256:abc123" {
			t.Errorf("Version = %q", d.Version)
		}
		if d.Environment != "staging" {
			t.Errorf("Environment = %q, want staging", d.Environment)
		}
		if d.Repository != "github.com/org/repo" {
			t.Errorf("Repository = %q", d.Repository)
		}
	})
}

func assertDeployment(t *testing.T, got, want *models.Deployment) {
	t.Helper()
	if got == nil {
		t.Fatal("ToDeployment returned nil")
	}
	gotJSON, _ := json.Marshal(got)
	wantJSON, _ := json.Marshal(want)
	if string(gotJSON) != string(wantJSON) {
		t.Errorf("deployment mismatch\n got: %s\nwant: %s", gotJSON, wantJSON)
	}
}

func TestToPipelineRun(t *testing.T) {
	event, err := Parse(loadFixture(t, "valid_pipelinerun_finished.json"))
	if err != nil {
		t.Fatalf("Parse() fixture: %v", err)
	}

	run := ToPipelineRun(event)
	if run.ID != "pipelinerun-98765" {
		t.Errorf("ID = %q", run.ID)
	}
	if run.PipelineName != "ci-cd-pipeline" {
		t.Errorf("PipelineName = %q", run.PipelineName)
	}
	if run.Status != "finished" {
		t.Errorf("Status = %q, want finished", run.Status)
	}
	if run.Outcome != "success" {
		t.Errorf("Outcome = %q, want success", run.Outcome)
	}
	if run.CommitSha != "a1b2c3d4e5f6789012345678901234567890abcd" {
		t.Errorf("CommitSha = %q", run.CommitSha)
	}
	if run.StartedAt.Format(time.RFC3339) != "2026-01-29T10:20:00Z" {
		t.Errorf("StartedAt = %s", run.StartedAt)
	}
	if run.FinishedAt == nil || run.FinishedAt.Format(time.RFC3339) != "2026-01-29T10:25:00Z" {
		t.Errorf("FinishedAt = %v", run.FinishedAt)
	}
	if d := run.Duration(); d == nil || *d != 5*time.Minute {
		t.Errorf("Duration = %v, want 5m", d)
	}
	if len(run.Links) != 1 || run.Links[0].LinkID != "evt-11111" {
		t.Errorf("Links = %v", run.Links)
	}
}

func TestToIncident(t *testing.T) {
	event := mustParse(t, `{
		"context": {"id": "inc-evt", "source": "alertmanager", "type": "dev.cdevents.incident.detected.0.2.0",
			"timestamp": "2024-01-15T11:00:00Z", "chainId": "chain-1",
			"links": [{"linkType": "causedBy", "linkId": "deploy-evt"}]},
		"subject": {"id": "incident-101", "type": "incident", "content": {
			"description": "High latency detected",
			"severity": "critical",
			"service": {"name": "my-app"},
			"environment": {"id": "production"}
		}}
	}`)

	inc := ToIncident(event)
	if inc.ID != "incident-101" || inc.Status != "detected" {
		t.Errorf("ID/Status = %q/%q", inc.ID, inc.Status)
	}
	if inc.Service != "my-app" || inc.Environment != "production" {
		t.Errorf("Service/Environment = %q/%q", inc.Service, inc.Environment)
	}
	if inc.Severity != "critical" || inc.Description != "High latency detected" {
		t.Errorf("Severity/Description = %q/%q", inc.Severity, inc.Description)
	}
	if inc.ResolvedAt != nil || inc.Duration() != nil {
		t.Error("detected incident should not have a resolution time")
	}

	resolvedEvent := mustParse(t, `{
		"context": {"id": "inc-res", "source": "alertmanager", "type": "dev.cdevents.incident.resolved.0.2.0",
			"timestamp": "2024-01-15T11:30:00Z"},
		"subject": {"id": "incident-101", "type": "incident", "content": {"service": "my-app"}}
	}`)
	resolved := ToIncident(resolvedEvent)
	if resolved.Status != "resolved" || resolved.ResolvedAt == nil {
		t.Errorf("resolved incident: status=%q resolvedAt=%v", resolved.Status, resolved.ResolvedAt)
	}
	if resolved.Service != "my-app" {
		t.Errorf("Service = %q, want my-app", resolved.Service)
	}
}

func TestToStoredEvent(t *testing.T) {
	timestamp, _ := time.Parse(time.RFC3339, "2024-01-15T10:30:00Z")

	t.Run("nil event", func(t *testing.T) {
		if ToStoredEvent(nil) != nil {
			t.Error("ToStoredEvent should return nil for nil event")
		}
	})

	t.Run("event with content and customData", func(t *testing.T) {
		event := &CDEvent{
			Context: EventContext{
				ID: "event-123", Type: "dev.cdevents.service.deployed.0.1.0",
				Source: "github.com/myorg/myrepo", Timestamp: timestamp, ChainID: "chain-abc",
			},
			Subject: EventSubject{
				ID: "api-service", Type: "service",
				Content: map[string]any{"artifactId": "myorg/api-service:v1.2.3"},
			},
			CustomData: map[string]any{"commitSha": "abc123"},
		}

		result := ToStoredEvent(event)
		if result.ID != "event-123" || result.Type != event.Context.Type {
			t.Errorf("ID/Type mismatch: %+v", result)
		}
		if result.SubjectID != "api-service" || result.SubjectType != "service" {
			t.Errorf("subject mismatch: %+v", result)
		}
		if result.ChainID != "chain-abc" {
			t.Errorf("ChainID = %q", result.ChainID)
		}
		if result.Summary != "Deployed api-service v1.2.3" {
			t.Errorf("Summary = %q", result.Summary)
		}
		if result.RawData["customData"] == nil || result.RawData["content"] == nil {
			t.Errorf("RawData should carry content and customData: %v", result.RawData)
		}
	})

	t.Run("event without data has nil rawData", func(t *testing.T) {
		result := ToStoredEvent(&CDEvent{Subject: EventSubject{ID: "x", Type: "service"}})
		if result.RawData != nil {
			t.Errorf("RawData should be nil, got %v", result.RawData)
		}
	})
}

func TestExtractChangeInfo(t *testing.T) {
	tests := []struct {
		name  string
		event *StoredEvent
		want  ChangeInfo
	}{
		{
			name:  "nil event",
			event: nil,
			want:  ChangeInfo{},
		},
		{
			name: "customData layout",
			event: &StoredEvent{RawData: map[string]any{
				"customData": map[string]any{
					"commitSha": "abc123", "author": "dev@example.com",
					"message": "Add feature", "repository": "github.com/org/repo",
				},
			}},
			want: ChangeInfo{Commit: "abc123", Author: "dev@example.com", Message: "Add feature", Repository: "github.com/org/repo"},
		},
		{
			name: "content layout with repository object",
			event: &StoredEvent{RawData: map[string]any{
				"content": map[string]any{
					"commit":      "def456",
					"repository":  map[string]any{"id": "github.com/org/repo"},
					"description": "Fix bug",
				},
			}},
			want: ChangeInfo{Commit: "def456", Message: "Fix bug", Repository: "github.com/org/repo"},
		},
		{
			name: "flat layout",
			event: &StoredEvent{RawData: map[string]any{
				"commit": "789", "author": "a", "message": "m", "repository": "r",
			}},
			want: ChangeInfo{Commit: "789", Author: "a", Message: "m", Repository: "r"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExtractChangeInfo(tt.event); got != tt.want {
				t.Errorf("ExtractChangeInfo() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestSummarize(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name: "deployment with version and environment",
			input: `{"context": {"type": "dev.cdevents.service.deployed.0.1.0"},
				"subject": {"id": "api", "type": "service", "content": {"artifactId": "api:v2", "environment": {"id": "prod"}}}}`,
			want: "Deployed api v2 to prod",
		},
		{
			name:  "rollback",
			input: `{"context": {"type": "dev.cdevents.service.rolledback.0.1.0"}, "subject": {"id": "api", "type": "service"}}`,
			want:  "Rolled back api",
		},
		{
			name: "pipeline finished with outcome",
			input: `{"context": {"type": "dev.cdevents.pipelinerun.finished.0.2.0"},
				"subject": {"id": "run-1", "type": "pipelineRun", "content": {"pipelineName": "build", "outcome": "success"}}}`,
			want: "Pipeline build finished (success)",
		},
		{
			name:  "change merged with author",
			input: `{"context": {"type": "dev.cdevents.change.merged.0.4.1"}, "subject": {"id": "pr-12", "type": "change"}, "customData": {"author": "dev"}}`,
			want:  "Merged change pr-12 by dev",
		},
		{
			name:  "incident detected",
			input: `{"context": {"type": "dev.cdevents.incident.detected.0.2.0"}, "subject": {"id": "inc-1", "type": "incident", "content": {"description": "latency"}}}`,
			want:  "Incident detected: latency",
		},
		{
			name:  "generic event",
			input: `{"context": {"type": "dev.cdevents.artifact.published.0.1.1"}, "subject": {"id": "pkg", "type": "artifact"}}`,
			want:  "artifact published: pkg",
		},
		{
			name:  "generic event without subject id",
			input: `{"context": {"type": "dev.cdevents.taskrun.started.0.1.0"}, "subject": {"type": "taskRun"}}`,
			want:  "taskrun started",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Summarize(mustParse(t, tt.input)); got != tt.want {
				t.Errorf("Summarize() = %q, want %q", got, tt.want)
			}
		})
	}

	if Summarize(nil) != "" {
		t.Error("Summarize(nil) should be empty")
	}
}

func TestParseRoundTrip(t *testing.T) {
	original := &CDEvent{
		Context: EventContext{
			Version:   "0.4.0",
			ID:        "event-123",
			Source:    "github.com/myorg/myrepo",
			Type:      "dev.cdevents.service.deployed.0.1.0",
			Timestamp: time.Now().UTC().Truncate(time.Second),
			Links:     []models.Link{{LinkType: "triggeredBy", LinkID: "p-1"}},
		},
		Subject: EventSubject{ID: "api-service", Type: "service"},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Failed to marshal event: %v", err)
	}

	parsed, err := Parse(data)
	if err != nil {
		t.Fatalf("Failed to parse event: %v", err)
	}

	if parsed.Context.ID != original.Context.ID || parsed.Context.Type != original.Context.Type {
		t.Errorf("context mismatch: %+v", parsed.Context)
	}
	if len(parsed.Context.Links) != 1 || parsed.Context.Links[0] != original.Context.Links[0] {
		t.Errorf("links did not survive round trip: %v", parsed.Context.Links)
	}
}
