/*
Unit tests for the Correlator.

Uses the real in-memory storage backend so chain reconstruction is exercised
the way the API uses it: events are saved, then BuildChain is asked to
reconstruct the chain from the newest event. Cases cover link traversal,
chainId union, ordering, root-cause selection and fallback, dangling links
and error propagation.
*/
package correlator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/iamrajiv/cdevents-otel-bridge/internal/cdevents"
	"github.com/iamrajiv/cdevents-otel-bridge/internal/storage"
	"github.com/iamrajiv/cdevents-otel-bridge/pkg/models"
)

var base = time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)

func newCorrelatorWith(t *testing.T, events ...*cdevents.StoredEvent) *Correlator {
	t.Helper()
	store := storage.NewMemoryStorage()
	for _, event := range events {
		if err := store.SaveEvent(context.Background(), event); err != nil {
			t.Fatalf("failed to save event: %v", err)
		}
	}
	return NewCorrelator(store)
}

func links(ids ...string) []models.Link {
	result := make([]models.Link, 0, len(ids))
	for _, id := range ids {
		result = append(result, models.Link{LinkType: "triggeredBy", LinkID: id})
	}
	return result
}

func eventIDs(chain *models.EventChain) []string {
	result := make([]string, 0, len(chain.Events))
	for _, e := range chain.Events {
		result = append(result, e.ID)
	}
	return result
}

func assertOrder(t *testing.T, chain *models.EventChain, want ...string) {
	t.Helper()
	got := eventIDs(chain)
	if len(got) != len(want) {
		t.Fatalf("chain has %d events %v, want %d %v", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("position %d: got %s, want %s (full order %v)", i, got[i], want[i], got)
		}
	}
}

func TestBuildChainFollowsLinksNewestFirst(t *testing.T) {
	c := newCorrelatorWith(t,
		&cdevents.StoredEvent{ID: "change", Type: "dev.cdevents.change.merged.0.4.1", Timestamp: base,
			RawData: map[string]any{"customData": map[string]any{"commitSha": "abc123", "author": "dev@example.com", "message": "Add feature", "repository": "github.com/org/repo"}}},
		&cdevents.StoredEvent{ID: "pipeline", Type: "dev.cdevents.pipelinerun.finished.0.2.0", Timestamp: base.Add(5 * time.Minute), Links: links("change")},
		&cdevents.StoredEvent{ID: "deploy", Type: "dev.cdevents.service.deployed.0.1.1", Timestamp: base.Add(10 * time.Minute), Links: links("pipeline"),
			RawData: map[string]any{"customData": map[string]any{"commitSha": "abc123"}}},
		&cdevents.StoredEvent{ID: "incident", Type: "dev.cdevents.incident.detected.0.2.0", Timestamp: base.Add(30 * time.Minute), Links: []models.Link{{LinkType: "causedBy", LinkID: "deploy"}}},
		&cdevents.StoredEvent{ID: "unrelated", Type: "dev.cdevents.service.deployed.0.1.1", Timestamp: base.Add(time.Hour)},
	)

	chain, err := c.BuildChain(context.Background(), "incident")
	if err != nil {
		t.Fatalf("BuildChain failed: %v", err)
	}

	assertOrder(t, chain, "incident", "deploy", "pipeline", "change")

	if chain.StartEvent != "incident" {
		t.Errorf("StartEvent = %q", chain.StartEvent)
	}
	if chain.RootCause == nil {
		t.Fatal("expected root cause")
	}
	if chain.RootCause.Commit != "abc123" || chain.RootCause.Author != "dev@example.com" ||
		chain.RootCause.Message != "Add feature" || chain.RootCause.Repository != "github.com/org/repo" {
		t.Errorf("unexpected root cause: %+v", chain.RootCause)
	}

	if chain.Events[3].Commit != "abc123" {
		t.Errorf("change event should expose its commit, got %q", chain.Events[3].Commit)
	}
	if chain.Events[1].Commit != "abc123" {
		t.Errorf("deploy event should expose its commit, got %q", chain.Events[1].Commit)
	}
	if chain.Events[0].Commit != "" {
		t.Errorf("incident event should not expose a commit, got %q", chain.Events[0].Commit)
	}
}

