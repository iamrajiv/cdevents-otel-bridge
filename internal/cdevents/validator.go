/*
Package cdevents provides validation utilities for CDEvents.

Validate enforces the fields the CDEvents specification marks as required
and that the bridge relies on downstream: context.id, context.type,
context.source, context.timestamp, subject.id and subject.type. Parsing and
validation are deliberately separate so the API can distinguish malformed
JSON from a well-formed event that is missing data.
*/
package cdevents

import (
	"errors"
	"fmt"
)

var ErrInvalidEvent = errors.New("invalid event")

func Validate(event *CDEvent) error {
	if event == nil {
		return fmt.Errorf("%w: event is nil", ErrInvalidEvent)
	}

	if event.Context.ID == "" {
		return fmt.Errorf("%w: context.id is required", ErrInvalidEvent)
	}

	if event.Context.Type == "" {
		return fmt.Errorf("%w: context.type is required", ErrInvalidEvent)
	}

	if event.Context.Source == "" {
		return fmt.Errorf("%w: context.source is required", ErrInvalidEvent)
	}

	if event.Context.Timestamp.IsZero() {
		return fmt.Errorf("%w: context.timestamp is required", ErrInvalidEvent)
	}

	if event.Subject.ID == "" {
		return fmt.Errorf("%w: subject.id is required", ErrInvalidEvent)
	}

	if event.Subject.Type == "" {
		return fmt.Errorf("%w: subject.type is required", ErrInvalidEvent)
	}

	return nil
}
