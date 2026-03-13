package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

type PromptTrack string

const (
	PromptTrackWeb         PromptTrack = "web"
	PromptTrackPython      PromptTrack = "python"
	PromptTrackCLI         PromptTrack = "cli"
	PromptTrackIntegration PromptTrack = "integration"
	PromptTrackMobile      PromptTrack = "mobile"
)

type PromptDefinition struct {
	ID            string            `json:"id,omitempty"`
	Title         string            `json:"title,omitempty"`
	Prompt        string            `json:"prompt"`
	Track         PromptTrack       `json:"track,omitempty"`
	Deterministic bool              `json:"deterministic"`
	Fixtures      []string          `json:"fixtures,omitempty"`
	Placeholders  map[string]string `json:"placeholders,omitempty"`
}

type PromptJSON []PromptDefinition

type PromptTemplateEntry struct {
	PromptID     string `json:"prompt_id,omitempty"`
	PromptNumber int    `json:"prompt_number,omitempty"`
	PromptText   string `json:"prompt_text,omitempty"`
}

type promptDefinitionFile struct {
	ID            string            `json:"id,omitempty"`
	Title         string            `json:"title,omitempty"`
	Prompt        string            `json:"prompt"`
	Track         PromptTrack       `json:"track,omitempty"`
	Deterministic *bool             `json:"deterministic,omitempty"`
	Fixtures      []string          `json:"fixtures,omitempty"`
	Placeholders  map[string]string `json:"placeholders,omitempty"`
}

func loadPrompts() (PromptJSON, error) {
	data, err := os.ReadFile(promptsFile)
	if err != nil {
		if os.IsNotExist(err) {
			return PromptJSON{}, nil
		}
		return nil, err
	}

	if len(data) == 0 {
		return PromptJSON{}, nil
	}

	var rawItems []json.RawMessage
	if err := json.Unmarshal(data, &rawItems); err != nil {
		return nil, err
	}

	prompts := make(PromptJSON, 0, len(rawItems))
	usedIDs := map[string]int{}
	for i, raw := range rawItems {
		def, err := parsePromptDefinition(raw, i, usedIDs)
		if err != nil {
			return nil, err
		}
		prompts = append(prompts, def)
	}

	return prompts, nil
}

func savePrompts(prompts PromptJSON) error {
	normalized := make(PromptJSON, 0, len(prompts))
	usedIDs := map[string]int{}
	for i, prompt := range prompts {
		fileDef := promptDefinitionFile{
			ID:            strings.TrimSpace(prompt.ID),
			Title:         strings.TrimSpace(prompt.Title),
			Prompt:        strings.TrimSpace(prompt.Prompt),
			Track:         normalizePromptTrack(prompt.Track, prompt.Prompt),
			Deterministic: boolPointer(prompt.Deterministic),
			Fixtures:      cloneStringSlice(prompt.Fixtures),
			Placeholders:  cloneStringMap(prompt.Placeholders),
		}

		encoded, err := json.Marshal(fileDef)
		if err != nil {
			return err
		}

		def, err := parsePromptDefinition(encoded, i, usedIDs)
		if err != nil {
			return err
		}
		normalized = append(normalized, def)
	}

	data, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(promptsFile, data, 0644)
}

func parsePromptDefinition(raw json.RawMessage, index int, usedIDs map[string]int) (PromptDefinition, error) {
	var legacy string
	if err := json.Unmarshal(raw, &legacy); err == nil {
		return normalizePromptDefinition(promptDefinitionFile{Prompt: legacy}, index, usedIDs)
	}

	var fileDef promptDefinitionFile
	if err := json.Unmarshal(raw, &fileDef); err != nil {
		return PromptDefinition{}, fmt.Errorf("invalid prompt #%d: %w", index+1, err)
	}

	return normalizePromptDefinition(fileDef, index, usedIDs)
}

func normalizePromptDefinition(raw promptDefinitionFile, index int, usedIDs map[string]int) (PromptDefinition, error) {
	promptText := strings.TrimSpace(raw.Prompt)
	if promptText == "" {
		return PromptDefinition{}, fmt.Errorf("prompt #%d cannot be empty", index+1)
	}

	track := normalizePromptTrack(raw.Track, promptText)
	deterministic := inferPromptDeterminism(track)
	if raw.Deterministic != nil {
		deterministic = *raw.Deterministic
	}

	def := PromptDefinition{
		ID:            ensurePromptID(raw.ID, raw.Title, promptText, index, usedIDs),
		Title:         ensurePromptTitle(raw.Title, promptText, index),
		Prompt:        promptText,
		Track:         track,
		Deterministic: deterministic,
		Fixtures:      normalizeFixturePaths(raw.Fixtures),
		Placeholders:  normalizePlaceholders(raw.Placeholders),
	}

	return def, nil
}

