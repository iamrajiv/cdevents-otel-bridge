/*
Unit tests for EventGraph traversal limits and helpers.
*/
package correlator

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/iamrajiv/cdevents-otel-bridge/internal/cdevents"
	"github.com/iamrajiv/cdevents-otel-bridge/internal/storage"
	"github.com/iamrajiv/cdevents-otel-bridge/pkg/models"
)

func TestTraverseChainRespectsDepthLimit(t *testing.T) {
	store := storage.NewMemoryStorage()
	ctx := context.Background()

	const length = 10
	for i := 0; i < length; i++ {
		event := &cdevents.StoredEvent{ID: fmt.Sprintf("e%d", i), Timestamp: base.Add(time.Duration(i) * time.Minute)}
		if i > 0 {
			event.Links = links(fmt.Sprintf("e%d", i-1))
		}
		if err := store.SaveEvent(ctx, event); err != nil {
			t.Fatal(err)
		}
	}

	graph := NewEventGraph(store)
	graph.maxDepth = 3

	events, err := graph.TraverseChain(ctx, "e9")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 4 {
		t.Errorf("expected 4 events (depth 0..3), got %d: %v", len(events), storageIDs(events))
	}
}

func TestTraverseChainRespectsEventLimit(t *testing.T) {
	store := storage.NewMemoryStorage()
	ctx := context.Background()

	root := &cdevents.StoredEvent{ID: "root", Timestamp: base}
	for i := 0; i < 20; i++ {
		id := fmt.Sprintf("leaf%d", i)
		root.Links = append(root.Links, models.Link{LinkType: "relatedTo", LinkID: id})
		if err := store.SaveEvent(ctx, &cdevents.StoredEvent{ID: id, Timestamp: base}); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.SaveEvent(ctx, root); err != nil {
		t.Fatal(err)
	}

	graph := NewEventGraph(store)
	graph.maxEvents = 5

	events, err := graph.TraverseChain(ctx, "root")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 5 {
		t.Errorf("expected 5 events, got %d", len(events))
	}
}

func TestTraverseChainUnknownStart(t *testing.T) {
	graph := NewEventGraph(storage.NewMemoryStorage())

	events, err := graph.TraverseChain(context.Background(), "missing")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected no events, got %d", len(events))
	}
}

func TestLinkedEventIDs(t *testing.T) {
	graph := NewEventGraph(storage.NewMemoryStorage())

	if got := graph.LinkedEventIDs(nil); got != nil {
		t.Errorf("nil event should give nil, got %v", got)
	}
	if got := graph.LinkedEventIDs(&cdevents.StoredEvent{}); got != nil {
		t.Errorf("event without links should give nil, got %v", got)
	}

	event := &cdevents.StoredEvent{Links: []models.Link{{LinkID: "a"}, {LinkID: ""}, {LinkID: "b"}}}
	got := graph.LinkedEventIDs(event)
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("LinkedEventIDs = %v, want [a b]", got)
	}
}

func storageIDs(events []*cdevents.StoredEvent) []string {
	result := make([]string, 0, len(events))
	for _, e := range events {
		result = append(result, e.ID)
	}
	return result
}
