/*
Unit tests for CDEvent validation.

Tests verify that the validator correctly identifies missing or invalid required fields
according to the CDEvents specification. Uses table-driven tests to cover all validation
scenarios.
*/
package cdevents

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestValidate(t *testing.T) {
	validTimestamp := time.Now()

	tests := []struct {
		name    string
		event   *CDEvent
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid event",
			event: &CDEvent{
				Context: EventContext{
					ID:        "event-123",
					Type:      "dev.cdevents.service.deployed.0.1.0",
					Source:    "github.com/myorg/myrepo",
					Timestamp: validTimestamp,
				},
				Subject: EventSubject{
					ID:   "api-service",
					Type: "service",
				},
			},
			wantErr: false,
		},
		{
			name:    "nil event",
			event:   nil,
			wantErr: true,
			errMsg:  "event is nil",
		},
		{
			name: "missing context.id",
			event: &CDEvent{
				Context: EventContext{
					Type:      "dev.cdevents.service.deployed.0.1.0",
					Source:    "github.com/myorg/myrepo",
					Timestamp: validTimestamp,
				},
				Subject: EventSubject{
					ID:   "api-service",
					Type: "service",
				},
			},
			wantErr: true,
			errMsg:  "context.id is required",
		},
		{
			name: "missing context.type",
			event: &CDEvent{
				Context: EventContext{
					ID:        "event-123",
					Source:    "github.com/myorg/myrepo",
					Timestamp: validTimestamp,
				},
				Subject: EventSubject{
					ID:   "api-service",
					Type: "service",
				},
			},
			wantErr: true,
			errMsg:  "context.type is required",
		},
		{
			name: "missing context.source",
			event: &CDEvent{
				Context: EventContext{
					ID:        "event-123",
					Type:      "dev.cdevents.service.deployed.0.1.0",
					Timestamp: validTimestamp,
				},
				Subject: EventSubject{
					ID:   "api-service",
					Type: "service",
				},
			},
			wantErr: true,
			errMsg:  "context.source is required",
		},
		{
			name: "missing context.timestamp",
			event: &CDEvent{
				Context: EventContext{
					ID:     "event-123",
					Type:   "dev.cdevents.service.deployed.0.1.0",
					Source: "github.com/myorg/myrepo",
				},
				Subject: EventSubject{
					ID:   "api-service",
					Type: "service",
				},
			},
			wantErr: true,
			errMsg:  "context.timestamp is required",
		},
		{
			name: "missing subject.id",
			event: &CDEvent{
				Context: EventContext{
					ID:        "event-123",
					Type:      "dev.cdevents.service.deployed.0.1.0",
					Source:    "github.com/myorg/myrepo",
					Timestamp: validTimestamp,
				},
				Subject: EventSubject{
					Type: "service",
				},
			},
			wantErr: true,
			errMsg:  "subject.id is required",
		},
		{
			name: "missing subject.type",
			event: &CDEvent{
				Context: EventContext{
					ID:        "event-123",
					Type:      "dev.cdevents.service.deployed.0.1.0",
					Source:    "github.com/myorg/myrepo",
					Timestamp: validTimestamp,
				},
				Subject: EventSubject{
					ID: "api-service",
				},
			},
			wantErr: true,
			errMsg:  "subject.type is required",
		},
		{
			name: "valid event with optional fields",
			event: &CDEvent{
				Context: EventContext{
					Version:   "0.4.0",
					ID:        "event-123",
					Type:      "dev.cdevents.service.deployed.0.1.0",
					Source:    "github.com/myorg/myrepo",
					Timestamp: validTimestamp,
					ChainID:   "chain-abc",
				},
				Subject: EventSubject{
					ID:     "api-service",
					Type:   "service",
					Source: "github.com/myorg/myrepo",
					Content: map[string]any{
						"artifactId": "myorg/api-service:v1.2.3",
					},
				},
				CustomData: map[string]any{
					"commitSha": "abc123",
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.event)

			if tt.wantErr {
				if err == nil {
					t.Error("Validate() expected error, got nil")
					return
				}

				if !errors.Is(err, ErrInvalidEvent) {
					t.Errorf("Validate() error should wrap ErrInvalidEvent, got %v", err)
				}

				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Validate() error message = %q, should contain %q", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("Validate() unexpected error: %v", err)
				}
			}
		})
	}
}

func TestValidateErrorWrapping(t *testing.T) {
	err := Validate(nil)
	if err == nil {
		t.Fatal("Validate(nil) should return error")
	}

	if !errors.Is(err, ErrInvalidEvent) {
		t.Error("Validate() error should wrap ErrInvalidEvent")
	}
}

func TestValidateAllFieldCombinations(t *testing.T) {
	validTimestamp := time.Now()

	baseEvent := &CDEvent{
		Context: EventContext{
			ID:        "event-123",
			Type:      "dev.cdevents.service.deployed.0.1.0",
			Source:    "github.com/myorg/myrepo",
			Timestamp: validTimestamp,
		},
		Subject: EventSubject{
			ID:   "api-service",
			Type: "service",
		},
	}

	err := Validate(baseEvent)
	if err != nil {
		t.Fatalf("Base valid event should not error: %v", err)
	}

	emptyIDEvent := *baseEvent
	emptyIDEvent.Context.ID = ""
	if err := Validate(&emptyIDEvent); err == nil {
		t.Error("Empty context.id should cause error")
	}

	emptyTypeEvent := *baseEvent
	emptyTypeEvent.Context.Type = ""
	if err := Validate(&emptyTypeEvent); err == nil {
		t.Error("Empty context.type should cause error")
	}

	emptySourceEvent := *baseEvent
	emptySourceEvent.Context.Source = ""
	if err := Validate(&emptySourceEvent); err == nil {
		t.Error("Empty context.source should cause error")
	}

	zeroTimestampEvent := *baseEvent
	zeroTimestampEvent.Context.Timestamp = time.Time{}
	if err := Validate(&zeroTimestampEvent); err == nil {
		t.Error("Zero timestamp should cause error")
	}

	emptySubjectIDEvent := *baseEvent
	emptySubjectIDEvent.Subject.ID = ""
	if err := Validate(&emptySubjectIDEvent); err == nil {
		t.Error("Empty subject.id should cause error")
	}

	emptySubjectTypeEvent := *baseEvent
	emptySubjectTypeEvent.Subject.Type = ""
	if err := Validate(&emptySubjectTypeEvent); err == nil {
		t.Error("Empty subject.type should cause error")
	}
}
