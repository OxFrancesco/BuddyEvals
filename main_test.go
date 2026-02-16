package main

import (
	"strings"
	"testing"
	"time"
)

func TestFilterModelsMatchesHyphenWithSpaceQuery(t *testing.T) {
	models := []string{
		"openrouter/claude-sonnet-4",
		"openrouter/glm-4.5",
		"openrouter/glm-5",
		"openrouter/gpt-5",
	}

	filtered := filterModels(models, "glm 5")
	if len(filtered) == 0 {
		t.Fatalf("expected matches for query %q", "glm 5")
	}

	if filtered[0] != "openrouter/glm-5" {
		t.Fatalf("expected top match to be openrouter/glm-5, got %q", filtered[0])
	}
}

func TestFilterModelsMatchesMultiTokenSeparatedName(t *testing.T) {
	models := []string{
		"anthropic/claude-3.5-haiku",
		"anthropic/claude-sonnet-4",
		"openrouter/gpt-5",
	}

	filtered := filterModels(models, "claude sonnet 4")
	if len(filtered) == 0 {
		t.Fatalf("expected matches for query %q", "claude sonnet 4")
	}

	if filtered[0] != "anthropic/claude-sonnet-4" {
		t.Fatalf("expected top match to be anthropic/claude-sonnet-4, got %q", filtered[0])
	}
}

func TestFilterModelsSubsequenceQuery(t *testing.T) {
	models := []string{
		"openrouter/glm-5",
		"openrouter/gpt-5",
	}

	filtered := filterModels(models, "gm5")
	if len(filtered) == 0 {
		t.Fatalf("expected fuzzy subsequence match for query %q", "gm5")
	}

	if filtered[0] != "openrouter/glm-5" {
		t.Fatalf("expected top match to be openrouter/glm-5, got %q", filtered[0])
	}
}

func TestPinSavedModelsMovesSavedToFront(t *testing.T) {
	models := []string{
		"openrouter/gpt-5",
		"openrouter/glm-5",
		"anthropic/claude-sonnet-4",
	}
	saved := map[string]struct{}{
		"openrouter/glm-5": {},
	}

	pinned := pinSavedModels(models, saved)

	if len(pinned) != len(models) {
		t.Fatalf("expected %d models, got %d", len(models), len(pinned))
	}

	if pinned[0] != "openrouter/glm-5" {
		t.Fatalf("expected first model to be pinned saved model, got %q", pinned[0])
	}
}

func TestPinSavedModelIDsMovesSavedToFront(t *testing.T) {
	modelIDs := []string{"gpt-5", "glm-5", "claude-sonnet-4"}
	saved := map[string]struct{}{
		"openrouter/glm-5": {},
	}

	pinned := pinSavedModelIDs("openrouter", modelIDs, saved)
	if pinned[0] != "glm-5" {
		t.Fatalf("expected first modelID to be pinned saved model, got %q", pinned[0])
	}
}

func TestIsTransientEvalError(t *testing.T) {
	cases := []struct {
		errMsg    string
		transient bool
	}{
		{"no agent activity for 180s", true},
		{"event stream error: bufio.Scanner: token too long", true},
		{"agent did not reach idle state", true},
		{"Failed to send prompt: HTTP 401", false},
		{"", false},
	}

	for _, tc := range cases {
		got := isTransientEvalError(tc.errMsg)
		if got != tc.transient {
			t.Fatalf("isTransientEvalError(%q) = %v, want %v", tc.errMsg, got, tc.transient)
		}
	}
}

func TestApplyRuntimeOptions(t *testing.T) {
	origTimeout := inactivityTimeout
	origRetries := transientRetries
	t.Cleanup(func() {
		inactivityTimeout = origTimeout
		transientRetries = origRetries
	})

	applyRuntimeOptions(240, 3)
	if inactivityTimeout != 240*time.Second {
		t.Fatalf("expected inactivity timeout 240s, got %s", inactivityTimeout)
	}
	if transientRetries != 3 {
		t.Fatalf("expected retries 3, got %d", transientRetries)
	}

	applyRuntimeOptions(0, -1)
	if inactivityTimeout != defaultInactivityTimeout {
		t.Fatalf("expected fallback inactivity timeout %s, got %s", defaultInactivityTimeout, inactivityTimeout)
	}
	if transientRetries != defaultTransientRetries {
		t.Fatalf("expected fallback retries %d, got %d", defaultTransientRetries, transientRetries)
	}
}

func TestSanitizeModelForFolder(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"openrouter/z-ai/glm-5", "openrouter-z-ai-glm-5"},
		{"glm-5", "openrouter-glm-5"},
		{" OpenRouter/Model:Name ", "openrouter-model-name"},
		{"", "unknown-model"},
	}

	for _, tc := range cases {
		got := sanitizeModelForFolder(tc.input)
		if got != tc.want {
			t.Fatalf("sanitizeModelForFolder(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestCreateTimestampFolderIncludesModel(t *testing.T) {
	folder := createTimestampFolder(3, 12, "openrouter/z-ai/glm-5")

	if !strings.HasPrefix(folder, "evals/") {
		t.Fatalf("expected folder to start with evals/, got %q", folder)
	}
	if !strings.Contains(folder, "_p12_3_openrouter-z-ai-glm-5") {
		t.Fatalf("expected folder to include prompt number, index, and sanitized model, got %q", folder)
	}
}

func TestParsePromptNumberFromFolder(t *testing.T) {
	if got := parsePromptNumberFromFolder("2026-02-16_09-35-43_p7_3_openrouter-z-ai-glm-5"); got != 7 {
		t.Fatalf("expected prompt number 7, got %d", got)
	}
	if got := parsePromptNumberFromFolder("2026-02-16_09-35-43_3_openrouter-z-ai-glm-5"); got != 0 {
		t.Fatalf("expected 0 for folder without prompt marker, got %d", got)
	}
}
