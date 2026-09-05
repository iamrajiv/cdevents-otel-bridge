/*
Package api provides the HTTP API for the CDEvents-OTel Bridge.

This file implements the request handlers:

	POST /api/v1/events                 ingest a CDEvent
	GET  /api/v1/events/{eventId}       fetch a stored event
	GET  /api/v1/deployments            list deployments (environment, since, limit)
	GET  /api/v1/deployments/{service}  current deployment of a service
	GET  /api/v1/chain/{eventId}        reconstruct the chain behind an event
	GET  /api/v1/health                 liveness/readiness with a storage check

Ingestion parses and validates the event, stores a deployment record for
deployment events, stores every event for chain reconstruction, and records
metrics. Client mistakes produce 4xx responses with a machine-readable
error code; storage failures produce 5xx.
*/
package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/iamrajiv/cdevents-otel-bridge/internal/cdevents"
	"github.com/iamrajiv/cdevents-otel-bridge/internal/correlator"
	"github.com/iamrajiv/cdevents-otel-bridge/internal/metrics"
	"github.com/iamrajiv/cdevents-otel-bridge/internal/storage"
	"github.com/iamrajiv/cdevents-otel-bridge/pkg/models"
)

const (
	maxBodyBytes        = 1 << 20
	defaultListLimit    = 50
	maxListLimit        = 100
	healthCheckTimeout  = 2 * time.Second
	statusHealthy       = "healthy"
	statusUnhealthy     = "unhealthy"
	errInvalidRequest   = "invalid_request"
	errInvalidEvent     = "invalid_event"
	errValidation       = "validation_error"
	errPayloadTooLarge  = "payload_too_large"
	errNotFound         = "not_found"
	errStorage          = "storage_error"
	errCorrelation      = "correlation_error"
	rejectParse         = "parse"
	rejectValidation    = "validation"
	rejectStorage       = "storage"
	rejectTooLarge      = "payload_too_large"
	rejectUnreadable    = "unreadable_body"
	deploymentLinkSelf  = "self"
	deploymentLinkEvent = "event"
	deploymentLinkChain = "chain"
)

type Handler struct {
	storage    storage.Storage
	correlator *correlator.Correlator
	metrics    *metrics.Metrics
	logger     *slog.Logger
	version    string
	startTime  time.Time
}

func NewHandler(store storage.Storage, corr *correlator.Correlator, m *metrics.Metrics, logger *slog.Logger, version string) *Handler {
	return &Handler{
		storage:    store,
		correlator: corr,
		metrics:    m,
		logger:     logger,
		version:    version,
		startTime:  time.Now(),
	}
}

func (h *Handler) HandlePostEvent(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			h.metrics.EventRejected(rejectTooLarge)
			respondError(w, http.StatusRequestEntityTooLarge, errPayloadTooLarge,
				fmt.Sprintf("Request body exceeds %d bytes", maxBodyBytes))
			return
		}
		h.metrics.EventRejected(rejectUnreadable)
		respondError(w, http.StatusBadRequest, errInvalidRequest, "Failed to read request body")
		return
	}

	event, err := cdevents.Parse(body)
	if err != nil {
		h.metrics.EventRejected(rejectParse)
		h.logger.Warn("rejected CDEvent: parse failure", "error", err)
		respondError(w, http.StatusBadRequest, errInvalidEvent, fmt.Sprintf("Failed to parse CDEvent: %v", err))
		return
	}

	if err := cdevents.Validate(event); err != nil {
		h.metrics.EventRejected(rejectValidation)
		h.logger.Warn("rejected CDEvent: validation failure", "error", err, "id", event.Context.ID)
		respondError(w, http.StatusBadRequest, errValidation, fmt.Sprintf("CDEvent validation failed: %v", err))
		return
	}

	kind := event.ShortType()
	logger := h.logger.With("event_id", event.Context.ID, "event_type", kind, "subject", event.Subject.ID)

	if event.IsDeployment() {
		deployment := cdevents.ToDeployment(event)
		if err := h.storage.SaveDeployment(r.Context(), deployment); err != nil {
			h.metrics.EventRejected(rejectStorage)
			logger.Error("failed to save deployment", "error", err)
			respondError(w, http.StatusInternalServerError, errStorage, "Failed to save deployment")
			return
		}
		logger.Info("deployment recorded",
			"service", deployment.Service,
			"environment", deployment.Environment,
			"version", deployment.Version,
			"commit", deployment.CommitSha,
		)
	} else {
		h.logEventDetails(logger, event, kind)
	}

	if err := h.storage.SaveEvent(r.Context(), cdevents.ToStoredEvent(event)); err != nil {
		h.metrics.EventRejected(rejectStorage)
		logger.Error("failed to save event", "error", err)
		respondError(w, http.StatusInternalServerError, errStorage, "Failed to save event")
		return
	}

	h.metrics.EventReceived(kind)
	h.metrics.ObserveProcessing(time.Since(start))

	respondJSON(w, http.StatusCreated, SuccessResponse{
		Status:    "accepted",
		EventID:   event.Context.ID,
		EventType: event.Context.Type,
		Timestamp: event.Context.Timestamp.UTC().Format(time.RFC3339),
	})
}

func (h *Handler) logEventDetails(logger *slog.Logger, event *cdevents.CDEvent, kind string) {
	switch {
	case strings.HasPrefix(kind, "pipelinerun."):
		run := cdevents.ToPipelineRun(event)
		logger.Info("pipeline run event received", "pipeline", run.PipelineName, "status", run.Status, "outcome", run.Outcome)
	case strings.HasPrefix(kind, "incident."):
		incident := cdevents.ToIncident(event)
		logger.Info("incident event received", "service", incident.Service, "severity", incident.Severity, "status", incident.Status)
	default:
		logger.Info("event received")
	}
}