func inferPromptTrack(prompt string) PromptTrack {
	lower := strings.ToLower(prompt)
	switch {
	case strings.Contains(lower, "expo") || strings.Contains(lower, "react native") || strings.Contains(lower, "mobile astrology app") || strings.Contains(lower, "pomodoro timer app"):
		return PromptTrackMobile
	case strings.Contains(lower, "uv init") || strings.Contains(lower, "python") || strings.Contains(lower, "manim"):
		return PromptTrackPython
	case strings.Contains(lower, "cli tool"):
		return PromptTrackCLI
	case strings.Contains(lower, "notion") || strings.Contains(lower, "airtable") || strings.Contains(lower, "linear") || strings.Contains(lower, "github commits") || strings.Contains(lower, "reference post") || strings.Contains(lower, "x.com") || strings.Contains(lower, "nano banana") || strings.Contains(lower, "vision model"):
		return PromptTrackIntegration
	default:
		return PromptTrackWeb
	}
}

func normalizePromptTrack(track PromptTrack, prompt string) PromptTrack {
	switch PromptTrack(strings.TrimSpace(strings.ToLower(string(track)))) {
	case PromptTrackWeb:
		return PromptTrackWeb
	case PromptTrackPython:
		return PromptTrackPython
	case PromptTrackCLI:
		return PromptTrackCLI
	case PromptTrackIntegration:
		return PromptTrackIntegration
	case PromptTrackMobile:
		return PromptTrackMobile
	default:
		return inferPromptTrack(prompt)
	}
}

func inferPromptDeterminism(track PromptTrack) bool {
	return track != PromptTrackIntegration && track != PromptTrackMobile
}

func ensurePromptTitle(rawTitle, promptText string, index int) string {
	if title := strings.TrimSpace(rawTitle); title != "" {
		return title
	}

	preview := promptPreview(promptText, 56)
	if preview == "" {
		return fmt.Sprintf("Prompt %d", index+1)
	}
	return fmt.Sprintf("Prompt %d · %s", index+1, preview)
}

func ensurePromptID(rawID, rawTitle, promptText string, index int, usedIDs map[string]int) string {
	candidate := slugifyIdentifier(strings.TrimSpace(rawID))
	if candidate == "" {
		candidate = slugifyIdentifier(strings.TrimSpace(rawTitle))
	}
	if candidate == "" {
		candidate = slugifyIdentifier(promptPreview(promptText, 48))
	}
	if candidate == "" {
		candidate = fmt.Sprintf("prompt-%02d", index+1)
	}

	count := usedIDs[candidate]
	usedIDs[candidate] = count + 1
	if count == 0 {
		return candidate
	}
	return fmt.Sprintf("%s-%d", candidate, count+1)
}

func normalizeFixturePaths(fixtures []string) []string {
	if len(fixtures) == 0 {
		return nil
	}

	normalized := make([]string, 0, len(fixtures))
	seen := map[string]struct{}{}
	for _, fixture := range fixtures {
		trimmed := strings.TrimSpace(strings.ReplaceAll(fixture, "\\", "/"))
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "./") {
			trimmed = strings.TrimPrefix(trimmed, "./")
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		normalized = append(normalized, trimmed)
	}
	sort.Strings(normalized)
	return normalized
}

func normalizePlaceholders(placeholders map[string]string) map[string]string {
	if len(placeholders) == 0 {
		return nil
	}

	normalized := make(map[string]string, len(placeholders))
	for key, value := range placeholders {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			continue
		}
		normalized[strings.ToUpper(trimmedKey)] = strings.TrimSpace(value)
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func promptPreview(prompt string, limit int) string {
	preview := strings.Join(strings.Fields(strings.TrimSpace(prompt)), " ")
	if limit > 3 && len(preview) > limit {
		return preview[:limit-3] + "..."
	}
	return preview
}

func promptDisplayLabel(prompt PromptDefinition, index, previewLimit int) string {
	preview := promptPreview(prompt.Prompt, previewLimit)
	return fmt.Sprintf("%d. [%s] %s — %s", index+1, prompt.Track, prompt.Title, preview)
}

func applyPromptPlaceholders(text string, values map[string]string) string {
	rendered := text
	if len(values) == 0 {
		return rendered
	}

	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		rendered = strings.ReplaceAll(rendered, "{{"+key+"}}", values[key])
	}
	return rendered
}

