package main

import (
	"os"
	"strings"
	"testing"
)

func TestLoadPromptsSupportsLegacyAndStructuredEntries(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	data := `[
  "Build a simple Bun page",
  {
    "id": "fixture-task",
    "title": "Fixture Task",
    "prompt": "Summarize {{INPUT_FILE}} into a local report.",
    "track": "integration",
    "deterministic": false,
    "fixtures": ["./fixtures/sample.json"],
    "placeholders": {"input_file": "fixtures/sample.json"}
  }
]`
	if err := os.WriteFile(promptsFile, []byte(data), 0644); err != nil {
		t.Fatalf("writing prompts.json: %v", err)
	}

	prompts, err := loadPrompts()
	if err != nil {
		t.Fatalf("loadPrompts returned error: %v", err)
	}
	if len(prompts) != 2 {
		t.Fatalf("expected 2 prompts, got %d", len(prompts))
	}

	if prompts[0].Prompt != "Build a simple Bun page" {
		t.Fatalf("expected legacy prompt body to be preserved, got %q", prompts[0].Prompt)
	}
	if prompts[0].ID == "" || prompts[0].Title == "" {
		t.Fatalf("expected inferred id/title for legacy prompt, got id=%q title=%q", prompts[0].ID, prompts[0].Title)
	}
	if prompts[0].Track != PromptTrackWeb {
		t.Fatalf("expected legacy prompt to infer web track, got %q", prompts[0].Track)
	}
	if !prompts[0].Deterministic {
		t.Fatalf("expected legacy web prompt to default to deterministic")
	}

	if prompts[1].ID != "fixture-task" {
		t.Fatalf("expected structured prompt id fixture-task, got %q", prompts[1].ID)
	}
	if prompts[1].Track != PromptTrackIntegration {
		t.Fatalf("expected integration track, got %q", prompts[1].Track)
	}
	if prompts[1].Deterministic {
		t.Fatalf("expected structured prompt deterministic=false to be preserved")
	}
	if len(prompts[1].Fixtures) != 1 || prompts[1].Fixtures[0] != "fixtures/sample.json" {
		t.Fatalf("expected normalized fixture path, got %#v", prompts[1].Fixtures)
	}
	if prompts[1].Placeholders["INPUT_FILE"] != "fixtures/sample.json" {
		t.Fatalf("expected placeholder keys to be uppercased, got %#v", prompts[1].Placeholders)
	}
}

func TestRenderPromptInjectsSharedPreambleAndPlaceholders(t *testing.T) {
	def := PromptDefinition{
		ID:            "sample-task",
		Title:         "Sample Task",
		Prompt:        "Use {{FIXTURE_NAME}} inside {{ASSIGNED_FOLDER}} for {{MODEL_NAME}}.",
		Track:         PromptTrackWeb,
		Deterministic: true,
		Fixtures:      []string{"fixtures/sample.json"},
		Placeholders: map[string]string{
			"fixture_name": "fixtures/sample.json",
		},
	}

	rendered := renderPrompt(def, "glm-5", "evals/2026-03-13_p1_0_glm-5")

	for _, needle := range []string{
		"Task ID: sample-task",
		"Task Title: Sample Task",
		"Track: web",
		"Assigned folder: 2026-03-13_p1_0_glm-5",
		"Use Bun for JavaScript and TypeScript tooling.",
		"fixtures/sample.json",
		"openrouter/glm-5",
		"Acceptance checklist:",
	} {
		if !strings.Contains(rendered, needle) {
			t.Fatalf("expected rendered prompt to contain %q", needle)
		}
	}

	if strings.Contains(rendered, "{{FIXTURE_NAME}}") || strings.Contains(rendered, "{{ASSIGNED_FOLDER}}") || strings.Contains(rendered, "{{MODEL_NAME}}") {
		t.Fatalf("expected placeholders to be rendered, got:\n%s", rendered)
	}
}
