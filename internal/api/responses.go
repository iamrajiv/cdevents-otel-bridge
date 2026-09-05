/*
Package api provides the HTTP API for the CDEvents-OTel Bridge.

This file defines the JSON envelopes shared by all handlers and the helpers
that write them. Responses are marshaled before any header is sent so an
encoding failure still produces a well-formed 500 instead of a truncated
body.
*/
package api

import (
	"encoding/json"
	"net/http"
)

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

type SuccessResponse struct {
	Status    string `json:"status"`
	EventID   string `json:"eventId,omitempty"`
	EventType string `json:"eventType,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`
}

func respondJSON(w http.ResponseWriter, status int, data any) {
	body, err := json.Marshal(data)
	if err != nil {
		http.Error(w, `{"error":"encoding_error","message":"Failed to encode response"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func respondError(w http.ResponseWriter, status int, errType, message string) {
	respondJSON(w, status, ErrorResponse{Error: errType, Message: message})
}