func (h *Handler) HandleGetDeployment(w http.ResponseWriter, r *http.Request) {
	service := chi.URLParam(r, "service")
	if service == "" {
		respondError(w, http.StatusBadRequest, errInvalidRequest, "Service name is required")
		return
	}

	environment := r.URL.Query().Get("environment")

	var (
		deployment *models.Deployment
		err        error
	)
	if environment == "" {
		deployment, err = h.storage.GetLatestDeployment(r.Context(), service)
	} else {
		deployment, err = h.storage.GetDeployment(r.Context(), service, environment)
	}

	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			message := fmt.Sprintf("No deployment found for service '%s'", service)
			if environment != "" {
				message += fmt.Sprintf(" in environment '%s'", environment)
			}
			respondError(w, http.StatusNotFound, errNotFound, message)
			return
		}
		h.logger.Error("failed to get deployment", "error", err, "service", service, "environment", environment)
		respondError(w, http.StatusInternalServerError, errStorage, "Failed to retrieve deployment")
		return
	}

	type deploymentResponse struct {
		Service    string             `json:"service"`
		Deployment *models.Deployment `json:"deployment"`
		Links      map[string]string  `json:"links"`
	}

	respondJSON(w, http.StatusOK, deploymentResponse{
		Service:    service,
		Deployment: deployment,
		Links: map[string]string{
			deploymentLinkSelf:  fmt.Sprintf("/api/v1/deployments/%s?environment=%s", service, deployment.Environment),
			deploymentLinkEvent: "/api/v1/events/" + deployment.EventID,
			deploymentLinkChain: "/api/v1/chain/" + deployment.EventID,
		},
	})
}

func (h *Handler) HandleListDeployments(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	filter := storage.ListFilter{
		Environment: query.Get("environment"),
		Limit:       defaultListLimit,
	}

	if limitStr := query.Get("limit"); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil || limit < 1 {
			respondError(w, http.StatusBadRequest, errInvalidRequest, "Invalid limit parameter: must be a positive integer")
			return
		}
		if limit > maxListLimit {
			limit = maxListLimit
		}
		filter.Limit = limit
	}

	if sinceStr := query.Get("since"); sinceStr != "" {
		since, err := time.Parse(time.RFC3339, sinceStr)
		if err != nil {
			respondError(w, http.StatusBadRequest, errInvalidRequest, "Invalid since parameter: must be an RFC 3339 timestamp")
			return
		}
		filter.Since = since
	}

	deployments, err := h.storage.ListDeployments(r.Context(), filter)
	if err != nil {
		h.logger.Error("failed to list deployments", "error", err, "environment", filter.Environment)
		respondError(w, http.StatusInternalServerError, errStorage, "Failed to list deployments")
		return
	}

	type listResponse struct {
		Deployments []*models.Deployment `json:"deployments"`
		Total       int                  `json:"total"`
		Limit       int                  `json:"limit"`
	}

	respondJSON(w, http.StatusOK, listResponse{
		Deployments: deployments,
		Total:       len(deployments),
		Limit:       filter.Limit,
	})
}

func (h *Handler) HandleGetEvent(w http.ResponseWriter, r *http.Request) {
	eventID := chi.URLParam(r, "eventId")
	if eventID == "" {
		respondError(w, http.StatusBadRequest, errInvalidRequest, "Event ID is required")
		return
	}

	event, err := h.storage.GetEvent(r.Context(), eventID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			respondError(w, http.StatusNotFound, errNotFound, fmt.Sprintf("Event not found: %s", eventID))
			return
		}
		h.logger.Error("failed to get event", "error", err, "event_id", eventID)
		respondError(w, http.StatusInternalServerError, errStorage, "Failed to retrieve event")
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{"event": event})
}

func (h *Handler) HandleGetChain(w http.ResponseWriter, r *http.Request) {
	eventID := chi.URLParam(r, "eventId")
	if eventID == "" {
		respondError(w, http.StatusBadRequest, errInvalidRequest, "Event ID is required")
		return
	}

	chain, err := h.correlator.BuildChain(r.Context(), eventID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			respondError(w, http.StatusNotFound, errNotFound, fmt.Sprintf("Event not found: %s", eventID))
			return
		}
		h.logger.Error("failed to build chain", "error", err, "event_id", eventID)
		respondError(w, http.StatusInternalServerError, errCorrelation, "Failed to build event chain")
		return
	}

	respondJSON(w, http.StatusOK, chain)
}

func (h *Handler) HandleHealth(w http.ResponseWriter, r *http.Request) {
	type healthResponse struct {
		Status  string            `json:"status"`
		Version string            `json:"version"`
		Uptime  string            `json:"uptime"`
		Checks  map[string]string `json:"checks"`
	}

	ctx, cancel := context.WithTimeout(r.Context(), healthCheckTimeout)
	defer cancel()

	status := statusHealthy
	code := http.StatusOK
	checks := map[string]string{"storage": "ok"}

	if err := h.storage.Ping(ctx); err != nil {
		status = statusUnhealthy
		code = http.StatusServiceUnavailable
		checks["storage"] = err.Error()
		h.logger.Warn("health check failed", "check", "storage", "error", err)
	}

	respondJSON(w, code, healthResponse{
		Status:  status,
		Version: h.version,
		Uptime:  time.Since(h.startTime).Round(time.Second).String(),
		Checks:  checks,
	})
}