func TestBuildChainUnionsChainID(t *testing.T) {
	c := newCorrelatorWith(t,
		&cdevents.StoredEvent{ID: "a", Type: "dev.cdevents.change.merged.0.4.1", Timestamp: base, ChainID: "chain-1"},
		&cdevents.StoredEvent{ID: "b", Type: "dev.cdevents.pipelinerun.finished.0.2.0", Timestamp: base.Add(time.Minute), ChainID: "chain-1"},
		&cdevents.StoredEvent{ID: "c", Type: "dev.cdevents.service.deployed.0.1.1", Timestamp: base.Add(2 * time.Minute), ChainID: "chain-1"},
		&cdevents.StoredEvent{ID: "d", Type: "dev.cdevents.service.deployed.0.1.1", Timestamp: base.Add(3 * time.Minute), ChainID: "chain-2"},
	)

	chain, err := c.BuildChain(context.Background(), "c")
	if err != nil {
		t.Fatalf("BuildChain failed: %v", err)
	}

	assertOrder(t, chain, "c", "b", "a")
	if chain.ChainID != "chain-1" {
		t.Errorf("ChainID = %q, want chain-1", chain.ChainID)
	}
	if chain.RootCause != nil {
		t.Errorf("expected no root cause without commits, got %+v", chain.RootCause)
	}
}

func TestBuildChainLinksAndChainIDDoNotDuplicate(t *testing.T) {
	c := newCorrelatorWith(t,
		&cdevents.StoredEvent{ID: "a", Type: "dev.cdevents.change.merged.0.4.1", Timestamp: base, ChainID: "chain-1"},
		&cdevents.StoredEvent{ID: "b", Type: "dev.cdevents.service.deployed.0.1.1", Timestamp: base.Add(time.Minute), ChainID: "chain-1", Links: links("a")},
	)

	chain, err := c.BuildChain(context.Background(), "b")
	if err != nil {
		t.Fatal(err)
	}
	assertOrder(t, chain, "b", "a")
}

func TestBuildChainSingleEvent(t *testing.T) {
	c := newCorrelatorWith(t, &cdevents.StoredEvent{ID: "only", Type: "dev.cdevents.change.created.0.1.0", Timestamp: base, ChainID: "chain-2"})

	chain, err := c.BuildChain(context.Background(), "only")
	if err != nil {
		t.Fatal(err)
	}
	assertOrder(t, chain, "only")
	if chain.ChainID != "chain-2" {
		t.Errorf("ChainID = %q", chain.ChainID)
	}
}