func renderPrompt(def PromptDefinition, modelStr, folderPath string) string {
	folderName := filepath.Base(folderPath)
	if folderName == "" || folderName == "." || folderName == string(filepath.Separator) {
		folderName = folderPath
	}

	placeholderValues := normalizePlaceholders(cloneStringMap(def.Placeholders))
	if placeholderValues == nil {
		placeholderValues = map[string]string{}
	}
	placeholderValues["MODEL_NAME"] = normalizeModelID(strings.TrimSpace(modelStr))
	placeholderValues["ASSIGNED_FOLDER"] = folderName

	body := applyPromptPlaceholders(def.Prompt, placeholderValues)
	instructions := []string{
		fmt.Sprintf("Task ID: %s", def.ID),
		fmt.Sprintf("Task Title: %s", def.Title),
		fmt.Sprintf("Track: %s", def.Track),
		fmt.Sprintf("Assigned folder: %s", folderName),
		"Write only inside the assigned folder. Do not create files or folders outside it.",
		"Do not add authentication, sign-in flows, or external accounts.",
		"Use the bundled local fixtures instead of live services or network dependencies whenever the task references fixture data.",
		"Verify the project locally before finishing and fix any issues you find.",
	}

	switch def.Track {
	case PromptTrackPython:
		instructions = append(instructions,
			"Use uv for Python environments, dependencies, and execution. Do not use pipenv, poetry, or bare pip workflows.",
			"Create a root .run script that executes the primary smoke path and exits non-zero on failure.",
		)
	case PromptTrackMobile:
		instructions = append(instructions,
			"This is a mobile track task. Keep it self-contained and fixture-driven.",
			"A root .run script is not required for mobile tasks, but include concise local start steps in README.md.",
			"Use Bun instead of npm, yarn, or pnpm for any JavaScript package management you need.",
		)
	default:
		instructions = append(instructions,
			"Use Bun for JavaScript and TypeScript tooling. Do not use Node, npm, pnpm, yarn, or Vite.",
			"For Tailwind on Bun, use Bun HTML imports with Bun.serve(), index.html, and CSS @import \"tailwindcss\". Never use Vite plugins.",
			"Do not create package-lock.json, pnpm-lock.yaml, yarn.lock, vite.config.*, or absolute browser imports like /src/main.tsx.",
			"Create a root .run script. Server tasks must bind 127.0.0.1:$PORT and keep the process alive. CLI tasks must run a smoke path and exit non-zero on failure.",
		)
	}

	checklist := []string{
		"Create a working root entry contract for the task type.",
		"Ensure all referenced local assets and scripts exist.",
		"Remove unresolved placeholders from generated files.",
		"Run the project locally and confirm the main path works.",
	}

	var builder strings.Builder
	builder.WriteString("You are running inside an isolated High-Evals workspace.\n\n")
	builder.WriteString("Global rules:\n")
	for _, line := range instructions {
		builder.WriteString("- ")
		builder.WriteString(line)
		builder.WriteString("\n")
	}

	if len(def.Fixtures) > 0 {
		builder.WriteString("\nBundled fixtures:\n")
		for _, fixture := range def.Fixtures {
			builder.WriteString("- ")
			builder.WriteString(fixture)
			builder.WriteString("\n")
		}
	}

	builder.WriteString("\nAcceptance checklist:\n")
	for _, item := range checklist {
		builder.WriteString("- ")
		builder.WriteString(item)
		builder.WriteString("\n")
	}

	builder.WriteString("\nTask:\n")
	builder.WriteString(body)
	builder.WriteString("\n")

	return builder.String()
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]string, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func cloneBoolMap(input map[string]bool) map[string]bool {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]bool, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func cloneStringSlice(input []string) []string {
	if len(input) == 0 {
		return nil
	}
	out := make([]string, len(input))
	copy(out, input)
	return out
}

func boolPointer(value bool) *bool {
	return &value
}

func slugifyIdentifier(value string) string {
	trimmed := strings.TrimSpace(strings.ToLower(value))
	if trimmed == "" {
		return ""
	}

	var builder strings.Builder
	lastDash := false
	for _, r := range trimmed {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			builder.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				builder.WriteByte('-')
				lastDash = true
			}
		}
	}

	return strings.Trim(builder.String(), "-")
}
