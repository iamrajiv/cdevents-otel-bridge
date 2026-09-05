/*
Package models defines the core data structures for the CDEvents-OTel Bridge.

This file contains the models returned by the chain API. EventChain is the
set of related events discovered from a starting event, ordered newest first
so an incident appears before the deployment that caused it, which appears
before the pipeline run and the merged change. RootCause carries the commit
the chain traces back to.
*/
package models

import "time"

type EventChain struct {
	ChainID    string       `json:"chainId,omitempty"`
	StartEvent string       `json:"startEvent"`
	Events     []ChainEvent `json:"events"`
	RootCause  *RootCause   `json:"rootCause,omitempty"`
}

type ChainEvent struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Summary   string    `json:"summary"`
	Commit    string    `json:"commit,omitempty"`
	Links     []Link    `json:"links,omitempty"`
}

type RootCause struct {
	Commit     string `json:"commit"`
	Author     string `json:"author,omitempty"`
	Message    string `json:"message,omitempty"`
	Repository string `json:"repository,omitempty"`
}
