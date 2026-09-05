/*
Unit tests for event type helpers.
*/
package cdevents

import "testing"

func TestShortType(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"dev.cdevents.service.deployed.0.1.1", "service.deployed"},
		{"dev.cdevents.service.deployed.v1", "service.deployed"},
		{"dev.cdevents.service.deployed", "service.deployed"},
		{"dev.cdevents.pipelinerun.finished.0.2.0", "pipelinerun.finished"},
		{"service.deployed.0.1.1", "service.deployed"},
		{"service.deployed", "service.deployed"},
		{"deployed", "deployed"},
		{"", ""},
		{"  dev.cdevents.change.merged.0.4.1  ", "change.merged"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := ShortType(tt.input); got != tt.want {
				t.Errorf("ShortType(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsDeployment(t *testing.T) {
	deployment := &CDEvent{Context: EventContext{Type: "dev.cdevents.service.deployed.0.1.1"}}
	rollback := &CDEvent{Context: EventContext{Type: "dev.cdevents.service.rolledback.0.1.1"}}
	pipeline := &CDEvent{Context: EventContext{Type: "dev.cdevents.pipelinerun.finished.0.2.0"}}

	if !deployment.IsDeployment() || !rollback.IsDeployment() {
		t.Error("service.deployed and service.rolledback should be deployments")
	}
	if pipeline.IsDeployment() {
		t.Error("pipelinerun.finished should not be a deployment")
	}
	if (&StoredEvent{Type: "dev.cdevents.change.merged.0.4.1"}).ShortType() != KindChangeMerged {
		t.Error("StoredEvent.ShortType should reduce the type")
	}
}
