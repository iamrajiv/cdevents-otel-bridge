/*
Package models defines the core data structures for the CDEvents-OTel Bridge.

This file contains the PipelineRun model which represents CI/CD pipeline
executions. A pipeline run captures the complete lifecycle of a pipeline
from queue through completion, including status, timing, and source context.

The Duration method calculates execution time from start to finish.
*/
package models

import "time"

type PipelineRun struct {
	ID           string     `json:"id"`
	PipelineName string     `json:"pipelineName"`
	Status       string     `json:"status"`
	Outcome      string     `json:"outcome,omitempty"`
	StartedAt    time.Time  `json:"startedAt"`
	FinishedAt   *time.Time `json:"finishedAt,omitempty"`
	Source       string     `json:"source"`
	PipelineURL  string     `json:"pipelineUrl,omitempty"`
	CommitSha    string     `json:"commitSha,omitempty"`
	Branch       string     `json:"branch,omitempty"`
	Repository   string     `json:"repository,omitempty"`
	Author       string     `json:"author,omitempty"`
	EventID      string     `json:"eventId"`
	ChainID      string     `json:"chainId,omitempty"`
	Links        []Link     `json:"links,omitempty"`
}

func (p *PipelineRun) Duration() *time.Duration {
	if p.FinishedAt == nil {
		return nil
	}
	d := p.FinishedAt.Sub(p.StartedAt)
	return &d
}
