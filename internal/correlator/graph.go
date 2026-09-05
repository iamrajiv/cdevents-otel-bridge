/*
Package correlator provides event correlation and chain reconstruction for
CDEvents.

This file implements EventGraph, which walks the links between stored events
to collect everything an event was triggered or caused by. The walk is a
breadth-first traversal over the storage backend rather than an in-memory
structure, so it works with any backend and after restarts. A visited set
protects against cycles, and depth and size limits bound the work done for
pathological link graphs. Links that point at events the bridge never
received are skipped rather than treated as errors.
*/
package correlator

import (
	"context"
	"errors"
	"fmt"

	"github.com/iamrajiv/cdevents-otel-bridge/internal/cdevents"
	"github.com/iamrajiv/cdevents-otel-bridge/internal/storage"
)

const (
	DefaultMaxDepth  = 32
	DefaultMaxEvents = 256
)

type EventGraph struct {
	store     storage.Storage
	maxDepth  int
	maxEvents int
}

func NewEventGraph(store storage.Storage) *EventGraph {
	return &EventGraph{
		store:     store,
		maxDepth:  DefaultMaxDepth,
		maxEvents: DefaultMaxEvents,
	}
}

type queueItem struct {
	id    string
	depth int
}

func (g *EventGraph) TraverseChain(ctx context.Context, startID string) ([]*cdevents.StoredEvent, error) {
	visited := map[string]bool{startID: true}
	queue := []queueItem{{id: startID}}
	result := make([]*cdevents.StoredEvent, 0)

	for len(queue) > 0 && len(result) < g.maxEvents {
		current := queue[0]
		queue = queue[1:]

		event, err := g.store.GetEvent(ctx, current.id)
		if errors.Is(err, storage.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("failed to load event %s: %w", current.id, err)
		}

		result = append(result, event)

		if current.depth >= g.maxDepth {
			continue
		}
		for _, link := range event.Links {
			if link.LinkID == "" || visited[link.LinkID] {
				continue
			}
			visited[link.LinkID] = true
			queue = append(queue, queueItem{id: link.LinkID, depth: current.depth + 1})
		}
	}

	return result, nil
}

func (g *EventGraph) LinkedEventIDs(event *cdevents.StoredEvent) []string {
	if event == nil || len(event.Links) == 0 {
		return nil
	}

	ids := make([]string, 0, len(event.Links))
	for _, link := range event.Links {
		if link.LinkID != "" {
			ids = append(ids, link.LinkID)
		}
	}
	return ids
}
