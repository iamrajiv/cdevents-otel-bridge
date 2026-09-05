/*
Unit tests for the shared data models: link decoding leniency and the
duration helpers on Incident and PipelineRun.
*/
package models

import (
	"encoding/json"
	"testing"
	"time"
)

func TestLinkUnmarshalJSON(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  Link
	}{
		{name: "spec shape", input: `{"linkType": "triggeredBy", "linkId": "evt-1"}`, want: Link{LinkType: "triggeredBy", LinkID: "evt-1"}},
		{name: "legacy shape", input: `{"type": "TRIGGERED_BY", "target": "evt-1"}`, want: Link{LinkType: "TRIGGERED_BY", LinkID: "evt-1"}},
		{name: "native shape with kind", input: `{"linkType": "RELATION", "linkKind": "TRIGGER", "from": {"contextId": "evt-1"}}`, want: Link{LinkType: "TRIGGER", LinkID: "evt-1"}},
		{name: "native shape without kind", input: `{"linkType": "PATH", "from": {"contextId": "evt-1"}}`, want: Link{LinkType: "PATH", LinkID: "evt-1"}},
		{name: "empty object", input: `{}`, want: Link{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got Link
			if err := json.Unmarshal([]byte(tt.input), &got); err != nil {
				t.Fatalf("Unmarshal() error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Unmarshal() = %+v, want %+v", got, tt.want)
			}
		})
	}

	var bad Link
	if err := json.Unmarshal([]byte(`"not-an-object"`), &bad); err == nil {
		t.Error("expected error for non-object link")
	}
}

func TestLinkMarshalRoundTrip(t *testing.T) {
	data, err := json.Marshal(Link{LinkType: "causedBy", LinkID: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"linkType":"causedBy","linkId":"x"}` {
		t.Errorf("unexpected encoding: %s", data)
	}
}

func TestIncidentDuration(t *testing.T) {
	detected := time.Date(2024, 1, 15, 11, 0, 0, 0, time.UTC)
	incident := &Incident{DetectedAt: detected}
	if incident.Duration() != nil {
		t.Error("open incident should have nil duration")
	}

	resolved := detected.Add(45 * time.Minute)
	incident.ResolvedAt = &resolved
	if d := incident.Duration(); d == nil || *d != 45*time.Minute {
		t.Errorf("Duration() = %v, want 45m", d)
	}
}

func TestPipelineRunDuration(t *testing.T) {
	started := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	run := &PipelineRun{StartedAt: started}
	if run.Duration() != nil {
		t.Error("running pipeline should have nil duration")
	}

	finished := started.Add(90 * time.Second)
	run.FinishedAt = &finished
	if d := run.Duration(); d == nil || *d != 90*time.Second {
		t.Errorf("Duration() = %v, want 90s", d)
	}
}
