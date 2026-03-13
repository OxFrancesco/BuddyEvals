package main

import (
	"os"
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

func TestNewFormKeyMapAddsSelectAllShortcut(t *testing.T) {
	keymap := newFormKeyMap()

	assertStringSliceEqual(t, keymap.MultiSelect.SelectAll.Keys(), []string{"A", "ctrl+a"})
	assertStringSliceEqual(t, keymap.MultiSelect.SelectNone.Keys(), []string{"A", "ctrl+a"})

	if help := keymap.MultiSelect.SelectAll.Help(); help.Key != "A" {
		t.Fatalf("expected select-all help key A, got %q", help.Key)
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

func TestResolvePromptTemplatePrefersPromptTextWhenPromptsReordered(t *testing.T) {
	prompts := PromptJSON{
		{ID: "prompt-b", Title: "Prompt B", Prompt: "prompt-b", Track: PromptTrackWeb, Deterministic: true},
		{ID: "prompt-a", Title: "Prompt A", Prompt: "prompt-a", Track: PromptTrackWeb, Deterministic: true},
		{ID: "prompt-c", Title: "Prompt C", Prompt: "prompt-c", Track: PromptTrackWeb, Deterministic: true},
	}
	templates := []PromptTemplate{
		{
			Name: "quick",
			Prompts: []PromptTemplateEntry{
				{PromptNumber: 1, PromptText: "prompt-a"},
				{PromptNumber: 3, PromptText: "prompt-c"},
			},
		},
	}

	template, indices, err := resolvePromptTemplate("quick", templates, prompts)
	if err != nil {
		t.Fatalf("resolvePromptTemplate returned error: %v", err)
	}
	if template.Name != "quick" {
		t.Fatalf("expected template name quick, got %q", template.Name)
	}

	assertIntSliceEqual(t, indices, []int{1, 2})
}

func TestResolvePromptTemplateFallsBackToPromptNumber(t *testing.T) {
	prompts := PromptJSON{
		{ID: "prompt-a", Title: "Prompt A", Prompt: "prompt-a", Track: PromptTrackWeb, Deterministic: true},
		{ID: "prompt-b", Title: "Prompt B", Prompt: "prompt-b", Track: PromptTrackWeb, Deterministic: true},
		{ID: "prompt-c", Title: "Prompt C", Prompt: "prompt-c", Track: PromptTrackWeb, Deterministic: true},
	}
	templates := []PromptTemplate{
		{
			Name: "fallback",
			Prompts: []PromptTemplateEntry{
				{PromptNumber: 2},
				{PromptNumber: 3},
			},
		},
	}

	_, indices, err := resolvePromptTemplate("fallback", templates, prompts)
	if err != nil {
		t.Fatalf("resolvePromptTemplate returned error: %v", err)
	}

	assertIntSliceEqual(t, indices, []int{1, 2})
}

func TestBuildResumeTasksKeepsPerEvalModelsWithoutOverride(t *testing.T) {
	folders := []EvalFolder{
		{
			Path:         "evals/run-1",
			Prompt:       "prompt-a",
			PromptNumber: 1,
			Result:       &EvalResultFile{Model: "openrouter/a"},
		},
		{
			Path:         "evals/run-2",
			Prompt:       "prompt-b",
			PromptNumber: 2,
			Result:       &EvalResultFile{Model: "openrouter/b"},
		},
	}

	tasks := buildResumeTasks(folders, []int{0, 1}, "")
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}
	if tasks[0].Model != "openrouter/a" {
		t.Fatalf("expected first task model openrouter/a, got %q", tasks[0].Model)
	}
	if tasks[1].Model != "openrouter/b" {
		t.Fatalf("expected second task model openrouter/b, got %q", tasks[1].Model)
	}
}

func TestBuildResumeTasksAppliesOverrideToAll(t *testing.T) {
	folders := []EvalFolder{
		{
			Path:         "evals/run-1",
			Prompt:       "prompt-a",
			PromptNumber: 1,
			Result:       &EvalResultFile{Model: "openrouter/a"},
		},
		{
			Path:         "evals/run-2",
			Prompt:       "prompt-b",
			PromptNumber: 2,
			Result:       &EvalResultFile{Model: "openrouter/b"},
		},
	}

	tasks := buildResumeTasks(folders, []int{0, 1}, "openrouter/shared")
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}
	if tasks[0].Model != "openrouter/shared" || tasks[1].Model != "openrouter/shared" {
		t.Fatalf("expected override model to be applied to all tasks, got %q and %q", tasks[0].Model, tasks[1].Model)
	}
}

func TestParsePromptIndicesDeduplicatesAndPreservesOrder(t *testing.T) {
	indices, err := parsePromptIndices("2,1,2,3", 4)
	if err != nil {
		t.Fatalf("parsePromptIndices returned error: %v", err)
	}

	assertIntSliceEqual(t, indices, []int{1, 0, 2})
}

func TestNormalizeRunMode(t *testing.T) {
	mode, err := normalizeRunMode("Parallel")
	if err != nil {
		t.Fatalf("normalizeRunMode returned error: %v", err)
	}
	if mode != "parallel" {
		t.Fatalf("expected parallel, got %q", mode)
	}

	mode, err = normalizeRunMode("")
	if err != nil {
		t.Fatalf("normalizeRunMode returned error for empty input: %v", err)
	}
	if mode != "sequential" {
		t.Fatalf("expected empty mode to default to sequential, got %q", mode)
	}

	if _, err := normalizeRunMode("turbo"); err == nil {
		t.Fatalf("expected invalid mode to return an error")
	}
}

func TestParseBunSDKJSONOutputParsesRunEvalResponse(t *testing.T) {
	var response sdkRunEvalResponse
	err := parseBunSDKJSONOutput(
		[]byte(`{"success":true,"sessionID":"session-1","completedBy":"session.idle","durationMs":1250}`),
		&response,
	)
	if err != nil {
		t.Fatalf("parseBunSDKJSONOutput returned error: %v", err)
	}

	if !response.Success {
		t.Fatalf("expected parsed response to be successful")
	}
	if response.SessionID != "session-1" {
		t.Fatalf("expected session-1, got %q", response.SessionID)
	}
	if response.CompletedBy != "session.idle" {
		t.Fatalf("expected completedBy session.idle, got %q", response.CompletedBy)
	}
	if response.DurationMs != 1250 {
		t.Fatalf("expected duration 1250ms, got %d", response.DurationMs)
	}
}

func TestGetProvidersDataParsesBunHelperResponse(t *testing.T) {
	origRunner := runBunSDKCommand
	t.Cleanup(func() {
		runBunSDKCommand = origRunner
	})

	runBunSDKCommand = func(dir string, args []string, input []byte) ([]byte, []byte, error) {
		return []byte(`{"providers":[{"id":"openrouter","name":"OpenRouter","models":{"glm-5":{}}}],"default":{"openrouter":"glm-5"}}`), nil, nil
	}

	providers, err := getProvidersData()
	if err != nil {
		t.Fatalf("getProvidersData returned error: %v", err)
	}

	if len(providers.Providers) != 1 {
		t.Fatalf("expected 1 provider, got %d", len(providers.Providers))
	}
	if providers.Providers[0].ID != "openrouter" {
		t.Fatalf("expected provider openrouter, got %q", providers.Providers[0].ID)
	}
	if providers.Default["openrouter"] != "glm-5" {
		t.Fatalf("expected default model glm-5, got %q", providers.Default["openrouter"])
	}
}

func TestRunAgentWithRetryRetriesTransientBunSDKFailures(t *testing.T) {
	origRunner := runBunSDKCommand
	t.Cleanup(func() {
		runBunSDKCommand = origRunner
	})

	attempts := 0
	runBunSDKCommand = func(dir string, args []string, input []byte) ([]byte, []byte, error) {
		attempts++
		if attempts == 1 {
			return []byte(`{"success":false,"error":"no agent activity for 180s","durationMs":1000}`), nil, nil
		}
		return []byte(`{"success":true,"sessionID":"session-2","completedBy":"session.idle","durationMs":1500}`), nil, nil
	}

	folder := t.TempDir()
	if err := os.WriteFile(folder+"/.run", []byte("#!/bin/sh\nexit 0\n"), 0644); err != nil {
		t.Fatalf("writing .run: %v", err)
	}

	result := runAgentWithRetry(EvalTask{
		Prompt:       "prompt",
		Track:        PromptTrackWeb,
		PromptNumber: 1,
		Folder:       folder,
	}, 0, "openrouter/glm-5")
	if !result.Success {
		t.Fatalf("expected retry to succeed, got error %q", result.Error)
	}
	if attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts)
	}
}

