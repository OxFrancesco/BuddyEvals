package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type PreviewMode string

const (
	PreviewModeStatic        PreviewMode = "static"
	PreviewModeProjectServer PreviewMode = "project_server"
	PreviewModeNone          PreviewMode = "none"
)

type RunMode string

const (
	RunModeDotRun RunMode = ".run"
	RunModeUV     RunMode = "uv"
	RunModeLegacy RunMode = "legacy"
	RunModeNone   RunMode = "none"
)

type ValidationReport struct {
	Track       PromptTrack     `json:"track"`
	PreviewMode PreviewMode     `json:"preview_mode"`
	RunMode     RunMode         `json:"run_mode"`
	Checks      map[string]bool `json:"checks"`
	Violations  []string        `json:"violations"`
}

func (report ValidationReport) ValidationSuccess() bool {
	return len(report.Violations) == 0
}

func validateEvalFolder(folderPath string, track PromptTrack) ValidationReport {
	normalizedTrack := normalizePromptTrack(track, "")
	if normalizedTrack == "" {
		normalizedTrack = PromptTrackWeb
	}

	report := ValidationReport{
		Track:       normalizedTrack,
		Checks:      map[string]bool{},
		Violations:  []string{},
		PreviewMode: detectPreviewMode(folderPath),
		RunMode:     detectRunMode(folderPath, normalizedTrack),
	}

	hasRun := fileExists(filepath.Join(folderPath, ".run"))
	hasRootEntryContract := hasRun || (normalizedTrack == PromptTrackMobile && fileExists(filepath.Join(folderPath, "README.md")))
	missingRefs := collectMissingLocalReferences(folderPath)
	placeholderLeaks := collectPlaceholderLeaks(folderPath)
	forbiddenTooling := collectForbiddenToolingViolations(folderPath, normalizedTrack)
	starterTemplates := collectStarterTemplateViolations(folderPath)
	nestedProjects := collectNestedProjectViolations(folderPath, hasRootEntryContract)

	report.Checks["has_run_contract"] = normalizedTrack == PromptTrackMobile || hasRun
	if normalizedTrack != PromptTrackMobile && !hasRun {
		report.Violations = append(report.Violations, "Missing root .run contract")
	}

	report.Checks["no_missing_local_refs"] = len(missingRefs) == 0
	if len(missingRefs) > 0 {
		report.Violations = append(report.Violations, fmt.Sprintf("Missing local asset references: %s", strings.Join(missingRefs, ", ")))
	}

	report.Checks["no_unresolved_placeholders"] = len(placeholderLeaks) == 0
	if len(placeholderLeaks) > 0 {
		report.Violations = append(report.Violations, fmt.Sprintf("Unresolved placeholders found in: %s", strings.Join(placeholderLeaks, ", ")))
	}

	report.Checks["no_forbidden_tooling"] = len(forbiddenTooling) == 0
	if len(forbiddenTooling) > 0 {
		report.Violations = append(report.Violations, forbiddenTooling...)
	}

	report.Checks["no_starter_templates"] = len(starterTemplates) == 0
	if len(starterTemplates) > 0 {
		report.Violations = append(report.Violations, starterTemplates...)
	}

	report.Checks["nested_project_contract"] = len(nestedProjects) == 0
	if len(nestedProjects) > 0 {
		report.Violations = append(report.Violations, nestedProjects...)
	}

	report.Checks["preview_available"] = report.PreviewMode != PreviewModeNone || normalizedTrack == PromptTrackPython || normalizedTrack == PromptTrackCLI || normalizedTrack == PromptTrackMobile

	report.Violations = uniqueSortedStrings(report.Violations)
	return report
}

