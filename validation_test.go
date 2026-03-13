package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateEvalFolderRequiresRunForWebTracks(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "index.html", `<html><body>Hello</body></html>`)

	report := validateEvalFolder(dir, PromptTrackWeb)

	if report.ValidationSuccess() {
		t.Fatalf("expected validation to fail without a .run contract")
	}
	if !hasViolationContaining(report.Violations, "Missing root .run contract") {
		t.Fatalf("expected missing .run violation, got %#v", report.Violations)
	}
	if report.RunMode != RunModeLegacy {
		t.Fatalf("expected legacy run mode, got %q", report.RunMode)
	}
	if report.PreviewMode != PreviewModeStatic {
		t.Fatalf("expected static preview mode, got %q", report.PreviewMode)
	}
}

func TestValidateEvalFolderFlagsMissingAssetReferences(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, ".run", "#!/bin/sh\nexit 0\n")
	writeTestFile(t, dir, "index.html", `<html><body><script src="./missing.js"></script></body></html>`)

	report := validateEvalFolder(dir, PromptTrackWeb)

	if !hasViolationContaining(report.Violations, "Missing local asset references") {
		t.Fatalf("expected missing asset violation, got %#v", report.Violations)
	}
}

func TestValidateEvalFolderFlagsForbiddenToolingAndVitePlugins(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, ".run", "#!/bin/sh\nexit 0\n")
	writeTestFile(t, dir, "package-lock.json", "{}")
	writeTestFile(t, dir, "vite.config.ts", "export default {};")
	writeTestFile(t, dir, "package.json", `{"devDependencies":{"@tailwindcss/vite":"^4.0.0"}}`)

	report := validateEvalFolder(dir, PromptTrackWeb)

	if !hasViolationContaining(report.Violations, "Forbidden toolchain file detected: package-lock.json") {
		t.Fatalf("expected package-lock violation, got %#v", report.Violations)
	}
	if !hasViolationContaining(report.Violations, "Forbidden Vite configuration detected") {
		t.Fatalf("expected vite config violation, got %#v", report.Violations)
	}
	if !hasViolationContaining(report.Violations, "Forbidden Vite plugin references detected") {
		t.Fatalf("expected vite plugin violation, got %#v", report.Violations)
	}
}

func TestValidateEvalFolderClassifiesProjectServerPreview(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, ".run", "#!/bin/sh\nexit 0\n")
	writeTestFile(t, dir, "index.html", `<html><body><script type="module" src="./frontend.tsx"></script></body></html>`)
	writeTestFile(t, dir, "frontend.tsx", `console.log("hello")`)

	report := validateEvalFolder(dir, PromptTrackWeb)

	if report.PreviewMode != PreviewModeProjectServer {
		t.Fatalf("expected project_server preview mode, got %q", report.PreviewMode)
	}
	if report.RunMode != RunModeDotRun {
		t.Fatalf("expected .run run mode, got %q", report.RunMode)
	}
	if !report.ValidationSuccess() {
		t.Fatalf("expected project server fixture to validate successfully, got %#v", report.Violations)
	}
}

func TestValidateEvalFolderFlagsStarterTemplatesWithoutRunRequirementForMobile(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "App.tsx", `export default function App() { return "Open up App.tsx to start working on your app!"; }`)

	report := validateEvalFolder(dir, PromptTrackMobile)

	if hasViolationContaining(report.Violations, "Missing root .run contract") {
		t.Fatalf("did not expect mobile track to require .run, got %#v", report.Violations)
	}
	if !hasViolationContaining(report.Violations, "Starter template content detected") {
		t.Fatalf("expected starter template violation, got %#v", report.Violations)
	}
}

func writeTestFile(t *testing.T, rootDir, relativePath, contents string) {
	t.Helper()

	fullPath := filepath.Join(rootDir, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		t.Fatalf("creating parent dirs for %s: %v", relativePath, err)
	}
	if err := os.WriteFile(fullPath, []byte(contents), 0644); err != nil {
		t.Fatalf("writing %s: %v", relativePath, err)
	}
}

func hasViolationContaining(violations []string, needle string) bool {
	for _, violation := range violations {
		if strings.Contains(violation, needle) {
			return true
		}
	}
	return false
}
