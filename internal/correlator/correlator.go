/*
Package correlator provides event correlation and chain reconstruction for
CDEvents.

The Correlator answers "what led to this event?". Starting from any stored
event it collects:

 1. every event reachable by following links (triggeredBy, causedBy, ...),
 2. every other event that shares the start event's chainId,

merges the two sets, orders them newest first and derives a root cause. The
root cause prefers the earliest change.merged event that carries a commit;
when no change event exists it falls back to the commit recorded on the
deployment itself, which is what most CI tools send.
*/
package correlator

import (
	"context"
	"fmt"
	"sort"

	"github.com/iamrajiv/cdevents-otel-bridge/internal/cdevents"
	"github.com/iamrajiv/cdevents-otel-bridge/internal/storage"
	"github.com/iamrajiv/cdevents-otel-bridge/pkg/models"
)

type Correlator struct {
	storage storage.Storage
	graph   *EventGraph
}

func NewCorrelator(store storage.Storage) *Correlator {
	return &Correlator{
		storage: store,
		graph:   NewEventGraph(store),
	}
}

func (c *Correlator) BuildChain(ctx context.Context, eventID string) (*models.EventChain, error) {
	startEvent, err := c.storage.GetEvent(ctx, eventID)
	if err != nil {
		return nil, fmt.Errorf("failed to get start event %s: %w", eventID, err)
	}

	linked, err := c.graph.TraverseChain(ctx, eventID)
	if err != nil {
		return nil, err
	}

	events := make(map[string]*cdevents.StoredEvent, len(linked))
	for _, event := range linked {
		events[event.ID] = event
	}

	if startEvent.ChainID != "" {
		siblings, err := c.storage.GetEventsByChain(ctx, startEvent.ChainID)
		if err != nil {
			return nil, fmt.Errorf("failed to load chain %s: %w", startEvent.ChainID, err)
		}
		for _, event := range siblings {
			if _, seen := events[event.ID]; !seen {
				events[event.ID] = event
			}
		}
	}

	ordered := make([]*cdevents.StoredEvent, 0, len(events))
	for _, event := range events {
		ordered = append(ordered, event)
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		if !ordered[i].Timestamp.Equal(ordered[j].Timestamp) {
			return ordered[i].Timestamp.After(ordered[j].Timestamp)
		}
		return ordered[i].ID < ordered[j].ID
	})

	chain := &models.EventChain{
		ChainID:    startEvent.ChainID,
		StartEvent: eventID,
		Events:     make([]models.ChainEvent, 0, len(ordered)),
		RootCause:  FindRootCause(ordered),
	}

	for _, event := range ordered {
		chain.Events = append(chain.Events, toChainEvent(event))
	}

	return chain, nil
}

func FindRootCause(events []*cdevents.StoredEvent) *models.RootCause {
	var fallback *models.RootCause

	for i := len(events) - 1; i >= 0; i-- {
		event := events[i]
		info := cdevents.ExtractChangeInfo(event)
		if info.Commit == "" {
			continue
		}

		cause := &models.RootCause{
			Commit:     info.Commit,
			Author:     info.Author,
			Message:    info.Message,
			Repository: info.Repository,
		}

		switch event.ShortType() {
		case cdevents.KindChangeMerged:
			return cause
		case cdevents.KindServiceDeployed, cdevents.KindServiceUpgraded, cdevents.KindServiceRolledback:
			if fallback == nil {
				fallback = cause
			}
		}
	}

	return fallback
}

func toChainEvent(event *cdevents.StoredEvent) models.ChainEvent {
	chainEvent := models.ChainEvent{
		ID:        event.ID,
		Type:      event.Type,
		Timestamp: event.Timestamp,
		Summary:   event.Summary,
		Links:     event.Links,
	}

	switch event.ShortType() {
	case cdevents.KindChangeMerged, cdevents.KindServiceDeployed, cdevents.KindServiceUpgraded, cdevents.KindServiceRolledback:
		chainEvent.Commit = cdevents.ExtractChangeInfo(event).Commit
	}

	return chainEvent
}
