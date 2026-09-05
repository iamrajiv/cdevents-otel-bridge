/*
Package cdevents provides parsing utilities for CDEvents.

This file holds the small, dependency-free helpers used to pull values out of
the loosely typed subject.content and customData maps. CI/CD tools disagree
about where metadata such as the commit SHA belongs, so every lookup accepts
a list of candidate keys and a list of candidate maps and returns the first
non-empty string it finds.
*/
package cdevents

import (
	"regexp"
	"strings"
)

var trailingVersionPattern = regexp.MustCompile(`(?:^|[-_])(v?\d+\.\d+(?:\.\d+)?(?:[-+][0-9A-Za-z.-]+)?)$`)

func getStringField(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	if val, ok := m[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

func getNestedString(m map[string]any, keys ...string) string {
	if m == nil || len(keys) == 0 {
		return ""
	}

	current := m
	for i, key := range keys {
		val, ok := current[key]
		if !ok {
			return ""
		}

		if i == len(keys)-1 {
			if str, ok := val.(string); ok {
				return str
			}
			return ""
		}

		nextMap, ok := val.(map[string]any)
		if !ok {
			return ""
		}
		current = nextMap
	}

	return ""
}

func stringFrom(keys []string, maps ...map[string]any) string {
	for _, m := range maps {
		for _, key := range keys {
			if v := getStringField(m, key); v != "" {
				return v
			}
		}
	}
	return ""
}

func identityFrom(keys []string, maps ...map[string]any) string {
	for _, m := range maps {
		if m == nil {
			continue
		}
		for _, key := range keys {
			val, ok := m[key]
			if !ok {
				continue
			}
			switch v := val.(type) {
			case string:
				if v != "" {
					return v
				}
			case map[string]any:
				if s := stringFrom([]string{"id", "url", "name"}, v); s != "" {
					return s
				}
			}
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func versionFromArtifactID(artifactID string) string {
	artifactID = strings.TrimSpace(artifactID)
	if artifactID == "" {
		return ""
	}

	if at := strings.LastIndex(artifactID, "@"); at >= 0 && at < len(artifactID)-1 {
		return artifactID[at+1:]
	}

	if strings.HasPrefix(artifactID, "pkg:") {
		return ""
	}

	if colon := strings.LastIndex(artifactID, ":"); colon >= 0 && colon < len(artifactID)-1 {
		return artifactID[colon+1:]
	}

	if match := trailingVersionPattern.FindStringSubmatch(artifactID); len(match) == 2 {
		return match[1]
	}

	return ""
}
