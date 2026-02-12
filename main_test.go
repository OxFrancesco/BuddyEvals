package main

import "testing"

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
