/*
Package cdevents provides types and utilities for handling CDEvents.

This file defines the core data structures:

  - CDEvent: the incoming event as sent by a CI/CD tool. Only the fields the
    bridge needs are typed; subject content and customData stay as maps so
    producers can send whatever extra data they have.
  - EventContext / EventSubject: the two mandatory CDEvents sections.
  - StoredEvent: the normalized, backend-agnostic record kept in storage and
    used by the correlator.

CDEvent type strings look like "dev.cdevents.service.deployed.0.1.1". The
ShortType helper reduces that to "service.deployed" so callers can dispatch
on the subject/predicate pair without caring about the schema version.
*/
package cdevents

import (
	"strings"
	"time"

	"github.com/iamrajiv/cdevents-otel-bridge/pkg/models"
)

const (
	KindServiceDeployed    = "service.deployed"
	KindServiceUpgraded    = "service.upgraded"
	KindServiceRolledback  = "service.rolledback"
	KindPipelineRunQueued  = "pipelinerun.queued"
	KindPipelineRunStarted = "pipelinerun.started"
	KindPipelineRunFinish  = "pipelinerun.finished"
	KindChangeMerged       = "change.merged"
	KindIncidentDetected   = "incident.detected"
	KindIncidentReported   = "incident.reported"
	KindIncidentResolved   = "incident.resolved"
)

type CDEvent struct {
	Context    EventContext   `json:"context"`
	Subject    EventSubject   `json:"subject"`
	CustomData map[string]any `json:"customData,omitempty"`
	Links      []models.Link  `json:"links,omitempty"`
}

type EventContext struct {
	Version   string        `json:"version,omitempty"`
	ID        string        `json:"id"`
	Source    string        `json:"source"`
	Type      string        `json:"type"`
	Timestamp time.Time     `json:"timestamp"`
	ChainID   string        `json:"chainId,omitempty"`
	Links     []models.Link `json:"links,omitempty"`
}

type EventSubject struct {
	ID      string         `json:"id"`
	Source  string         `json:"source,omitempty"`
	Type    string         `json:"type"`
	Content map[string]any `json:"content,omitempty"`
}

type StoredEvent struct {
	ID          string         `json:"id"`
	Type        string         `json:"type"`
	Source      string         `json:"source"`
	Timestamp   time.Time      `json:"timestamp"`
	SubjectID   string         `json:"subjectId"`
	SubjectType string         `json:"subjectType"`
	ChainID     string         `json:"chainId,omitempty"`
	Links       []models.Link  `json:"links,omitempty"`
	Summary     string         `json:"summary"`
	RawData     map[string]any `json:"rawData,omitempty"`
}

func (e *CDEvent) ShortType() string {
	return ShortType(e.Context.Type)
}

func (e *StoredEvent) ShortType() string {
	return ShortType(e.Type)
}

func (e *CDEvent) IsDeployment() bool {
	switch e.ShortType() {
	case KindServiceDeployed, KindServiceUpgraded, KindServiceRolledback:
		return true
	}
	return false
}

func ShortType(eventType string) string {
	parts := strings.Split(strings.TrimSpace(eventType), ".")
	if len(parts) >= 4 && parts[0] == "dev" && parts[1] == "cdevents" {
		return parts[2] + "." + parts[3]
	}

	for len(parts) > 2 && isVersionSegment(parts[len(parts)-1]) {
		parts = parts[:len(parts)-1]
	}

	if len(parts) >= 2 {
		return parts[len(parts)-2] + "." + parts[len(parts)-1]
	}
	return eventType
}

func isVersionSegment(s string) bool {
	if s == "" {
		return false
	}
	if s[0] == 'v' || s[0] == 'V' {
		s = s[1:]
	}
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