func detectRunMode(folderPath string, track PromptTrack) RunMode {
	if fileExists(filepath.Join(folderPath, ".run")) {
		return RunModeDotRun
	}
	if track == PromptTrackPython && (fileExists(filepath.Join(folderPath, "pyproject.toml")) || folderContainsExtension(folderPath, ".py")) {
		return RunModeUV
	}
	if fileExists(filepath.Join(folderPath, "index.html")) || fileExists(filepath.Join(folderPath, "package.json")) || folderContainsExtension(folderPath, ".ts") || folderContainsExtension(folderPath, ".tsx") {
		return RunModeLegacy
	}
	return RunModeNone
}

func detectPreviewMode(folderPath string) PreviewMode {
	if !fileExists(filepath.Join(folderPath, "index.html")) {
		return PreviewModeNone
	}
	if hasProjectServerSignals(folderPath) {
		return PreviewModeProjectServer
	}
	return PreviewModeStatic
}

func hasProjectServerSignals(folderPath string) bool {
	if fileContains(filepath.Join(folderPath, "index.ts"), "Bun.serve(") || fileContains(filepath.Join(folderPath, "server.ts"), "Bun.serve(") {
		return true
	}

	indexHTMLPath := filepath.Join(folderPath, "index.html")
	if !fileExists(indexHTMLPath) {
		return false
	}

	data, err := os.ReadFile(indexHTMLPath)
	if err != nil {
		return false
	}
	content := string(data)

	localTSModuleRef := regexp.MustCompile(`(?i)<script[^>]+src=["'][^"']+\.(ts|tsx|jsx)["']`)
	if localTSModuleRef.MatchString(content) {
		return true
	}

	apiFetchRef := regexp.MustCompile(`fetch\(["']/api/`)
	if apiFetchRef.MatchString(content) {
		return true
	}

	return folderContainsContent(folderPath, []string{".js", ".jsx", ".ts", ".tsx"}, []string{`fetch("/api/`, `fetch('/api/`, "Bun.serve("})
}

func collectMissingLocalReferences(folderPath string) []string {
	var missing []string
	seen := map[string]struct{}{}

	_ = filepath.WalkDir(folderPath, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			if shouldSkipValidationDir(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		refs := collectFileReferences(path)
		if len(refs) == 0 {
			return nil
		}

		baseDir := filepath.Dir(path)
		for _, ref := range refs {
			target := resolveReferenceTarget(folderPath, baseDir, ref)
			if target == "" || fileExists(target) {
				continue
			}
			label := fmt.Sprintf("%s -> %s", relPathOrBase(folderPath, path), ref)
			if _, exists := seen[label]; exists {
				continue
			}
			seen[label] = struct{}{}
			missing = append(missing, label)
		}
		return nil
	})

	sort.Strings(missing)
	return missing
}

func collectFileReferences(path string) []string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".html":
		return collectHTMLReferences(path)
	case ".css":
		return collectCSSReferences(path)
	case ".js", ".jsx", ".ts", ".tsx", ".mjs", ".cjs":
		return collectScriptReferences(path)
	default:
		return nil
	}
}

func collectHTMLReferences(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	content := string(data)

	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)<script[^>]+src=["']([^"']+)["']`),
		regexp.MustCompile(`(?i)<link[^>]+href=["']([^"']+)["']`),
		regexp.MustCompile(`(?i)<img[^>]+src=["']([^"']+)["']`),
		regexp.MustCompile(`(?i)<source[^>]+src=["']([^"']+)["']`),
	}

	refs := make([]string, 0)
	for _, pattern := range patterns {
		matches := pattern.FindAllStringSubmatch(content, -1)
		for _, match := range matches {
			if len(match) < 2 {
				continue
			}
			ref := sanitizeReference(match[1])
			if isLocalAssetReference(ref, true) {
				refs = append(refs, ref)
			}
		}
	}
	return refs
}

func collectCSSReferences(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	content := string(data)

	urlPattern := regexp.MustCompile(`url\(([^)]+)\)`)
	matches := urlPattern.FindAllStringSubmatch(content, -1)
	refs := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		ref := sanitizeReference(match[1])
		if isLocalAssetReference(ref, true) {
			refs = append(refs, ref)
		}
	}
	return refs
}

func collectScriptReferences(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	content := string(data)

	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?m)import\s+(?:[^"'\n]+?\s+from\s+)?["']([^"']+)["']`),
		regexp.MustCompile(`(?m)import\(["']([^"']+)["']\)`),
	}

	refs := make([]string, 0)
	for _, pattern := range patterns {
		matches := pattern.FindAllStringSubmatch(content, -1)
		for _, match := range matches {
			if len(match) < 2 {
				continue
			}
			ref := sanitizeReference(match[1])
			if isLocalAssetReference(ref, false) {
				refs = append(refs, ref)
			}
		}
	}
	return refs
}