func TestRunAgentPreservesModelNotFoundErrorFromBunSDK(t *testing.T) {
	origRunner := runBunSDKCommand
	t.Cleanup(func() {
		runBunSDKCommand = origRunner
	})

	runBunSDKCommand = func(dir string, args []string, input []byte) ([]byte, []byte, error) {
		return []byte(`{"success":false,"error":"Model not found. Did you mean: openrouter/glm-5?","durationMs":1000}`), nil, nil
	}

	folder := t.TempDir()
	if err := os.WriteFile(folder+"/.run", []byte("#!/bin/sh\nexit 0\n"), 0644); err != nil {
		t.Fatalf("writing .run: %v", err)
	}

	result := runAgent(EvalTask{
		Prompt:       "prompt",
		Track:        PromptTrackWeb,
		PromptNumber: 1,
		Folder:       folder,
	}, 0, "openrouter/missing-model")
	if result.Error != "Model not found. Did you mean: openrouter/glm-5?" {
		t.Fatalf("expected helper error to be preserved, got %q", result.Error)
	}
}

func TestRunAgentMarksValidationFailuresAsUnsuccessful(t *testing.T) {
	origRunner := runBunSDKCommand
	t.Cleanup(func() {
		runBunSDKCommand = origRunner
	})

	runBunSDKCommand = func(dir string, args []string, input []byte) ([]byte, []byte, error) {
		return []byte(`{"success":true,"sessionID":"session-3","completedBy":"session.idle","durationMs":750}`), nil, nil
	}

	folder := t.TempDir()
	result := runAgent(EvalTask{
		Prompt:       "prompt",
		Track:        PromptTrackWeb,
		PromptNumber: 1,
		Folder:       folder,
	}, 0, "openrouter/glm-5")

	if !result.AgentSuccess {
		t.Fatalf("expected agent success to be recorded")
	}
	if result.Success {
		t.Fatalf("expected overall success to fail when validation fails")
	}
	if result.Validation.ValidationSuccess() {
		t.Fatalf("expected validation to fail without a .run contract")
	}
}

func assertIntSliceEqual(t *testing.T, got, want []int) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected %v, got %v", want, got)
		}
	}
}

func assertStringSliceEqual(t *testing.T, got, want []string) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected %v, got %v", want, got)
		}
	}
}
