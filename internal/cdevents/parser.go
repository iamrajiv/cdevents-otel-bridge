/*
Package cdevents provides parsing utilities for CDEvents.

This file converts raw JSON into a CDEvent and then into the domain models
the bridge stores (Deployment, PipelineRun, Incident, StoredEvent).

CI/CD tools are inconsistent about where they put metadata, so every field is
resolved through an ordered list of locations. For a deployment:

  - service:     subject.content.service.name, customData.service, subject.id
  - version:     subject.content.service.version, customData.version,
    derived from subject.content.artifactId
  - environment: subject.content.environment.id, subject.content.environment,
    customData.environment
  - commit, repository, author, branch, pipeline id/url, deployer:
    customData first, then subject.content

Parse only fails on malformed JSON; use Validate for required-field checks.
*/
package cdevents

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/iamrajiv/cdevents-otel-bridge/pkg/models"
)

var (
	commitKeys     = []string{"commitSha", "commit", "sha", "gitCommit"}
	repositoryKeys = []string{"repository", "repo", "repositoryUrl"}
	authorKeys     = []string{"author", "committer", "actor"}
	messageKeys    = []string{"message", "commitMessage", "title", "description"}
	branchKeys     = []string{"branch", "ref"}
	pipelineIDKeys = []string{"pipelineId", "pipelineRunId", "buildId", "runId"}
	pipelineURLKey = []string{"pipelineUrl", "buildUrl", "workflowUrl", "runUrl"}
	deployerKeys   = []string{"deployer", "deployedBy"}
	serviceKeys    = []string{"service", "serviceName"}
	severityKeys   = []string{"severity", "priority"}
	descKeys       = []string{"description", "summary", "title", "message"}
	outcomeKeys    = []string{"outcome", "status", "result"}
)

type ChangeInfo struct {
	Commit     string
	Author     string
	Message    string
	Repository string
}

func Parse(data []byte) (*CDEvent, error) {
	var event CDEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return nil, fmt.Errorf("failed to unmarshal CDEvent: %w", err)
	}

	NormalizeLinks(&event)
	return &event, nil
}

func ToDeployment(event *CDEvent) *models.Deployment {
	if event == nil {
		return nil
	}

	content := event.Subject.Content
	custom := event.CustomData

	service := firstNonEmpty(
		getNestedString(content, "service", "name"),
		stringFrom(serviceKeys, custom),
		event.Subject.ID,
	)

	version := firstNonEmpty(
		getNestedString(content, "service", "version"),
		stringFrom([]string{"version"}, custom),
		versionFromArtifactID(getStringField(content, "artifactId")),
	)

	environment := firstNonEmpty(
		getNestedString(content, "environment", "id"),
		getStringField(content, "environment"),
		identityFrom([]string{"environment"}, custom),
	)

	return &models.Deployment{
		ID:          event.Subject.ID,
		Service:     service,
		Version:     version,
		Environment: environment,
		CommitSha:   stringFrom(commitKeys, custom, content),
		Repository:  identityFrom(repositoryKeys, custom, content),
		Branch:      stringFrom(branchKeys, custom, content),
		Author:      stringFrom(authorKeys, custom, content),
		PipelineID:  stringFrom(pipelineIDKeys, custom, content),
		PipelineURL: stringFrom(pipelineURLKey, custom, content),
		DeployedAt:  event.Context.Timestamp,
		DeployedBy:  stringFrom(deployerKeys, custom, content),
		EventID:     event.Context.ID,
		ChainID:     event.Context.ChainID,
		Links:       event.Context.Links,
	}
}

func ToPipelineRun(event *CDEvent) *models.PipelineRun {
	if event == nil {
		return nil
	}

	content := event.Subject.Content
	custom := event.CustomData

	status := "unknown"
	switch event.ShortType() {
	case KindPipelineRunQueued:
		status = "queued"
	case KindPipelineRunStarted:
		status = "started"
	case KindPipelineRunFinish:
		status = "finished"
	}

	run := &models.PipelineRun{
		ID:           event.Subject.ID,
		PipelineName: firstNonEmpty(getStringField(content, "pipelineName"), getStringField(content, "name")),
		Status:       status,
		Outcome:      stringFrom(outcomeKeys, content, custom),
		StartedAt:    parseTimeOr(getStringField(content, "startTime"), event.Context.Timestamp),
		Source:       event.Context.Source,
		PipelineURL:  firstNonEmpty(getStringField(content, "url"), stringFrom(pipelineURLKey, custom, content)),
		CommitSha:    stringFrom(commitKeys, custom, content),
		Branch:       stringFrom(branchKeys, custom, content),
		Repository:   identityFrom(repositoryKeys, custom, content),
		Author:       stringFrom(authorKeys, custom, content),
		EventID:      event.Context.ID,
		ChainID:      event.Context.ChainID,
		Links:        event.Context.Links,
	}

	if status == "finished" {
		finished := parseTimeOr(getStringField(content, "endTime"), event.Context.Timestamp)
		run.FinishedAt = &finished
	}

	return run
}