func TestBuildChainNotFound(t *testing.T) {
	c := newCorrelatorWith(t)

	_, err := c.BuildChain(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestBuildChainSkipsDanglingLinks(t *testing.T) {
	c := newCorrelatorWith(t,
		&cdevents.StoredEvent{ID: "deploy", Type: "dev.cdevents.service.deployed.0.1.1", Timestamp: base, Links: links("never-received")},
	)

	chain, err := c.BuildChain(context.Background(), "deploy")
	if err != nil {
		t.Fatalf("dangling link should not fail: %v", err)
	}
	assertOrder(t, chain, "deploy")
}

func TestBuildChainHandlesCycles(t *testing.T) {
	c := newCorrelatorWith(t,
		&cdevents.StoredEvent{ID: "a", Type: "dev.cdevents.change.merged.0.4.1", Timestamp: base, Links: links("b")},
		&cdevents.StoredEvent{ID: "b", Type: "dev.cdevents.pipelinerun.finished.0.2.0", Timestamp: base.Add(time.Minute), Links: links("a", "b")},
	)

	chain, err := c.BuildChain(context.Background(), "a")
	if err != nil {
		t.Fatal(err)
	}
	assertOrder(t, chain, "b", "a")
}

func TestBuildChainBranching(t *testing.T) {
	c := newCorrelatorWith(t,
		&cdevents.StoredEvent{ID: "change-1", Type: "dev.cdevents.change.merged.0.4.1", Timestamp: base},
		&cdevents.StoredEvent{ID: "change-2", Type: "dev.cdevents.change.merged.0.4.1", Timestamp: base.Add(time.Minute)},
		&cdevents.StoredEvent{ID: "deploy", Type: "dev.cdevents.service.deployed.0.1.1", Timestamp: base.Add(2 * time.Minute), Links: links("change-1", "change-2")},
	)

	chain, err := c.BuildChain(context.Background(), "deploy")
	if err != nil {
		t.Fatal(err)
	}
	assertOrder(t, chain, "deploy", "change-2", "change-1")
}

func TestFindRootCause(t *testing.T) {
	tests := []struct {
		name   string
		events []*cdevents.StoredEvent
		want   *models.RootCause
	}{
		{
			name:   "no events",
			events: nil,
			want:   nil,
		},
		{
			name: "earliest change.merged with commit wins",
			events: []*cdevents.StoredEvent{
				{ID: "newer", Type: "dev.cdevents.change.merged.0.4.1", RawData: map[string]any{"customData": map[string]any{"commitSha": "new"}}},
				{ID: "older", Type: "dev.cdevents.change.merged.0.4.1", RawData: map[string]any{"customData": map[string]any{"commitSha": "old", "author": "a"}}},
			},
			want: &models.RootCause{Commit: "old", Author: "a"},
		},
		{
			name: "change.merged without commit is skipped",
			events: []*cdevents.StoredEvent{
				{ID: "deploy", Type: "dev.cdevents.service.deployed.0.1.1", RawData: map[string]any{"customData": map[string]any{"commitSha": "dep", "repository": "r"}}},
				{ID: "change", Type: "dev.cdevents.change.merged.0.4.1", RawData: map[string]any{"customData": map[string]any{"author": "a"}}},
			},
			want: &models.RootCause{Commit: "dep", Repository: "r"},
		},
		{
			name: "falls back to deployment commit",
			events: []*cdevents.StoredEvent{
				{ID: "incident", Type: "dev.cdevents.incident.detected.0.2.0", RawData: map[string]any{"customData": map[string]any{"commitSha": "ignored"}}},
				{ID: "deploy", Type: "dev.cdevents.service.deployed.0.1.1", RawData: map[string]any{"content": map[string]any{"commitSha": "dep123", "repository": "github.com/o/r"}}},
			},
			want: &models.RootCause{Commit: "dep123", Repository: "github.com/o/r"},
		},
		{
			name: "flat rawData layout",
			events: []*cdevents.StoredEvent{
				{ID: "change", Type: "dev.cdevents.change.merged.0.1.0", RawData: map[string]any{"commit": "abc", "author": "john", "message": "Fix", "repository": "repo"}},
			},
			want: &models.RootCause{Commit: "abc", Author: "john", Message: "Fix", Repository: "repo"},
		},
		{
			name: "no commit anywhere",
			events: []*cdevents.StoredEvent{
				{ID: "build", Type: "dev.cdevents.build.started.0.1.0"},
				{ID: "deploy", Type: "dev.cdevents.service.deployed.0.1.1"},
			},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FindRootCause(tt.events)
			if (got == nil) != (tt.want == nil) {
				t.Fatalf("FindRootCause() = %+v, want %+v", got, tt.want)
			}
			if got != nil && *got != *tt.want {
				t.Errorf("FindRootCause() = %+v, want %+v", *got, *tt.want)
			}
		})
	}
}

type failingStorage struct {
	storage.Storage
	failOn string
}

func (f *failingStorage) GetEvent(ctx context.Context, id string) (*cdevents.StoredEvent, error) {
	if id == f.failOn {
		return nil, errors.New("boom")
	}
	return f.Storage.GetEvent(ctx, id)
}

func (f *failingStorage) GetEventsByChain(ctx context.Context, chainID string) ([]*cdevents.StoredEvent, error) {
	if f.failOn == "chain:"+chainID {
		return nil, errors.New("chain boom")
	}
	return f.Storage.GetEventsByChain(ctx, chainID)
}

func TestBuildChainPropagatesStorageErrors(t *testing.T) {
	store := storage.NewMemoryStorage()
	ctx := context.Background()
	_ = store.SaveEvent(ctx, &cdevents.StoredEvent{ID: "a", Type: "dev.cdevents.service.deployed.0.1.1", Timestamp: base, ChainID: "c", Links: links("b")})
	_ = store.SaveEvent(ctx, &cdevents.StoredEvent{ID: "b", Type: "dev.cdevents.change.merged.0.4.1", Timestamp: base})

	linkFail := NewCorrelator(&failingStorage{Storage: store, failOn: "b"})
	if _, err := linkFail.BuildChain(ctx, "a"); err == nil || errors.Is(err, storage.ErrNotFound) {
		t.Errorf("expected traversal error, got %v", err)
	}

	chainFail := NewCorrelator(&failingStorage{Storage: store, failOn: "chain:c"})
	if _, err := chainFail.BuildChain(ctx, "a"); err == nil {
		t.Error("expected chain lookup error")
	}
}
