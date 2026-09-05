/*
Package models defines the core data structures for the CDEvents-OTel Bridge.

This file contains the Deployment and Link models. A Deployment records the
release of a specific version of a service to an environment, including the
source commit, the pipeline that produced it and the CDEvent it was derived
from.

Link is the normalized representation of a CDEvents link. Producers in the
wild send links in several shapes, so Link accepts all of the following when
decoding JSON and always encodes as {"linkType": ..., "linkId": ...}:

  - {"linkType": "triggeredBy", "linkId": "evt-1"}       (this project's spec)
  - {"type": "TRIGGERED_BY", "target": "evt-1"}          (older examples)
  - {"linkType": "RELATION", "linkKind": "TRIGGER",
    "from": {"contextId": "evt-1"}}                     (CDEvents 0.4 native)

When a CDEvents-native link carries a linkKind, that value is used as the
LinkType because it names the relationship; the coarse PATH/END/RELATION
value is kept only when no linkKind is present.
*/
package models

import (
	"encoding/json"
	"time"
)

type Deployment struct {
	ID          string    `json:"id"`
	Service     string    `json:"service"`
	Version     string    `json:"version"`
	Environment string    `json:"environment"`
	CommitSha   string    `json:"commitSha"`
	Repository  string    `json:"repository"`
	Branch      string    `json:"branch,omitempty"`
	Author      string    `json:"author,omitempty"`
	PipelineID  string    `json:"pipelineId,omitempty"`
	PipelineURL string    `json:"pipelineUrl,omitempty"`
	DeployedAt  time.Time `json:"deployedAt"`
	DeployedBy  string    `json:"deployedBy"`
	EventID     string    `json:"eventId"`
	ChainID     string    `json:"chainId,omitempty"`
	Links       []Link    `json:"links,omitempty"`
}

type Link struct {
	LinkType string `json:"linkType"`
	LinkID   string `json:"linkId"`
}

type rawLink struct {
	LinkType string `json:"linkType"`
	LinkID   string `json:"linkId"`
	LinkKind string `json:"linkKind"`
	Type     string `json:"type"`
	Target   string `json:"target"`
	From     struct {
		ContextID string `json:"contextId"`
	} `json:"from"`
}

func (l *Link) UnmarshalJSON(data []byte) error {
	var raw rawLink
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	l.LinkType = firstNonEmpty(raw.LinkKind, raw.LinkType, raw.Type)
	l.LinkID = firstNonEmpty(raw.LinkID, raw.Target, raw.From.ContextID)
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
