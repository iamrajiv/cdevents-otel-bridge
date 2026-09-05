/*
Package models defines the core data structures for the CDEvents-OTel Bridge.

This file contains the Incident model which represents operational incidents
detected in production environments. Incidents track issues from detection
through resolution, including severity, affected services, and timeline.

The Duration method calculates the total time from detection to resolution.
*/
package models

import "time"

type Incident struct {
	ID               string      `json:"id"`
	Service          string      `json:"service"`
	Environment      string      `json:"environment"`
	Severity         string      `json:"severity"`
	Description      string      `json:"description"`
	Status           string      `json:"status"`
	DetectedAt       time.Time   `json:"detectedAt"`
	ResolvedAt       *time.Time  `json:"resolvedAt,omitempty"`
	RootCause        string      `json:"rootCause,omitempty"`
	EventID          string      `json:"eventId"`
	ChainID          string      `json:"chainId,omitempty"`
	DeploymentID     string      `json:"deploymentId,omitempty"`
	Links            []Link      `json:"links,omitempty"`
	LinkedDeployment *Deployment `json:"linkedDeployment,omitempty"`
}

func (i *Incident) Duration() *time.Duration {
	if i.ResolvedAt == nil {
		return nil
	}
	d := i.ResolvedAt.Sub(i.DetectedAt)
	return &d
}
