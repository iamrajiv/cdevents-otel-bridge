/*
Mock deployer: replays a chain of CDEvents into the bridge.

The tool plays the role of a CI/CD system. It loads events.json (a JSON
array of CDEvents), rewrites their timestamps so the sequence ends "now"
while keeping the original spacing, waits for the bridge to report healthy,
and posts the events in order with a short pause between them. The default
file contains a change.merged -> pipelinerun.finished -> service.deployed
-> incident.detected chain, which exercises deployment lookup, trace
enrichment and chain reconstruction end to end.

Environment variables:

	BRIDGE_URL     bridge base URL (default http://localhost:8080)
	EVENTS_FILE    path to the events file (default events.json)
	EVENT_DELAY    pause between events, Go duration (default 1s)
	WAIT_TIMEOUT   how long to wait for the bridge to become healthy
	               (default 60s)
*/
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	defaultBridgeURL  = "http://localhost:8080"
	defaultEventsFile = "events.json"
	defaultDelay      = time.Second
	defaultWait       = 60 * time.Second
	requestTimeout    = 10 * time.Second
)

type event struct {
	raw       map[string]any
	id        string
	eventType string
	subjectID string
	timestamp time.Time
}

func main() {
	if err := run(); err != nil {
		log.Fatalf("mock-deployer: %v", err)
	}
}

func run() error {
	bridgeURL := strings.TrimRight(getEnv("BRIDGE_URL", defaultBridgeURL), "/")
	eventsFile := getEnv("EVENTS_FILE", defaultEventsFile)

	delay, err := time.ParseDuration(getEnv("EVENT_DELAY", defaultDelay.String()))
	if err != nil {
		return fmt.Errorf("invalid EVENT_DELAY: %w", err)
	}
	waitTimeout, err := time.ParseDuration(getEnv("WAIT_TIMEOUT", defaultWait.String()))
	if err != nil {
		return fmt.Errorf("invalid WAIT_TIMEOUT: %w", err)
	}

	events, err := loadEvents(eventsFile)
	if err != nil {
		return err
	}
	shiftTimestamps(events, time.Now().UTC())

	log.Printf("bridge: %s", bridgeURL)
	log.Printf("loaded %d events from %s", len(events), eventsFile)

	client := &http.Client{Timeout: requestTimeout}
	if err := waitForBridge(client, bridgeURL, waitTimeout); err != nil {
		return err
	}

	var deployedService, incidentID string
	for i, ev := range events {
		log.Printf("[%d/%d] %s (id=%s subject=%s)", i+1, len(events), ev.eventType, ev.id, ev.subjectID)
		if err := sendEvent(client, bridgeURL, ev); err != nil {
			return fmt.Errorf("send event %s: %w", ev.id, err)
		}

		if strings.Contains(ev.eventType, ".service.deployed.") {
			deployedService = serviceName(ev)
		}
		if strings.Contains(ev.eventType, ".incident.detected.") {
			incidentID = ev.id
		}

		if i < len(events)-1 && delay > 0 {
			time.Sleep(delay)
		}
	}

	log.Printf("all %d events accepted", len(events))
	if deployedService != "" {
		log.Printf("current deployment:  curl -s %s/api/v1/deployments/%s | jq", bridgeURL, deployedService)
	}
	if incidentID != "" {
		log.Printf("incident to commit:  curl -s %s/api/v1/chain/%s | jq", bridgeURL, incidentID)
	}
	return nil
}

func loadEvents(path string) ([]*event, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read events file: %w", err)
	}

	var raws []map[string]any
	if err := json.Unmarshal(data, &raws); err != nil {
		return nil, fmt.Errorf("parse events file: %w", err)
	}
	if len(raws) == 0 {
		return nil, errors.New("events file contains no events")
	}

	events := make([]*event, 0, len(raws))
	for i, raw := range raws {
		ctx, _ := raw["context"].(map[string]any)
		subject, _ := raw["subject"].(map[string]any)
		if ctx == nil {
			return nil, fmt.Errorf("event %d has no context", i)
		}

		ts, err := time.Parse(time.RFC3339, stringValue(ctx["timestamp"]))
		if err != nil {
			return nil, fmt.Errorf("event %d has an invalid timestamp: %w", i, err)
		}

		events = append(events, &event{
			raw:       raw,
			id:        stringValue(ctx["id"]),
			eventType: stringValue(ctx["type"]),
			subjectID: stringValue(subject["id"]),
			timestamp: ts,
		})
	}
	return events, nil
}

func shiftTimestamps(events []*event, now time.Time) {
	if len(events) == 0 {
		return
	}
	last := events[len(events)-1].timestamp
	for _, ev := range events {
		shifted := now.Add(ev.timestamp.Sub(last))
		ev.timestamp = shifted
		if ctx, ok := ev.raw["context"].(map[string]any); ok {
			ctx["timestamp"] = shifted.Format(time.RFC3339)
		}
	}
}

func waitForBridge(client *http.Client, bridgeURL string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	healthURL := bridgeURL + "/api/v1/health"

	for attempt := 1; ; attempt++ {
		err := checkHealth(client, healthURL)
		if err == nil {
			log.Printf("bridge is healthy")
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("bridge did not become healthy within %s: %w", timeout, err)
		}
		if attempt == 1 || attempt%5 == 0 {
			log.Printf("waiting for bridge (%v)", err)
		}
		time.Sleep(2 * time.Second)
	}
}

func checkHealth(client *http.Client, healthURL string) error {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health returned %d", resp.StatusCode)
	}
	return nil
}

func sendEvent(client *http.Client, bridgeURL string, ev *event) error {
	body, err := json.Marshal(ev.raw)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, bridgeURL+"/api/v1/events", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated, http.StatusAccepted:
		log.Printf("      accepted (%d): %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
		return nil
	default:
		return fmt.Errorf("bridge returned %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
}

func serviceName(ev *event) string {
	if subject, ok := ev.raw["subject"].(map[string]any); ok {
		if content, ok := subject["content"].(map[string]any); ok {
			if service, ok := content["service"].(map[string]any); ok {
				if name := stringValue(service["name"]); name != "" {
					return name
				}
			}
		}
	}
	return ev.subjectID
}

func stringValue(v any) string {
	s, _ := v.(string)
	return s
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
