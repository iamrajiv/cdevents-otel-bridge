/*
Package cdevents provides link handling utilities for CDEvents.

CDEvents links connect an event to the events that caused or triggered it and
are the basis for chain reconstruction in the correlator. Producers place
links either under context.links (CDEvents 0.4) or at the top level of the
envelope (older tooling). NormalizeLinks merges both locations into
context.links, drops links without a target and removes duplicates so the
rest of the bridge only ever deals with one canonical list.
*/
package cdevents

import "github.com/iamrajiv/cdevents-otel-bridge/pkg/models"

func NormalizeLinks(event *CDEvent) {
	if event == nil {
		return
	}

	merged := make([]models.Link, 0, len(event.Context.Links)+len(event.Links))
	seen := make(map[string]bool)

	add := func(link models.Link) {
		if link.LinkID == "" {
			return
		}
		key := link.LinkType + "\x00" + link.LinkID
		if seen[key] {
			return
		}
		seen[key] = true
		merged = append(merged, link)
	}

	for _, link := range event.Context.Links {
		add(link)
	}
	for _, link := range event.Links {
		add(link)
	}

	if len(merged) == 0 {
		merged = nil
	}
	event.Context.Links = merged
	event.Links = nil
}

func ExtractLinks(event *CDEvent) []models.Link {
	if event == nil {
		return nil
	}
	return event.Context.Links
}

func GetLinkedEventIDs(event *CDEvent) []string {
	if event == nil || len(event.Context.Links) == 0 {
		return nil
	}

	ids := make([]string, 0, len(event.Context.Links))
	for _, link := range event.Context.Links {
		if link.LinkID != "" {
			ids = append(ids, link.LinkID)
		}
	}
	return ids
}

func HasLinkType(event *CDEvent, linkType string) bool {
	if event == nil || linkType == "" {
		return false
	}

	for _, link := range event.Context.Links {
		if link.LinkType == linkType {
			return true
		}
	}
	return false
}