func ToIncident(event *CDEvent) *models.Incident {
	if event == nil {
		return nil
	}

	content := event.Subject.Content
	custom := event.CustomData

	status := "unknown"
	switch event.ShortType() {
	case KindIncidentDetected:
		status = "detected"
	case KindIncidentReported:
		status = "reported"
	case KindIncidentResolved:
		status = "resolved"
	}

	incident := &models.Incident{
		ID:          event.Subject.ID,
		Service:     firstNonEmpty(getNestedString(content, "service", "name"), identityFrom(serviceKeys, content, custom)),
		Environment: firstNonEmpty(getNestedString(content, "environment", "id"), identityFrom([]string{"environment"}, content, custom)),
		Severity:    stringFrom(severityKeys, content, custom),
		Description: stringFrom(descKeys, content, custom),
		Status:      status,
		DetectedAt:  event.Context.Timestamp,
		EventID:     event.Context.ID,
		ChainID:     event.Context.ChainID,
		Links:       event.Context.Links,
	}

	if status == "resolved" {
		resolved := event.Context.Timestamp
		incident.ResolvedAt = &resolved
	}

	return incident
}

func ToStoredEvent(event *CDEvent) *StoredEvent {
	if event == nil {
		return nil
	}

	rawData := make(map[string]any)
	if event.Subject.Content != nil {
		rawData["content"] = event.Subject.Content
	}
	if event.CustomData != nil {
		rawData["customData"] = event.CustomData
	}
	if len(rawData) == 0 {
		rawData = nil
	}

	return &StoredEvent{
		ID:          event.Context.ID,
		Type:        event.Context.Type,
		Source:      event.Context.Source,
		Timestamp:   event.Context.Timestamp,
		SubjectID:   event.Subject.ID,
		SubjectType: event.Subject.Type,
		ChainID:     event.Context.ChainID,
		Links:       event.Context.Links,
		Summary:     Summarize(event),
		RawData:     rawData,
	}
}

func ExtractChangeInfo(event *StoredEvent) ChangeInfo {
	if event == nil || event.RawData == nil {
		return ChangeInfo{}
	}

	custom, _ := event.RawData["customData"].(map[string]any)
	content, _ := event.RawData["content"].(map[string]any)
	flat := event.RawData

	return ChangeInfo{
		Commit:     stringFrom(commitKeys, custom, content, flat),
		Author:     stringFrom(authorKeys, custom, content, flat),
		Message:    stringFrom(messageKeys, custom, content, flat),
		Repository: identityFrom(repositoryKeys, custom, content, flat),
	}
}

func Summarize(event *CDEvent) string {
	if event == nil {
		return ""
	}

	kind := event.ShortType()
	subjectType, predicate, _ := strings.Cut(kind, ".")
	content := event.Subject.Content
	custom := event.CustomData

	switch kind {
	case KindServiceDeployed, KindServiceUpgraded, KindServiceRolledback:
		d := ToDeployment(event)
		parts := []string{predicateVerb(predicate), d.Service}
		if d.Version != "" {
			parts = append(parts, d.Version)
		}
		if d.Environment != "" {
			parts = append(parts, "to", d.Environment)
		}
		return strings.Join(parts, " ")
	case KindPipelineRunQueued, KindPipelineRunStarted, KindPipelineRunFinish:
		run := ToPipelineRun(event)
		name := firstNonEmpty(run.PipelineName, run.ID)
		summary := fmt.Sprintf("Pipeline %s %s", name, predicate)
		if run.Outcome != "" {
			summary += " (" + run.Outcome + ")"
		}
		return summary
	case KindChangeMerged:
		summary := "Merged change " + event.Subject.ID
		if author := stringFrom(authorKeys, custom, content); author != "" {
			summary += " by " + author
		}
		return summary
	case KindIncidentDetected, KindIncidentReported, KindIncidentResolved:
		inc := ToIncident(event)
		summary := "Incident " + predicate
		if inc.Description != "" {
			summary += ": " + inc.Description
		} else if inc.ID != "" {
			summary += ": " + inc.ID
		}
		return summary
	}

	if event.Subject.ID == "" {
		return fmt.Sprintf("%s %s", subjectType, predicate)
	}
	return fmt.Sprintf("%s %s: %s", subjectType, predicate, event.Subject.ID)
}

func predicateVerb(predicate string) string {
	switch predicate {
	case "deployed":
		return "Deployed"
	case "upgraded":
		return "Upgraded"
	case "rolledback":
		return "Rolled back"
	}
	return predicate
}

func parseTimeOr(value string, fallback time.Time) time.Time {
	if value == "" {
		return fallback
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t
	}
	return fallback
}
