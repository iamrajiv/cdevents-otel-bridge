/*
Unit tests for link helpers.
*/
package cdevents

import (
	"testing"

	"github.com/iamrajiv/cdevents-otel-bridge/pkg/models"
)

func TestLinkHelpers(t *testing.T) {
	event := &CDEvent{Context: EventContext{Links: []models.Link{
		{LinkType: "triggeredBy", LinkID: "a"},
		{LinkType: "causedBy", LinkID: "b"},
		{LinkType: "relatedTo", LinkID: ""},
	}}}

	if got := ExtractLinks(event); len(got) != 3 {
		t.Errorf("ExtractLinks() returned %d links, want 3", len(got))
	}

	ids := GetLinkedEventIDs(event)
	if len(ids) != 2 || ids[0] != "a" || ids[1] != "b" {
		t.Errorf("GetLinkedEventIDs() = %v, want [a b]", ids)
	}

	if !HasLinkType(event, "causedBy") || HasLinkType(event, "missing") || HasLinkType(event, "") {
		t.Error("HasLinkType() gave unexpected result")
	}

	if ExtractLinks(nil) != nil || GetLinkedEventIDs(nil) != nil || HasLinkType(nil, "x") {
		t.Error("nil event should yield zero values")
	}
	if GetLinkedEventIDs(&CDEvent{}) != nil {
		t.Error("event without links should yield nil ids")
	}
}

func TestNormalizeLinksNil(t *testing.T) {
	NormalizeLinks(nil)

	event := &CDEvent{}
	NormalizeLinks(event)
	if event.Context.Links != nil {
		t.Errorf("expected nil links, got %v", event.Context.Links)
	}
}