func sanitizeReference(value string) string {
	ref := strings.TrimSpace(value)
	ref = strings.Trim(ref, `"'`)
	ref = strings.SplitN(ref, "#", 2)[0]
	ref = strings.SplitN(ref, "?", 2)[0]
	return ref
}

func isLocalAssetReference(ref string, allowBare bool) bool {
	if ref == "" {
		return false
	}
	lower := strings.ToLower(ref)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "data:") || strings.HasPrefix(lower, "mailto:") || strings.HasPrefix(lower, "tel:") || strings.HasPrefix(lower, "javascript:") || strings.HasPrefix(lower, "#") {
		return false
	}
	if strings.HasPrefix(ref, "/") || strings.HasPrefix(ref, "./") || strings.HasPrefix(ref, "../") {
		return true
	}
	return allowBare
}

func resolveReferenceTarget(rootDir, baseDir, ref string) string {
	if ref == "" {
		return ""
	}
	if strings.HasPrefix(ref, "/") {
		return filepath.Join(rootDir, filepath.FromSlash(strings.TrimPrefix(ref, "/")))
	}
	return filepath.Join(baseDir, filepath.FromSlash(ref))
}

func collectPlaceholderLeaks(folderPath string) []string {
	placeholderPattern := regexp.MustCompile(`(<[A-Z][A-Z0-9_]+>)|(\{\{[A-Z][A-Z0-9_]+\}\})`)
	var leaks []string

	_ = filepath.WalkDir(folderPath, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			if shouldSkipValidationDir(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !isTextValidationFile(path) {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil || len(data) > 512*1024 {
			return nil
		}
		if placeholderPattern.Match(data) {
			leaks = append(leaks, relPathOrBase(folderPath, path))
		}
		return nil
	})

	sort.Strings(leaks)
	return leaks
}

func collectForbiddenToolingViolations(folderPath string, track PromptTrack) []string {
	var violations []string
	forbiddenFiles := map[string]string{
		"package-lock.json": "Forbidden toolchain file detected: package-lock.json",
		"pnpm-lock.yaml":    "Forbidden toolchain file detected: pnpm-lock.yaml",
		"yarn.lock":         "Forbidden toolchain file detected: yarn.lock",
	}

	for name, message := range forbiddenFiles {
		if folderContainsFile(folderPath, name) {
			violations = append(violations, message)
		}
	}

	if folderContainsFile(folderPath, "vite.config.ts") || folderContainsFile(folderPath, "vite.config.js") || folderContainsFile(folderPath, "vite.config.mjs") || folderContainsFile(folderPath, "vite.config.cjs") {
		violations = append(violations, "Forbidden Vite configuration detected")
	}

	if track == PromptTrackWeb || track == PromptTrackCLI || track == PromptTrackIntegration {
		if folderContainsContent(folderPath, []string{".js", ".jsx", ".ts", ".tsx", ".json", ".html", ".css"}, []string{"@vitejs/plugin-react", "@tailwindcss/vite"}) {
			violations = append(violations, "Forbidden Vite plugin references detected")
		}
		if folderContainsContent(folderPath, []string{".html"}, []string{`/src/`}) {
			violations = append(violations, "Absolute /src browser imports are forbidden on Bun tracks")
		}
	}

	return uniqueSortedStrings(violations)
}

func collectStarterTemplateViolations(folderPath string) []string {
	templateStrings := []string{
		"Open up App.tsx to start working on your app!",
		"Open up app/index.tsx to start working on your app!",
	}

	var violations []string
	_ = filepath.WalkDir(folderPath, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			if shouldSkipValidationDir(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !isTextValidationFile(path) {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil || len(data) > 512*1024 {
			return nil
		}
		content := string(data)
		for _, needle := range templateStrings {
			if strings.Contains(content, needle) {
				violations = append(violations, fmt.Sprintf("Starter template content detected in %s", relPathOrBase(folderPath, path)))
				break
			}
		}
		return nil
	})

	return uniqueSortedStrings(violations)
}

func collectNestedProjectViolations(folderPath string, hasRootEntryContract bool) []string {
	if hasRootEntryContract {
		return nil
	}

	projectFiles := map[string]struct{}{
		"package.json":   {},
		"pyproject.toml": {},
		"app.json":       {},
		"Cargo.toml":     {},
		"go.mod":         {},
	}

	var nested []string
	_ = filepath.WalkDir(folderPath, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			if path != folderPath && shouldSkipValidationDir(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if _, ok := projectFiles[entry.Name()]; !ok {
			return nil
		}
		if filepath.Dir(path) == folderPath {
			return nil
		}
		nested = append(nested, relPathOrBase(folderPath, path))
		return nil
	})

	if len(nested) == 0 {
		return nil
	}

	return []string{fmt.Sprintf("Nested project detected without a root entry contract: %s", strings.Join(uniqueSortedStrings(nested), ", "))}
}

func folderContainsFile(rootDir, baseName string) bool {
	found := false
	_ = filepath.WalkDir(rootDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			if path != rootDir && shouldSkipValidationDir(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Name() == baseName {
			found = true
			return fs.SkipAll
		}
		return nil
	})
	return found
}

func folderContainsExtension(rootDir, ext string) bool {
	found := false
	_ = filepath.WalkDir(rootDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			if path != rootDir && shouldSkipValidationDir(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.EqualFold(filepath.Ext(entry.Name()), ext) {
			found = true
			return fs.SkipAll
		}
		return nil
	})
	return found
}

func folderContainsContent(rootDir string, extensions []string, needles []string) bool {
	allowed := map[string]struct{}{}
	for _, ext := range extensions {
		allowed[strings.ToLower(ext)] = struct{}{}
	}

	found := false
	_ = filepath.WalkDir(rootDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			if path != rootDir && shouldSkipValidationDir(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if _, ok := allowed[strings.ToLower(filepath.Ext(entry.Name()))]; !ok {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil || len(data) > 512*1024 {
			return nil
		}
		content := string(data)
		for _, needle := range needles {
			if strings.Contains(content, needle) {
				found = true
				return fs.SkipAll
			}
		}
		return nil
	})

	return found
}

func fileContains(path, needle string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), needle)
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func shouldSkipValidationDir(name string) bool {
	switch name {
	case ".git", "node_modules", ".venv", "__pycache__", ".ruff_cache", "dist", "build":
		return true
	default:
		return false
	}
}

func isTextValidationFile(path string) bool {
	base := filepath.Base(path)
	if base == ".run" || base == "README" || strings.HasPrefix(base, "README.") {
		return true
	}

	switch strings.ToLower(filepath.Ext(base)) {
	case ".txt", ".md", ".json", ".html", ".css", ".js", ".jsx", ".ts", ".tsx", ".py", ".sh", ".toml", ".yaml", ".yml":
		return true
	default:
		return false
	}
}

func relPathOrBase(rootDir, path string) string {
	rel, err := filepath.Rel(rootDir, path)
	if err != nil {
		return filepath.Base(path)
	}
	return filepath.ToSlash(rel)
}

func uniqueSortedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	set := map[string]struct{}{}
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		set[value] = struct{}{}
	}
	normalized := make([]string, 0, len(set))
	for value := range set {
		normalized = append(normalized, value)
	}
	sort.Strings(normalized)
	return normalized
}
