/*
Unit tests for the map lookup helpers and artifact version parsing.
*/
package cdevents

import "testing"

func TestGetStringField(t *testing.T) {
	tests := []struct {
		name string
		m    map[string]any
		key  string
		want string
	}{
		{name: "existing string field", m: map[string]any{"key": "value"}, key: "key", want: "value"},
		{name: "missing field", m: map[string]any{"other": "value"}, key: "key", want: ""},
		{name: "non-string field", m: map[string]any{"key": 123}, key: "key", want: ""},
		{name: "nil map", m: nil, key: "key", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getStringField(tt.m, tt.key); got != tt.want {
				t.Errorf("getStringField() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetNestedString(t *testing.T) {
	tests := []struct {
		name string
		m    map[string]any
		keys []string
		want string
	}{
		{name: "single level", m: map[string]any{"key": "value"}, keys: []string{"key"}, want: "value"},
		{name: "nested two levels", m: map[string]any{"env": map[string]any{"id": "production"}}, keys: []string{"env", "id"}, want: "production"},
		{name: "missing nested key", m: map[string]any{"env": map[string]any{"name": "prod"}}, keys: []string{"env", "id"}, want: ""},
		{name: "intermediate is not a map", m: map[string]any{"env": "prod"}, keys: []string{"env", "id"}, want: ""},
		{name: "nil map", m: nil, keys: []string{"key"}, want: ""},
		{name: "empty keys", m: map[string]any{"key": "value"}, keys: []string{}, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getNestedString(tt.m, tt.keys...); got != tt.want {
				t.Errorf("getNestedString() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStringFromPrefersEarlierMapsAndKeys(t *testing.T) {
	custom := map[string]any{"commit": "from-custom"}
	content := map[string]any{"commitSha": "from-content", "commit": "ignored"}

	if got := stringFrom([]string{"commitSha", "commit"}, custom, content); got != "from-custom" {
		t.Errorf("stringFrom() = %q, want from-custom (first map wins)", got)
	}
	if got := stringFrom([]string{"commitSha", "commit"}, nil, content); got != "from-content" {
		t.Errorf("stringFrom() = %q, want from-content (first key wins)", got)
	}
	if got := stringFrom([]string{"missing"}, custom, content); got != "" {
		t.Errorf("stringFrom() = %q, want empty", got)
	}
}

func TestIdentityFrom(t *testing.T) {
	tests := []struct {
		name string
		maps []map[string]any
		want string
	}{
		{name: "string value", maps: []map[string]any{{"repository": "github.com/o/r"}}, want: "github.com/o/r"},
		{name: "object with id", maps: []map[string]any{{"repository": map[string]any{"id": "o/r", "url": "https://x"}}}, want: "o/r"},
		{name: "object with url only", maps: []map[string]any{{"repository": map[string]any{"url": "https://x"}}}, want: "https://x"},
		{name: "object with name only", maps: []map[string]any{{"repository": map[string]any{"name": "r"}}}, want: "r"},
		{name: "unsupported type", maps: []map[string]any{{"repository": 42}}, want: ""},
		{name: "nil map skipped", maps: []map[string]any{nil, {"repository": "r"}}, want: "r"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := identityFrom([]string{"repository"}, tt.maps...); got != tt.want {
				t.Errorf("identityFrom() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestVersionFromArtifactID(t *testing.T) {
	tests := []struct {
		artifactID string
		want       string
	}{
		{"", ""},
		{"my-app:v1.2.3", "v1.2.3"},
		{"myorg/api-service:v1.2.3", "v1.2.3"},
		{"registry.io:5000/team/app:2.0.1", "2.0.1"},
		{"pkg:docker/myapp@sha256:abc123", "sha256:abc123"},
		{"pkg:oci/app@v1.2.3", "v1.2.3"},
		{"pkg:docker/myapp", ""},
		{"pkg-sample-app-v1.2.3", "v1.2.3"},
		{"sample_app_1.4.0-rc.1", "1.4.0-rc.1"},
		{"just-a-name", ""},
		{"trailing-colon:", ""},
	}

	for _, tt := range tests {
		t.Run(tt.artifactID, func(t *testing.T) {
			if got := versionFromArtifactID(tt.artifactID); got != tt.want {
				t.Errorf("versionFromArtifactID(%q) = %q, want %q", tt.artifactID, got, tt.want)
			}
		})
	}
}
