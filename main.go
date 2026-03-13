package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/huh"
)

const (
	defaultInactivityTimeout = 180 * time.Second
	defaultTransientRetries  = 1
	basePort                 = 4096
	ocCleanupPortScanCount   = 256
	defaultModelID           = "opencode/kimi-k2.5-free"
	promptsFile              = "prompts.json"
	promptTemplatesFile      = "prompt-templates.json"
	savedModelsFile          = "saved-models.json"
)

var (
	inactivityTimeout = defaultInactivityTimeout
	transientRetries  = defaultTransientRetries
	promptNumberRE    = regexp.MustCompile(`(?:^|_)p(\d+)(?:_|$)`)
)

func newEscBackForm(groups ...*huh.Group) *huh.Form {
	keymap := newFormKeyMap()
	return huh.NewForm(groups...).WithKeyMap(keymap)
}

func newFormKeyMap() *huh.KeyMap {
	keymap := huh.NewDefaultKeyMap()
	keymap.Quit = key.NewBinding(
		key.WithKeys("esc", "ctrl+c"),
		key.WithHelp("esc", "back"),
	)
	keymap.MultiSelect.SelectAll = key.NewBinding(
		key.WithKeys("A", "ctrl+a"),
		key.WithHelp("A", "select all"),
	)
	keymap.MultiSelect.SelectNone = key.NewBinding(
		key.WithKeys("A", "ctrl+a"),
		key.WithHelp("A", "select none"),
		key.WithDisabled(),
	)
	return keymap
}

func runFormWithBack(form *huh.Form) (aborted bool, err error) {
	err = form.Run()
	if err == nil {
		return false, nil
	}
	if errors.Is(err, huh.ErrUserAborted) {
		return true, nil
	}
	return false, err
}

type EvalResult struct {
	Prompt       string
	PromptNumber int
	PromptID     string
	PromptTitle  string
	Track        PromptTrack
	AgentSuccess bool
	Validation   ValidationReport
	Folder       string
	Success      bool
	Error        string
	Duration     time.Duration
}

type PromptTemplate struct {
	Name    string                `json:"name"`
	Prompts []PromptTemplateEntry `json:"prompts"`
}

type sdkRunEvalRequest struct {
	Title                    string `json:"title"`
	Prompt                   string `json:"prompt"`
	ProviderID               string `json:"providerID"`
	ModelID                  string `json:"modelID"`
	Hostname                 string `json:"hostname"`
	Port                     int    `json:"port"`
	InactivityTimeoutSeconds int    `json:"inactivityTimeoutSeconds"`
}

type sdkRunEvalResponse struct {
	Success     bool   `json:"success"`
	SessionID   string `json:"sessionID"`
	Error       string `json:"error"`
	CompletedBy string `json:"completedBy"`
	DurationMs  int64  `json:"durationMs"`
}

type EvalResultFile struct {
	Prompt            string          `json:"prompt"`
	PromptID          string          `json:"prompt_id,omitempty"`
	PromptTitle       string          `json:"prompt_title,omitempty"`
	PromptNumber      int             `json:"prompt_number,omitempty"`
	Model             string          `json:"model"`
	Track             PromptTrack     `json:"track,omitempty"`
	Success           bool            `json:"success"`
	AgentSuccess      bool            `json:"agent_success"`
	ValidationSuccess bool            `json:"validation_success"`
	Error             string          `json:"error,omitempty"`
	DurationSeconds   int             `json:"duration_seconds"`
	CompletedAt       string          `json:"completed_at"`
	CostUSD           float64         `json:"cost_usd,omitempty"`
	PreviewMode       PreviewMode     `json:"preview_mode,omitempty"`
	RunMode           RunMode         `json:"run_mode,omitempty"`
	Violations        []string        `json:"violations,omitempty"`
	Checks            map[string]bool `json:"checks,omitempty"`
}

type EvalFolder struct {
	Path         string
	Prompt       string
	PromptID     string
	PromptTitle  string
	PromptNumber int
	Track        PromptTrack
	Result       *EvalResultFile
}

type Provider struct {
	ID     string                     `json:"id"`
	Name   string                     `json:"name"`
	Models map[string]json.RawMessage `json:"models"`
}

type ProvidersData struct {
	Providers []Provider        `json:"providers"`
	Default   map[string]string `json:"default"`
}

type listeningProcess struct {
	Command string
	PID     int
	Port    int
}

var runBunSDKCommand = defaultRunBunSDKCommand

func main() {
	if len(os.Args) < 2 {
		interactiveMenu()
		return
	}

	command := os.Args[1]

	switch command {
	case "run":
		runCommand()
	case "resume":
		resumeCommand()
	case "audit":
		auditCommand(os.Args[2:])
	case "templates":
		templatesCommand(os.Args[2:])
	case "models":
		modelsCommand(os.Args[2:])
	case "oc":
		ocCommand(os.Args[2:])
	case "list":
		listCommand()
	case "add":
		addCommand()
	case "edit":
		editCommand()
	case "remove":
		removeCommand()
	case "help", "-h", "--help":
		showHelp()
	default:
		fmt.Printf("Unknown command: %s\n\n", command)
		showHelp()
		os.Exit(1)
	}
}

func interactiveMenu() {
	for {
		var action string

		promptCount := 0
		if prompts, err := loadPrompts(); err == nil {
			promptCount = len(prompts)
		}

		evalCount := 0
		if folders, err := scanEvalFolders(); err == nil {
			evalCount = len(folders)
		}

		savedCount := 0
		if saved, err := loadSavedModels(); err == nil {
			savedCount = len(saved)
		}

		templateCount := 0
		if templates, err := loadPromptTemplates(); err == nil {
			templateCount = len(templates)
		}

		form := newEscBackForm(
			huh.NewGroup(
				huh.NewSelect[string]().
					Title("High-Evals").
					Description(fmt.Sprintf("%d prompt(s) · %d eval(s) · %d saved model(s) · %d template(s)", promptCount, evalCount, savedCount, templateCount)).
					Options(
						huh.NewOption("Run evals        select prompts and model, then run", "run"),
						huh.NewOption("Resume evals     re-run previous evals from evals/", "resume"),
						huh.NewOption("Audit evals      validate folders and refresh result metadata", "audit"),
						huh.NewOption("OC cleanup       stop stale opencode sessions", "oc-cleanup"),
						huh.NewOption("Manage models    browse, search and save models", "models"),
						huh.NewOption("Templates        save named prompt subsets", "templates"),
						huh.NewOption("List prompts     show all prompts in prompts.json", "list"),
						huh.NewOption("Add prompt       create a new prompt", "add"),
						huh.NewOption("Edit prompt      modify an existing prompt", "edit"),
						huh.NewOption("Remove prompt    delete a prompt", "remove"),
						huh.NewOption("Exit", "exit"),
					).
					Value(&action),
			),
		)

		aborted, err := runFormWithBack(form)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return
		}
		if aborted {
			return
		}

		if action == "exit" {
			return
		}

		fmt.Println()

		switch action {
		case "run":
			runCommand()
		case "resume":
			resumeCommand()
		case "audit":
			auditCommand(nil)
		case "oc-cleanup":
			ocCleanupCommand()
		case "models":
			interactiveModelsCommand()
		case "templates":
			interactiveTemplatesCommand()
		case "list":
			listCommand()
		case "add":
			addCommand()
		case "edit":
			editCommand()
		case "remove":
			removeCommand()
		}

		fmt.Println()
	}
}

func showHelp() {
	fmt.Println(`High-Evals - Run coding agent evaluations

Usage:
  high-evals <command> [options]

Commands:
  run      Interactively select prompts and model, then run evals
  resume   Resume or re-run previous evals from the evals/ folder
  audit    Validate eval folders and optionally refresh result metadata
  templates Manage named prompt subsets stored in prompt-templates.json
  oc       OpenCode utilities (cleanup stale local sessions)
  models   Interactively browse and save models for reuse
  list     List all prompts in prompts.json
  add      Add a new prompt to prompts.json
  edit     Edit an existing prompt
  remove   Remove a prompt from prompts.json
  help     Show this help message

Examples:
  high-evals run
  high-evals run -t quick-web -m openrouter/z-ai/glm-5
  high-evals resume
  high-evals audit --write
  high-evals templates
  high-evals templates add
  high-evals oc cleanup
  high-evals models
  high-evals models list
  high-evals models check openrouter/glm-5
  high-evals models saved
  high-evals add
  high-evals list

Interactive shortcuts:
  Esc      Go back/cancel current screen
  A        Select/deselect all in multi-select lists
  Ctrl+C   Quit`)
}

func ocCommand(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: high-evals oc cleanup")
		os.Exit(1)
	}

	switch args[0] {
	case "cleanup":
		ocCleanupCommand()
	default:
		fmt.Fprintf(os.Stderr, "Unknown oc subcommand: %s\n", args[0])
		fmt.Fprintln(os.Stderr, "Usage: high-evals oc cleanup")
		os.Exit(1)
	}
}

func ocCleanupCommand() {
	minPort := basePort
	maxPort := basePort + ocCleanupPortScanCount - 1

	procs, err := listListeningOpencodeProcesses(minPort, maxPort)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error scanning local opencode sessions: %v\n", err)
		os.Exit(1)
	}

	if len(procs) == 0 {
		fmt.Printf("No stale opencode sessions found on ports %d-%d.\n", minPort, maxPort)
		return
	}

	portsByPID := make(map[int][]int)
	commandByPID := make(map[int]string)
	for _, p := range procs {
		portsByPID[p.PID] = append(portsByPID[p.PID], p.Port)
		commandByPID[p.PID] = p.Command
	}

	pids := make([]int, 0, len(portsByPID))
	for pid := range portsByPID {
		pids = append(pids, pid)
	}
	sort.Ints(pids)

	fmt.Printf("Found %d opencode session process(es) to clean up.\n", len(pids))

	cleaned := 0
	failed := 0
	for _, pid := range pids {
		ports := portsByPID[pid]
		sort.Ints(ports)
		if err := terminateProcess(pid, ports); err != nil {
			fmt.Printf("✗ PID %d (%s) on ports %s: %v\n", pid, commandByPID[pid], formatPorts(ports), err)
			failed++
			continue
		}
		fmt.Printf("✓ Stopped PID %d (%s) on ports %s\n", pid, commandByPID[pid], formatPorts(ports))
		cleaned++
	}

	fmt.Printf("Cleanup complete: %d stopped, %d failed.\n", cleaned, failed)
	if failed > 0 {
		os.Exit(1)
	}
}

func listListeningOpencodeProcesses(minPort, maxPort int) ([]listeningProcess, error) {
	output, err := exec.Command("lsof", "-nP", "-iTCP", "-sTCP:LISTEN").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("running lsof: %w (%s)", err, strings.TrimSpace(string(output)))
	}

	lineRE := regexp.MustCompile(`^(\S+)\s+(\d+)\s+.*TCP .*:(\d+) \(LISTEN\)$`)
	lines := strings.Split(string(output), "\n")
	procs := make([]listeningProcess, 0)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "COMMAND ") {
			continue
		}

		m := lineRE.FindStringSubmatch(line)
		if len(m) != 4 {
			continue
		}

		command := strings.ToLower(m[1])
		if !strings.Contains(command, "opencode") {
			continue
		}

		pid, err := strconv.Atoi(m[2])
		if err != nil || pid <= 0 {
			continue
		}

		port, err := strconv.Atoi(m[3])
		if err != nil {
			continue
		}
		if port < minPort || port > maxPort {
			continue
		}

		procs = append(procs, listeningProcess{
			Command: m[1],
			PID:     pid,
			Port:    port,
		})
	}

	return procs, nil
}

func terminateProcess(pid int, ports []int) error {
	_ = terminateSinglePID(pid)
	if waitForPortsClosed(ports, 2*time.Second) {
		return nil
	}

	parentPID, err := getParentPID(pid)
	if err == nil && parentPID > 1 {
		parentCmd, cmdErr := getProcessCommand(parentPID)
		if cmdErr == nil && strings.Contains(strings.ToLower(parentCmd), "high-evals") {
			_ = terminateSinglePID(parentPID)
			if waitForPortsClosed(ports, 2*time.Second) {
				return nil
			}
		}
	}

	return errors.New("session ports still listening after termination attempts")
}

func terminateSinglePID(pid int) error {
	pidStr := strconv.Itoa(pid)
	_ = exec.Command("kill", "-TERM", pidStr).Run()

	deadline := time.Now().Add(1200 * time.Millisecond)
	for time.Now().Before(deadline) {
		if !isProcessAlive(pid) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	_ = exec.Command("kill", "-KILL", pidStr).Run()
	time.Sleep(150 * time.Millisecond)
	return nil
}

func isProcessAlive(pid int) bool {
	return exec.Command("kill", "-0", strconv.Itoa(pid)).Run() == nil
}

func getParentPID(pid int) (int, error) {
	output, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "ppid=").CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("reading parent PID for %d: %w (%s)", pid, err, strings.TrimSpace(string(output)))
	}
	parentStr := strings.TrimSpace(string(output))
	if parentStr == "" {
		return 0, errors.New("empty parent PID")
	}
	parentPID, err := strconv.Atoi(parentStr)
	if err != nil {
		return 0, fmt.Errorf("invalid parent PID %q", parentStr)
	}
	return parentPID, nil
}

func getProcessCommand(pid int) (string, error) {
	output, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "command=").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("reading command for %d: %w (%s)", pid, err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}

func waitForPortsClosed(ports []int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !anyPortListening(ports) {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return !anyPortListening(ports)
}

func anyPortListening(ports []int) bool {
	for _, port := range ports {
		if isPortListening(port) {
			return true
		}
	}
	return false
}

func isPortListening(port int) bool {
	output, err := exec.Command("lsof", "-nP", fmt.Sprintf("-iTCP:%d", port), "-sTCP:LISTEN", "-t").CombinedOutput()
	if err != nil {
		return strings.TrimSpace(string(output)) != ""
	}
	return strings.TrimSpace(string(output)) != ""
}

func formatPorts(ports []int) string {
	if len(ports) == 0 {
		return "-"
	}
	parts := make([]string, len(ports))
	for i, p := range ports {
		parts[i] = strconv.Itoa(p)
	}
	return strings.Join(parts, ",")
}

func modelsCommand(args []string) {
	if len(args) == 0 {
		interactiveModelsCommand()
		return
	}

	switch args[0] {
	case "save":
		saveModelsCommand(args[1:])
	case "saved":
		listSavedModelsCommand()
	case "list":
		providersData, err := getProvidersData()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error fetching providers/models: %v\n", err)
			os.Exit(1)
		}
		savedSet, err := loadSavedModelSet()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not load saved models for pinning: %v\n", err)
			savedSet = map[string]struct{}{}
		}
		printProviders(providersData, savedSet)
	case "check":
		checkModelCommand(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "Unknown models subcommand: %s\n", args[0])
		fmt.Fprintln(os.Stderr, "Usage: high-evals models [save <provider/model>|saved|list|check <provider/model>]")
		os.Exit(1)
	}
}

func interactiveModelsCommand() {
	providersData, err := getProvidersData()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error fetching providers/models: %v\n", err)
		os.Exit(1)
	}

	allModels := flattenModelIDs(providersData)
	if len(allModels) == 0 {
		fmt.Fprintln(os.Stderr, "No models available.")
		os.Exit(1)
	}

	selected, err := promptModelsToSave(allModels)
	if err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if len(selected) == 0 {
		fmt.Println("No models selected.")
		return
	}

	existing, err := loadSavedModels()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading saved models: %v\n", err)
		os.Exit(1)
	}

	set := make(map[string]struct{}, len(existing))
	for _, model := range existing {
		set[model] = struct{}{}
	}

	added := 0
	for _, model := range selected {
		if _, exists := set[model]; exists {
			continue
		}
		existing = append(existing, model)
		set[model] = struct{}{}
		added++
	}

	if added == 0 {
		fmt.Printf("No new models added. Selected models already saved in %s.\n", savedModelsFile)
		return
	}

	sort.Strings(existing)
	if err := saveSavedModels(existing); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving models: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Saved %d model(s) to %s.\n", added, savedModelsFile)
}

func loadPromptTemplates() ([]PromptTemplate, error) {
	data, err := os.ReadFile(promptTemplatesFile)
	if err != nil {
		if os.IsNotExist(err) {
			return []PromptTemplate{}, nil
		}
		return nil, err
	}

	if len(data) == 0 {
		return []PromptTemplate{}, nil
	}

	var templates []PromptTemplate
	if err := json.Unmarshal(data, &templates); err != nil {
		return nil, err
	}

	return templates, nil
}

func savePromptTemplates(templates []PromptTemplate) error {
	sort.Slice(templates, func(i, j int) bool {
		return strings.ToLower(strings.TrimSpace(templates[i].Name)) < strings.ToLower(strings.TrimSpace(templates[j].Name))
	})

	data, err := json.MarshalIndent(templates, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(promptTemplatesFile, data, 0644)
}

func findPromptTemplateIndexByName(templates []PromptTemplate, name string) int {
	trimmedName := strings.TrimSpace(name)
	for i, template := range templates {
		if strings.EqualFold(strings.TrimSpace(template.Name), trimmedName) {
			return i
		}
	}
	return -1
}

func buildPromptTemplateEntries(selectedIndices []int, prompts PromptJSON) []PromptTemplateEntry {
	entries := make([]PromptTemplateEntry, 0, len(selectedIndices))
	for _, idx := range selectedIndices {
		if idx < 0 || idx >= len(prompts) {
			continue
		}
		entries = append(entries, PromptTemplateEntry{
			PromptID:     prompts[idx].ID,
			PromptNumber: idx + 1,
			PromptText:   prompts[idx].Prompt,
		})
	}
	return entries
}

func buildPromptIndexByText(prompts PromptJSON) map[string]int {
	lookup := make(map[string]int, len(prompts))
	for i, prompt := range prompts {
		if _, exists := lookup[prompt.Prompt]; exists {
			continue
		}
		lookup[prompt.Prompt] = i
	}
	return lookup
}

func buildPromptIndexByID(prompts PromptJSON) map[string]int {
	lookup := make(map[string]int, len(prompts))
	for i, prompt := range prompts {
		if prompt.ID == "" {
			continue
		}
		if _, exists := lookup[prompt.ID]; exists {
			continue
		}
		lookup[prompt.ID] = i
	}
	return lookup
}

func resolvePromptTemplate(name string, templates []PromptTemplate, prompts PromptJSON) (PromptTemplate, []int, error) {
	templateIdx := findPromptTemplateIndexByName(templates, name)
	if templateIdx == -1 {
		return PromptTemplate{}, nil, fmt.Errorf("template %q not found", name)
	}

	template := templates[templateIdx]
	if len(template.Prompts) == 0 {
		return template, nil, fmt.Errorf("template %q has no prompts", template.Name)
	}

	promptIndexByText := buildPromptIndexByText(prompts)
	promptIndexByID := buildPromptIndexByID(prompts)
	selectedIndices := make([]int, 0, len(template.Prompts))
	seen := make(map[int]struct{}, len(template.Prompts))

	for _, entry := range template.Prompts {
		resolvedIdx := -1

		if entry.PromptID != "" {
			if idx, ok := promptIndexByID[entry.PromptID]; ok {
				resolvedIdx = idx
			}
		}

		if resolvedIdx == -1 && entry.PromptText != "" {
			if idx, ok := promptIndexByText[entry.PromptText]; ok {
				resolvedIdx = idx
			}
		}

		if resolvedIdx == -1 && entry.PromptNumber >= 1 && entry.PromptNumber <= len(prompts) {
			resolvedIdx = entry.PromptNumber - 1
		}

		if resolvedIdx == -1 {
			return template, nil, fmt.Errorf("template %q references a prompt that no longer exists", template.Name)
		}

		if _, exists := seen[resolvedIdx]; exists {
			continue
		}
		seen[resolvedIdx] = struct{}{}
		selectedIndices = append(selectedIndices, resolvedIdx)
	}

	if len(selectedIndices) == 0 {
		return template, nil, fmt.Errorf("template %q has no valid prompts", template.Name)
	}

	return template, selectedIndices, nil
}

func templatePromptNumbers(template PromptTemplate) []int {
	numbers := make([]int, 0, len(template.Prompts))
	for _, entry := range template.Prompts {
		if entry.PromptNumber > 0 {
			numbers = append(numbers, entry.PromptNumber)
		}
	}
	return numbers
}

func formatPromptNumberSummary(numbers []int) string {
	if len(numbers) == 0 {
		return "-"
	}

	parts := make([]string, len(numbers))
	for i, number := range numbers {
		parts[i] = fmt.Sprintf("p%d", number)
	}
	return strings.Join(parts, ", ")
}

func buildPromptOptions(prompts PromptJSON, previewLimit int) []huh.Option[int] {
	options := make([]huh.Option[int], len(prompts))
	for i, prompt := range prompts {
		options[i] = huh.NewOption(promptDisplayLabel(prompt, i, previewLimit), i)
	}
	return options
}

func templatesCommand(args []string) {
	if len(args) == 0 {
		interactiveTemplatesCommand()
		return
	}

	switch args[0] {
	case "list":
		listTemplatesCommand()
	case "add":
		addTemplateCommand()
	case "remove":
		removeTemplateCommand(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "Unknown templates subcommand: %s\n", args[0])
		fmt.Fprintln(os.Stderr, "Usage: high-evals templates [list|add|remove <name>]")
		os.Exit(1)
	}
}

func interactiveTemplatesCommand() {
	for {
		templates, err := loadPromptTemplates()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading templates: %v\n", err)
			os.Exit(1)
		}

		var action string
		form := newEscBackForm(
			huh.NewGroup(
				huh.NewSelect[string]().
					Title("Prompt templates").
					Description(fmt.Sprintf("%d template(s) stored in %s", len(templates), promptTemplatesFile)).
					Options(
						huh.NewOption("List templates   show saved prompt subsets", "list"),
						huh.NewOption("Add template     save a new named prompt subset", "add"),
						huh.NewOption("Remove template  delete an existing template", "remove"),
						huh.NewOption("Back", "back"),
					).
					Value(&action),
			),
		)

		aborted, err := runFormWithBack(form)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if aborted || action == "back" {
			return
		}

		fmt.Println()
		switch action {
		case "list":
			listTemplatesCommand()
		case "add":
			addTemplateCommand()
		case "remove":
			removeTemplateCommand(nil)
		}
		fmt.Println()
	}
}

func listTemplatesCommand() {
	templates, err := loadPromptTemplates()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading templates: %v\n", err)
		os.Exit(1)
	}

	if len(templates) == 0 {
		fmt.Printf("No prompt templates found. Use 'high-evals templates add' to create one.\n")
		return
	}

	fmt.Printf("Templates in %s:\n\n", promptTemplatesFile)
	for i, template := range templates {
		numbers := templatePromptNumbers(template)
		fmt.Printf("  %d. %s (%d prompt(s): %s)\n", i+1, template.Name, len(template.Prompts), formatPromptNumberSummary(numbers))
	}
}

func addTemplateCommand() {
	prompts, err := loadPrompts()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading prompts: %v\n", err)
		os.Exit(1)
	}

	if len(prompts) == 0 {
		fmt.Println("No prompts found. Add prompts before creating a template.")
		return
	}

	var selectedIndices []int
	selectForm := newEscBackForm(
		huh.NewGroup(
			huh.NewMultiSelect[int]().
				Title("Select prompts for the template").
				Description("Choose the subset you want to re-run quickly. The template keeps this prompt order.").
				Options(buildPromptOptions(prompts, 60)...).
				Value(&selectedIndices).
				Filterable(true),
		),
	)

	aborted, err := runFormWithBack(selectForm)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if aborted {
		return
	}

	if len(selectedIndices) == 0 {
		fmt.Println("No prompts selected.")
		return
	}

	var templateName string
	nameForm := newEscBackForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Template name").
				Description("Example: quick-web, core-8, or regressions").
				Placeholder("quick-web").
				Value(&templateName),
		),
	)

	aborted, err = runFormWithBack(nameForm)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if aborted {
		return
	}

	templateName = strings.TrimSpace(templateName)
	if templateName == "" {
		fmt.Println("Template name cannot be empty.")
		os.Exit(1)
	}

	templates, err := loadPromptTemplates()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading templates: %v\n", err)
		os.Exit(1)
	}

	template := PromptTemplate{
		Name:    templateName,
		Prompts: buildPromptTemplateEntries(selectedIndices, prompts),
	}

	existingIdx := findPromptTemplateIndexByName(templates, templateName)
	if existingIdx != -1 {
		var overwrite bool
		confirmForm := newEscBackForm(
			huh.NewGroup(
				huh.NewConfirm().
					Title(fmt.Sprintf("Overwrite template %q?", templates[existingIdx].Name)).
					Value(&overwrite),
			),
		)

		aborted, err = runFormWithBack(confirmForm)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if aborted || !overwrite {
			fmt.Println("Cancelled.")
			return
		}

		templates[existingIdx] = template
	} else {
		templates = append(templates, template)
	}

	if err := savePromptTemplates(templates); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving templates: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Saved template %q with %d prompt(s) to %s.\n", template.Name, len(template.Prompts), promptTemplatesFile)
}

func removeTemplateCommand(args []string) {
	templates, err := loadPromptTemplates()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading templates: %v\n", err)
		os.Exit(1)
	}

	if len(templates) == 0 {
		fmt.Println("No templates to remove.")
		return
	}

	var templateName string
	if len(args) > 0 {
		templateName = strings.TrimSpace(args[0])
	} else {
		var selectedName string
		options := make([]huh.Option[string], len(templates))
		for i, template := range templates {
			options[i] = huh.NewOption(
				fmt.Sprintf("%s (%d prompt(s): %s)", template.Name, len(template.Prompts), formatPromptNumberSummary(templatePromptNumbers(template))),
				template.Name,
			)
		}

		form := newEscBackForm(
			huh.NewGroup(
				huh.NewSelect[string]().
					Title("Select a template to remove").
					Options(options...).
					Value(&selectedName),
			),
		)

		aborted, err := runFormWithBack(form)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if aborted {
			return
		}

		templateName = selectedName
	}

	templateIdx := findPromptTemplateIndexByName(templates, templateName)
	if templateIdx == -1 {
		fmt.Fprintf(os.Stderr, "Template not found: %s\n", templateName)
		os.Exit(1)
	}

	var confirmRemove bool
	confirmForm := newEscBackForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(fmt.Sprintf("Remove template %q?", templates[templateIdx].Name)).
				Value(&confirmRemove),
		),
	)

	aborted, err := runFormWithBack(confirmForm)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if aborted {
		return
	}
	if !confirmRemove {
		fmt.Println("Cancelled.")
		return
	}

	removedName := templates[templateIdx].Name
	templates = append(templates[:templateIdx], templates[templateIdx+1:]...)
	if err := savePromptTemplates(templates); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving templates: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Removed template %q.\n", removedName)
}

func listCommand() {
	prompts, err := loadPrompts()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading prompts: %v\n", err)
		os.Exit(1)
	}

	if len(prompts) == 0 {
		fmt.Println("No prompts found. Use 'high-evals add' to add one.")
		return
	}

	fmt.Printf("Prompts in %s:\n\n", promptsFile)
	for i, p := range prompts {
		fmt.Printf("  %d. [%s] %s\n", i+1, p.Track, p.Title)
		fmt.Printf("     %s\n", promptPreview(p.Prompt, 96))
	}
	fmt.Printf("\nTotal: %d prompt(s)\n", len(prompts))
}

func addCommand() {
	var (
		newTitle         string
		newPrompt        string
		newTrack         string
		newDeterministic = true
	)

	form := newEscBackForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Prompt title").
				Description("Short human-friendly label for the task").
				Value(&newTitle),
			huh.NewSelect[string]().
				Title("Track").
				Options(
					huh.NewOption("Web", string(PromptTrackWeb)),
					huh.NewOption("Python", string(PromptTrackPython)),
					huh.NewOption("CLI", string(PromptTrackCLI)),
					huh.NewOption("Integration", string(PromptTrackIntegration)),
					huh.NewOption("Mobile", string(PromptTrackMobile)),
				).
				Value(&newTrack),
			huh.NewConfirm().
				Title("Deterministic / self-contained?").
				Description("Headline comparisons only use deterministic non-mobile, non-integration prompts.").
				Value(&newDeterministic),
			huh.NewText().
				Title("Enter the new prompt").
				Description("Write the task body only. Shared runner instructions are injected automatically.").
				Value(&newPrompt).
				CharLimit(4000),
		),
	)

	aborted, err := runFormWithBack(form)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if aborted {
		return
	}

	newPrompt = strings.TrimSpace(newPrompt)
	if newPrompt == "" {
		fmt.Println("Prompt cannot be empty.")
		os.Exit(1)
	}

	prompts, err := loadPrompts()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading prompts: %v\n", err)
		os.Exit(1)
	}

	prompts = append(prompts, PromptDefinition{
		Title:         strings.TrimSpace(newTitle),
		Prompt:        newPrompt,
		Track:         normalizePromptTrack(PromptTrack(newTrack), newPrompt),
		Deterministic: newDeterministic,
	})

	if err := savePrompts(prompts); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving prompts: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Added prompt #%d\n", len(prompts))
}

func editCommand() {
	prompts, err := loadPrompts()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading prompts: %v\n", err)
		os.Exit(1)
	}

	if len(prompts) == 0 {
		fmt.Println("No prompts to edit. Use 'high-evals add' to add one.")
		return
	}

	var selectedIdx int
	options := make([]huh.Option[int], len(prompts))
	for i, p := range prompts {
		options[i] = huh.NewOption(promptDisplayLabel(p, i, 60), i)
	}

	selectForm := newEscBackForm(
		huh.NewGroup(
			huh.NewSelect[int]().
				Title("Select a prompt to edit").
				Options(options...).
				Value(&selectedIdx),
		),
	)

	aborted, err := runFormWithBack(selectForm)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if aborted {
		return
	}

	editedPrompt := prompts[selectedIdx]
	editedTrack := string(editedPrompt.Track)

	editForm := newEscBackForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Edit title").
				Value(&editedPrompt.Title),
			huh.NewSelect[string]().
				Title("Edit track").
				Options(
					huh.NewOption("Web", string(PromptTrackWeb)),
					huh.NewOption("Python", string(PromptTrackPython)),
					huh.NewOption("CLI", string(PromptTrackCLI)),
					huh.NewOption("Integration", string(PromptTrackIntegration)),
					huh.NewOption("Mobile", string(PromptTrackMobile)),
				).
				Value(&editedTrack),
			huh.NewConfirm().
				Title("Deterministic / self-contained?").
				Value(&editedPrompt.Deterministic),
			huh.NewText().
				Title("Edit the prompt").
				Value(&editedPrompt.Prompt).
				CharLimit(4000),
		),
	)

	aborted, err = runFormWithBack(editForm)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if aborted {
		return
	}

	editedPrompt.Prompt = strings.TrimSpace(editedPrompt.Prompt)
	editedPrompt.Title = strings.TrimSpace(editedPrompt.Title)
	editedPrompt.Track = normalizePromptTrack(PromptTrack(editedTrack), editedPrompt.Prompt)
	if editedPrompt.Prompt == "" {
		fmt.Println("Prompt cannot be empty.")
		os.Exit(1)
	}

	prompts[selectedIdx] = editedPrompt

	if err := savePrompts(prompts); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving prompts: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Updated prompt #%d\n", selectedIdx+1)
}

func removeCommand() {
	prompts, err := loadPrompts()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading prompts: %v\n", err)
		os.Exit(1)
	}

	if len(prompts) == 0 {
		fmt.Println("No prompts to remove.")
		return
	}

	var selectedIdx int
	options := make([]huh.Option[int], len(prompts))
	for i, p := range prompts {
		options[i] = huh.NewOption(promptDisplayLabel(p, i, 60), i)
	}

	form := newEscBackForm(
		huh.NewGroup(
			huh.NewSelect[int]().
				Title("Select a prompt to remove").
				Options(options...).
				Value(&selectedIdx),
		),
	)

	aborted, err := runFormWithBack(form)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if aborted {
		return
	}

	var confirmRemove bool
	confirmForm := newEscBackForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(fmt.Sprintf("Remove prompt #%d?", selectedIdx+1)).
				Value(&confirmRemove),
		),
	)

	aborted, err = runFormWithBack(confirmForm)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if aborted {
		return
	}

	if !confirmRemove {
		fmt.Println("Cancelled.")
		return
	}

	prompts = append(prompts[:selectedIdx], prompts[selectedIdx+1:]...)

	if err := savePrompts(prompts); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving prompts: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Removed prompt #%d\n", selectedIdx+1)
}

func parsePromptIndices(value string, promptCount int) ([]int, error) {
	if strings.TrimSpace(value) == "" {
		return nil, errors.New("prompt indices cannot be empty")
	}

	indices := make([]int, 0)
	seen := make(map[int]struct{})

	for _, raw := range strings.Split(value, ",") {
		part := strings.TrimSpace(raw)
		idx, err := strconv.Atoi(part)
		if err != nil || idx < 1 || idx > promptCount {
			return nil, fmt.Errorf("invalid prompt index: %s (must be 1-%d)", part, promptCount)
		}

		zeroBased := idx - 1
		if _, exists := seen[zeroBased]; exists {
			continue
		}
		seen[zeroBased] = struct{}{}
		indices = append(indices, zeroBased)
	}

	return indices, nil
}

func normalizeRunMode(mode string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(mode))
	switch normalized {
	case "", "sequential":
		return "sequential", nil
	case "parallel":
		return "parallel", nil
	default:
		return "", fmt.Errorf("invalid mode %q (expected parallel or sequential)", mode)
	}
}

func promptForPromptIndices(prompts PromptJSON, title, description string) ([]int, bool, error) {
	var selectedIndices []int
	form := newEscBackForm(
		huh.NewGroup(
			huh.NewMultiSelect[int]().
				Title(title).
				Description(description).
				Options(buildPromptOptions(prompts, 60)...).
				Value(&selectedIndices).
				Filterable(true),
		),
	)

	aborted, err := runFormWithBack(form)
	if err != nil {
		return nil, false, err
	}
	if aborted {
		return nil, true, nil
	}

	return selectedIndices, false, nil
}

func selectRunPromptIndices(prompts PromptJSON, templates []PromptTemplate) ([]int, string, bool, error) {
	if len(templates) == 0 {
		selectedIndices, aborted, err := promptForPromptIndices(prompts, "Select prompts to run", "Choose one or more prompts from prompts.json")
		return selectedIndices, "", aborted, err
	}

	var promptSource string
	sourceForm := newEscBackForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Prompt source").
				Description("Run a manual prompt selection or a saved template").
				Options(
					huh.NewOption("Manual selection", "manual"),
					huh.NewOption("Saved template", "template"),
				).
				Value(&promptSource),
		),
	)

	aborted, err := runFormWithBack(sourceForm)
	if err != nil {
		return nil, "", false, err
	}
	if aborted {
		return nil, "", true, nil
	}

	if promptSource != "template" {
		selectedIndices, aborted, err := promptForPromptIndices(prompts, "Select prompts to run", "Choose one or more prompts from prompts.json")
		return selectedIndices, "", aborted, err
	}

	options := make([]huh.Option[string], len(templates))
	for i, template := range templates {
		options[i] = huh.NewOption(
			fmt.Sprintf("%s (%d prompt(s): %s)", template.Name, len(template.Prompts), formatPromptNumberSummary(templatePromptNumbers(template))),
			template.Name,
		)
	}

	var selectedTemplateName string
	templateForm := newEscBackForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select a template").
				Description("Templates are loaded from prompt-templates.json").
				Options(options...).
				Value(&selectedTemplateName),
		),
	)

	aborted, err = runFormWithBack(templateForm)
	if err != nil {
		return nil, "", false, err
	}
	if aborted {
		return nil, "", true, nil
	}

	template, selectedIndices, err := resolvePromptTemplate(selectedTemplateName, templates, prompts)
	if err != nil {
		return nil, "", false, err
	}

	return selectedIndices, template.Name, false, nil
}

func promptForRunMode() (string, bool, error) {
	var runMode string
	form := newEscBackForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Execution mode").
				Options(
					huh.NewOption("Parallel (run all at once)", "parallel"),
					huh.NewOption("Sequential (run one at a time)", "sequential"),
				).
				Value(&runMode),
		),
	)

	aborted, err := runFormWithBack(form)
	if err != nil {
		return "", false, err
	}
	if aborted {
		return "", true, nil
	}

	return runMode, false, nil
}

func runCommand() {
	prompts, err := loadPrompts()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading prompts: %v\n", err)
		os.Exit(1)
	}

	if len(prompts) == 0 {
		fmt.Println("No prompts found. Use 'high-evals add' to add prompts first.")
		os.Exit(1)
	}

	// Parse optional CLI flags for non-interactive mode
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	flagModel := fs.String("m", "", "Model to use (e.g. opencode/kimi-k2.5-free)")
	flagPrompts := fs.String("p", "", "Comma-separated 1-based prompt indices (e.g. 1,3,5)")
	flagTemplate := fs.String("t", "", "Prompt template name from prompt-templates.json")
	flagMode := fs.String("mode", "sequential", "Execution mode: parallel or sequential")
	flagInactivityTimeout := fs.Int("inactivity-timeout", int(defaultInactivityTimeout.Seconds()), "Inactivity timeout in seconds before failing a run")
	flagRetries := fs.Int("retries", defaultTransientRetries, "Retries for transient failures (timeout/stream errors)")
	if len(os.Args) > 2 {
		fs.Parse(os.Args[2:])
	}
	applyRuntimeOptions(*flagInactivityTimeout, *flagRetries)

	var selectedIndices []int
	var selectedTemplateName string
	var modelStr string
	var runMode string

	if strings.TrimSpace(*flagPrompts) != "" && strings.TrimSpace(*flagTemplate) != "" {
		fmt.Fprintln(os.Stderr, "Use either -p or -t, not both.")
		os.Exit(1)
	}

	runMode, err = normalizeRunMode(*flagMode)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if strings.TrimSpace(*flagPrompts) != "" {
		selectedIndices, err = parsePromptIndices(*flagPrompts, len(prompts))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		modelStr = strings.TrimSpace(*flagModel)
	} else if strings.TrimSpace(*flagTemplate) != "" {
		templates, err := loadPromptTemplates()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading templates: %v\n", err)
			os.Exit(1)
		}

		template, resolvedIndices, err := resolvePromptTemplate(*flagTemplate, templates, prompts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		selectedTemplateName = template.Name
		selectedIndices = resolvedIndices
		modelStr = strings.TrimSpace(*flagModel)
	} else {
		templates, err := loadPromptTemplates()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading templates: %v\n", err)
			os.Exit(1)
		}

		var aborted bool
		selectedIndices, selectedTemplateName, aborted, err = selectRunPromptIndices(prompts, templates)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if aborted {
			return
		}

		if len(selectedIndices) == 0 {
			fmt.Println("No prompts selected.")
			return
		}

		runMode, aborted, err = promptForRunMode()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if aborted {
			return
		}
	}

	if len(selectedIndices) == 0 {
		fmt.Println("No prompts selected.")
		return
	}

	if strings.TrimSpace(modelStr) == "" {
		var modelSelectionAborted bool
		modelStr, modelSelectionAborted = promptModelSelector("Select or type a model ID", false)
		if modelSelectionAborted {
			return
		}
	}
	if modelStr == "" {
		modelStr = defaultModelID
	}

	tasks := make([]EvalTask, len(selectedIndices))
	for i, idx := range selectedIndices {
		definition := prompts[idx]
		tasks[i] = EvalTask{
			Definition:   &definition,
			PromptID:     definition.ID,
			PromptTitle:  definition.Title,
			Track:        definition.Track,
			PromptNumber: idx + 1,
		}
	}

	fmt.Printf("\nStarting %d eval(s) with model: %s\n", len(tasks), modelStr)
	if selectedTemplateName != "" {
		fmt.Printf("Template: %s\n", selectedTemplateName)
	}
	fmt.Printf("Mode: %s\n", runMode)
	fmt.Printf("Inactivity timeout: %ds · transient retries: %d\n", int(inactivityTimeout.Seconds()), transientRetries)
	fmt.Println(strings.Repeat("─", 50))

	var results []EvalResult
	if runMode == "parallel" {
		results = runAllEvalsParallel(tasks, modelStr)
	} else {
		results = runAllEvalsSequential(tasks, modelStr)
	}

	fmt.Printf("\n%s\n", strings.Repeat("═", 50))
	fmt.Println("SUMMARY")
	fmt.Println(strings.Repeat("═", 50))

	for _, result := range results {
		status := "✓"
		if !result.Success {
			status = "✗"
		}
		fmt.Printf("%s [%ds] %s\n", status, int(result.Duration.Seconds()), result.Folder)
		if !result.AgentSuccess {
			fmt.Printf("  Agent: failed\n")
		} else {
			fmt.Printf("  Agent: ok\n")
		}
		fmt.Printf("  Validation: %t (%s / %s)\n", result.Validation.ValidationSuccess(), result.Validation.RunMode, result.Validation.PreviewMode)
		if result.Error != "" {
			fmt.Printf("  Error: %s\n", result.Error)
		}
		if len(result.Validation.Violations) > 0 {
			fmt.Printf("  Violations: %s\n", strings.Join(result.Validation.Violations, "; "))
		}
	}

	successful := 0
	for _, r := range results {
		if r.Success {
			successful++
		}
	}
	fmt.Printf("\n%d/%d evals completed successfully\n", successful, len(results))
}

func resumeCommand() {
	fs := flag.NewFlagSet("resume", flag.ExitOnError)
	flagInactivityTimeout := fs.Int("inactivity-timeout", int(defaultInactivityTimeout.Seconds()), "Inactivity timeout in seconds before failing a run")
	flagRetries := fs.Int("retries", defaultTransientRetries, "Retries for transient failures (timeout/stream errors)")
	if len(os.Args) > 2 {
		fs.Parse(os.Args[2:])
	}
	applyRuntimeOptions(*flagInactivityTimeout, *flagRetries)

	folders, err := scanEvalFolders()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error scanning eval folders: %v\n", err)
		os.Exit(1)
	}

	if len(folders) == 0 {
		fmt.Println("No eval folders found in evals/. Run 'high-evals run' first.")
		return
	}

	options := make([]huh.Option[int], len(folders))
	for i, ef := range folders {
		status := "?"
		extra := ""
		if ef.Result != nil {
			if ef.Result.Success {
				status = "✓"
			} else {
				status = "✗"
			}
			extra = fmt.Sprintf(" [%s, %ds]", ef.Result.Model, ef.Result.DurationSeconds)
		}

		preview := ef.Prompt
		if len(preview) > 50 {
			preview = preview[:47] + "..."
		}

		promptTag := "p?"
		if ef.PromptNumber > 0 {
			promptTag = fmt.Sprintf("p%d", ef.PromptNumber)
		}

		label := fmt.Sprintf("%s [%s] %s — %s%s", status, promptTag, filepath.Base(ef.Path), preview, extra)
		options[i] = huh.NewOption(label, i)
	}

	var selectedIndices []int
	var runMode string

	form := newEscBackForm(
		huh.NewGroup(
			huh.NewMultiSelect[int]().
				Title("Select evals to resume").
				Description("✓ = succeeded, ✗ = failed, ? = incomplete").
				Options(options...).
				Value(&selectedIndices).
				Filterable(true),
		),
	)

	aborted, err := runFormWithBack(form)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if aborted {
		return
	}

	if len(selectedIndices) == 0 {
		fmt.Println("No evals selected.")
		return
	}

	runMode, aborted, err = promptForRunMode()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if aborted {
		return
	}

	tasks := buildResumeTasks(folders, selectedIndices, "")

	fmt.Printf("\nResuming %d eval(s)\n", len(tasks))
	fmt.Printf("Model selection: stored per eval (fallback: %s)\n", defaultModelID)
	fmt.Printf("Mode: %s\n", runMode)
	fmt.Printf("Inactivity timeout: %ds · transient retries: %d\n", int(inactivityTimeout.Seconds()), transientRetries)
	fmt.Println(strings.Repeat("─", 50))

	var results []EvalResult
	if runMode == "parallel" {
		results = runAllEvalsParallel(tasks, defaultModelID)
	} else {
		results = runAllEvalsSequential(tasks, defaultModelID)
	}

	fmt.Printf("\n%s\n", strings.Repeat("═", 50))
	fmt.Println("SUMMARY")
	fmt.Println(strings.Repeat("═", 50))

	for _, result := range results {
		status := "✓"
		if !result.Success {
			status = "✗"
		}
		fmt.Printf("%s [%ds] %s\n", status, int(result.Duration.Seconds()), result.Folder)
		if !result.AgentSuccess {
			fmt.Printf("  Agent: failed\n")
		} else {
			fmt.Printf("  Agent: ok\n")
		}
		fmt.Printf("  Validation: %t (%s / %s)\n", result.Validation.ValidationSuccess(), result.Validation.RunMode, result.Validation.PreviewMode)
		if result.Error != "" {
			fmt.Printf("  Error: %s\n", result.Error)
		}
		if len(result.Validation.Violations) > 0 {
			fmt.Printf("  Violations: %s\n", strings.Join(result.Validation.Violations, "; "))
		}
	}

	successful := 0
	for _, r := range results {
		if r.Success {
			successful++
		}
	}
	fmt.Printf("\n%d/%d evals completed successfully\n", successful, len(results))
}

func opencodeSDKScriptPath() (string, error) {
	return filepath.Abs(filepath.Join("scripts", "opencode-sdk.ts"))
}

func defaultRunBunSDKCommand(dir string, args []string, input []byte) ([]byte, []byte, error) {
	scriptPath, err := opencodeSDKScriptPath()
	if err != nil {
		return nil, nil, err
	}

	commandArgs := append([]string{scriptPath}, args...)
	cmd := exec.Command("bun", commandArgs...)
	cmd.Dir = dir
	if len(input) > 0 {
		cmd.Stdin = bytes.NewReader(input)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

func parseBunSDKJSONOutput(stdout []byte, output interface{}) error {
	trimmed := bytes.TrimSpace(stdout)
	if len(trimmed) == 0 {
		return errors.New("empty JSON output from Bun SDK helper")
	}

	if err := json.Unmarshal(trimmed, output); err != nil {
		return fmt.Errorf("parsing Bun SDK helper output: %w (output: %s)", err, string(trimmed))
	}

	return nil
}

func formatBunSDKCommandError(err error, stdout, stderr []byte) error {
	stderrText := strings.TrimSpace(string(stderr))
	stdoutText := strings.TrimSpace(string(stdout))

	switch {
	case stderrText != "":
		return fmt.Errorf("%w: %s", err, stderrText)
	case stdoutText != "":
		return fmt.Errorf("%w: %s", err, stdoutText)
	default:
		return err
	}
}

func runBunSDKJSONHelper(dir string, args []string, input interface{}, output interface{}) error {
	var body []byte
	if input != nil {
		encoded, err := json.Marshal(input)
		if err != nil {
			return fmt.Errorf("encoding Bun SDK input: %w", err)
		}
		body = encoded
	}

	stdout, stderr, err := runBunSDKCommand(dir, args, body)
	if err != nil {
		return formatBunSDKCommandError(err, stdout, stderr)
	}

	if output == nil {
		return nil
	}

	return parseBunSDKJSONOutput(stdout, output)
}

func runEvalWithBunSDK(folderPath string, request sdkRunEvalRequest) (sdkRunEvalResponse, error) {
	var response sdkRunEvalResponse
	if err := runBunSDKJSONHelper(folderPath, []string{"run-eval"}, request, &response); err != nil {
		return sdkRunEvalResponse{}, err
	}
	return response, nil
}

func getProvidersData() (ProvidersData, error) {
	var providersData ProvidersData
	err := runBunSDKJSONHelper(
		".",
		[]string{"providers", "--hostname", "127.0.0.1", "--port", fmt.Sprintf("%d", basePort)},
		nil,
		&providersData,
	)
	if err != nil {
		return ProvidersData{}, err
	}
	return providersData, nil
}

func printProviders(data ProvidersData, savedSet map[string]struct{}) {
	if len(data.Providers) == 0 {
		fmt.Println("No providers returned by opencode.")
		return
	}

	sort.Slice(data.Providers, func(i, j int) bool {
		return data.Providers[i].ID < data.Providers[j].ID
	})

	fmt.Println("Available providers and model IDs:")
	for _, provider := range data.Providers {
		modelIDs := make([]string, 0, len(provider.Models))
		for modelID := range provider.Models {
			modelIDs = append(modelIDs, modelID)
		}
		sort.Strings(modelIDs)
		orderedModelIDs := pinSavedModelIDs(provider.ID, modelIDs, savedSet)

		defaultModel := data.Default[provider.ID]
		if defaultModel != "" {
			fmt.Printf("\n- %s (%d model(s), default: %s)\n", provider.ID, len(orderedModelIDs), defaultModel)
		} else {
			fmt.Printf("\n- %s (%d model(s))\n", provider.ID, len(orderedModelIDs))
		}

		for _, modelID := range orderedModelIDs {
			fullModelID := provider.ID + "/" + modelID
			if isSavedModel(savedSet, fullModelID) {
				fmt.Printf("  [saved] %s\n", fullModelID)
				continue
			}
			fmt.Printf("  %s\n", fullModelID)
		}
	}
}

func checkModelCommand(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: high-evals models check <provider/model>")
		os.Exit(1)
	}

	model := normalizeModelID(strings.TrimSpace(args[0]))
	if model == "" {
		fmt.Fprintln(os.Stderr, "Model ID cannot be empty.")
		os.Exit(1)
	}

	providersData, err := getProvidersData()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error fetching providers/models: %v\n", err)
		os.Exit(1)
	}

	savedSet, err := loadSavedModelSet()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not load saved models for pinning: %v\n", err)
		savedSet = map[string]struct{}{}
	}

	if isKnownModel(providersData, model) {
		fmt.Printf("Available: %s\n", model)
		if isSavedModel(savedSet, model) {
			fmt.Println("Pinned: yes (saved in saved-models.json)")
		} else {
			fmt.Printf("Pinned: no (run 'high-evals models save %s' to pin it)\n", model)
		}
		return
	}

	fmt.Printf("Not available: %s\n", model)

	allModels := flattenModelIDs(providersData)
	suggestions := filterModels(allModels, model)
	suggestions = pinSavedModels(suggestions, savedSet)
	if len(suggestions) == 0 {
		os.Exit(1)
	}

	limit := len(suggestions)
	if limit > 8 {
		limit = 8
	}

	fmt.Println("\nClosest matches:")
	for i := 0; i < limit; i++ {
		prefix := "  "
		if isSavedModel(savedSet, suggestions[i]) {
			prefix = "  [saved] "
		}
		fmt.Printf("%s%s\n", prefix, suggestions[i])
	}
	os.Exit(1)
}

func saveModelsCommand(args []string) {
	providersData, err := getProvidersData()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error fetching providers/models: %v\n", err)
		os.Exit(1)
	}

	modelsToSave := make([]string, 0)

	if len(args) > 0 {
		model := normalizeModelID(strings.TrimSpace(args[0]))
		if model == "" {
			fmt.Fprintln(os.Stderr, "Model ID cannot be empty.")
			os.Exit(1)
		}
		modelsToSave = append(modelsToSave, model)
	} else {
		allModels := flattenModelIDs(providersData)
		if len(allModels) == 0 {
			fmt.Fprintln(os.Stderr, "No models available to save.")
			os.Exit(1)
		}

		selected, err := promptModelsToSave(allModels)
		if err != nil {
			if errors.Is(err, huh.ErrUserAborted) {
				return
			}
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		if len(selected) == 0 {
			fmt.Println("No models selected.")
			return
		}

		modelsToSave = selected
	}

	for _, model := range modelsToSave {
		if !isKnownModel(providersData, model) {
			fmt.Fprintf(os.Stderr, "Unknown model: %s\n", model)
			os.Exit(1)
		}
	}

	existing, err := loadSavedModels()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading saved models: %v\n", err)
		os.Exit(1)
	}

	set := make(map[string]struct{}, len(existing))
	for _, model := range existing {
		set[model] = struct{}{}
	}

	added := 0
	for _, model := range modelsToSave {
		if _, exists := set[model]; exists {
			continue
		}
		existing = append(existing, model)
		set[model] = struct{}{}
		added++
	}

	sort.Strings(existing)
	if err := saveSavedModels(existing); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving models: %v\n", err)
		os.Exit(1)
	}

	if added == 0 {
		fmt.Printf("No new models added. Saved models are already up to date in %s.\n", savedModelsFile)
		return
	}

	fmt.Printf("Saved %d model(s) to %s.\n", added, savedModelsFile)
}

func promptModelsToSave(allModels []string) ([]string, error) {
	searchQuery := ""
	savedSet, err := loadSavedModelSet()
	if err != nil {
		return nil, fmt.Errorf("loading saved models: %w", err)
	}

	for {
		inputForm := newEscBackForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Search models").
					Description("Type part of provider/model (leave empty to show all models)").
					Placeholder("e.g. openrouter/glm").
					Value(&searchQuery),
			),
		)
		aborted, err := runFormWithBack(inputForm)
		if err != nil {
			return nil, err
		}
		if aborted {
			return nil, huh.ErrUserAborted
		}

		filtered := filterModels(allModels, searchQuery)
		if len(filtered) == 0 {
			fmt.Fprintf(os.Stderr, "No models matched %q. Try another search.\n", searchQuery)
			continue
		}
		filtered = pinSavedModels(filtered, savedSet)

		searchLabel := strings.TrimSpace(searchQuery)
		if searchLabel == "" {
			searchLabel = "all models"
		}

		var selected []string
		options := make([]huh.Option[string], len(filtered))
		for i, model := range filtered {
			label := model
			if isSavedModel(savedSet, model) {
				label = "[saved] " + model
			}
			options[i] = huh.NewOption(label, model)
		}

		selectForm := newEscBackForm(
			huh.NewGroup(
				huh.NewMultiSelect[string]().
					Title("Select model(s) to save").
					Description(fmt.Sprintf("Search: %s (%d/%d shown). Saved models are pinned first. Use space to select, enter to confirm.", searchLabel, len(filtered), len(allModels))).
					Options(options...).
					Value(&selected).
					Filterable(false),
			),
		)
		aborted, err = runFormWithBack(selectForm)
		if err != nil {
			return nil, err
		}
		if aborted {
			return nil, huh.ErrUserAborted
		}

		return selected, nil
	}
}

func listSavedModelsCommand() {
	saved, err := loadSavedModels()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading saved models: %v\n", err)
		os.Exit(1)
	}

	if len(saved) == 0 {
		fmt.Printf("No saved models yet. Use 'high-evals models save <provider/model>' to add one.\n")
		return
	}

	fmt.Printf("Saved models in %s:\n\n", savedModelsFile)
	for i, model := range saved {
		fmt.Printf("  %d. %s\n", i+1, model)
	}
}

func loadSavedModels() ([]string, error) {
	data, err := os.ReadFile(savedModelsFile)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}

	if len(data) == 0 {
		return []string{}, nil
	}

	var models []string
	if err := json.Unmarshal(data, &models); err != nil {
		return nil, err
	}

	return models, nil
}

func saveSavedModels(models []string) error {
	data, err := json.MarshalIndent(models, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(savedModelsFile, data, 0644)
}

func loadSavedModelSet() (map[string]struct{}, error) {
	savedModels, err := loadSavedModels()
	if err != nil {
		return nil, err
	}

	set := make(map[string]struct{}, len(savedModels))
	for _, model := range savedModels {
		set[model] = struct{}{}
	}
	return set, nil
}

func flattenModelIDs(data ProvidersData) []string {
	models := make([]string, 0)
	for _, provider := range data.Providers {
		for modelID := range provider.Models {
			models = append(models, provider.ID+"/"+modelID)
		}
	}
	sort.Strings(models)
	return models
}

func isSavedModel(savedSet map[string]struct{}, model string) bool {
	if len(savedSet) == 0 {
		return false
	}
	_, ok := savedSet[model]
	return ok
}

func pinSavedModels(models []string, savedSet map[string]struct{}) []string {
	if len(models) == 0 || len(savedSet) == 0 {
		return models
	}

	pinned := make([]string, 0, len(models))
	others := make([]string, 0, len(models))
	for _, model := range models {
		if isSavedModel(savedSet, model) {
			pinned = append(pinned, model)
		} else {
			others = append(others, model)
		}
	}

	return append(pinned, others...)
}

func pinSavedModelIDs(providerID string, modelIDs []string, savedSet map[string]struct{}) []string {
	if len(modelIDs) == 0 || len(savedSet) == 0 {
		return modelIDs
	}

	pinned := make([]string, 0, len(modelIDs))
	others := make([]string, 0, len(modelIDs))
	for _, modelID := range modelIDs {
		fullModelID := providerID + "/" + modelID
		if isSavedModel(savedSet, fullModelID) {
			pinned = append(pinned, modelID)
		} else {
			others = append(others, modelID)
		}
	}
	return append(pinned, others...)
}

func filterModels(models []string, query string) []string {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return models
	}

	lowerQuery := strings.ToLower(trimmed)
	normalizedQuery := normalizeForSearch(trimmed)
	queryTokens := splitSearchTokens(trimmed)

	type modelScore struct {
		model string
		score int
	}

	scored := make([]modelScore, 0, len(models))
	for _, model := range models {
		score, ok := scoreModelMatch(model, lowerQuery, normalizedQuery, queryTokens)
		if ok {
			scored = append(scored, modelScore{model: model, score: score})
		}
	}

	sort.Slice(scored, func(i, j int) bool {
		if scored[i].score == scored[j].score {
			return scored[i].model < scored[j].model
		}
		return scored[i].score > scored[j].score
	})

	filtered := make([]string, len(scored))
	for i, match := range scored {
		filtered[i] = match.model
	}

	return filtered
}

func scoreModelMatch(model, lowerQuery, normalizedQuery string, queryTokens []string) (int, bool) {
	lowerModel := strings.ToLower(model)
	normalizedModel := normalizeForSearch(model)
	score := 0
	matched := false

	if lowerQuery != "" && strings.Contains(lowerModel, lowerQuery) {
		score += 140
		matched = true
	}

	if normalizedQuery != "" {
		if strings.Contains(normalizedModel, normalizedQuery) {
			score += 120
			matched = true
		}

		if strings.HasPrefix(normalizedModel, normalizedQuery) {
			score += 30
		}

		if isSubsequence(normalizedQuery, normalizedModel) {
			score += 50
			matched = true
		}
	}

	tokenHits := 0
	tokenScore := 0
	searchPos := 0
	ordered := true

	for _, token := range queryTokens {
		if strings.Contains(lowerModel, token) {
			tokenHits++
			tokenScore += 20
		}

		if ordered {
			next := strings.Index(lowerModel[searchPos:], token)
			if next == -1 {
				ordered = false
			} else {
				searchPos += next + len(token)
			}
		}
	}

	if tokenHits > 0 {
		score += tokenScore
		matched = true
		if tokenHits == len(queryTokens) {
			score += 40
			if ordered && len(queryTokens) > 1 {
				score += 20
			}
		}
	}

	return score, matched
}

func normalizeForSearch(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func splitSearchTokens(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsDigit(r))
	})
}

func isSubsequence(query, target string) bool {
	if query == "" {
		return true
	}

	queryRunes := []rune(query)
	q := 0
	for _, r := range target {
		if q < len(queryRunes) && queryRunes[q] == r {
			q++
			if q == len(queryRunes) {
				return true
			}
		}
	}
	return false
}

func isKnownModel(data ProvidersData, fullModelID string) bool {
	parts := strings.SplitN(fullModelID, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return false
	}

	providerID := parts[0]
	modelID := parts[1]

	for _, provider := range data.Providers {
		if provider.ID != providerID {
			continue
		}
		_, ok := provider.Models[modelID]
		return ok
	}

	return false
}

func normalizeModelID(model string) string {
	if model == "" {
		return ""
	}
	if strings.Contains(model, "/") {
		return model
	}
	return "openrouter/" + model
}

func parseModel(modelStr string) (string, string) {
	idx := strings.Index(modelStr, "/")
	if idx != -1 {
		return modelStr[:idx], modelStr[idx+1:]
	}
	return "openrouter", modelStr
}

func sanitizeModelForFolder(model string) string {
	model = strings.ToLower(normalizeModelID(strings.TrimSpace(model)))
	if model == "" {
		return "unknown-model"
	}

	var b strings.Builder
	b.Grow(len(model))
	prevDash := false

	for _, r := range model {
		isASCIIAlpha := r >= 'a' && r <= 'z'
		isASCIIDigit := r >= '0' && r <= '9'
		switch {
		case isASCIIAlpha || isASCIIDigit || r == '-' || r == '_' || r == '.':
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}

	sanitized := strings.Trim(b.String(), "-_.")
	if sanitized == "" {
		return "unknown-model"
	}

	const maxLen = 64
	if len(sanitized) > maxLen {
		sanitized = strings.TrimRight(sanitized[:maxLen], "-_.")
		if sanitized == "" {
			return "unknown-model"
		}
	}

	return sanitized
}

func createTimestampFolder(index, promptNumber int, model string) string {
	now := time.Now()
	if promptNumber < 1 {
		promptNumber = 0
	}
	return fmt.Sprintf("evals/%d-%02d-%02d_%02d-%02d-%02d_p%d_%d_%s",
		now.Year(), now.Month(), now.Day(),
		now.Hour(), now.Minute(), now.Second(), promptNumber, index, sanitizeModelForFolder(model))
}

func parsePromptNumberFromFolder(folderName string) int {
	matches := promptNumberRE.FindStringSubmatch(folderName)
	if len(matches) < 2 {
		return 0
	}

	n, err := strconv.Atoi(matches[1])
	if err != nil || n < 1 {
		return 0
	}
	return n
}

func buildPromptNumberByPrompt() map[string]int {
	prompts, err := loadPrompts()
	if err != nil || len(prompts) == 0 {
		return map[string]int{}
	}

	m := make(map[string]int, len(prompts))
	for i, p := range prompts {
		if _, exists := m[p.Prompt]; exists {
			continue
		}
		m[p.Prompt] = i + 1
	}
	return m
}

func buildPromptNumberByID() map[string]int {
	prompts, err := loadPrompts()
	if err != nil || len(prompts) == 0 {
		return map[string]int{}
	}

	m := make(map[string]int, len(prompts))
	for i, prompt := range prompts {
		if prompt.ID == "" {
			continue
		}
		if _, exists := m[prompt.ID]; exists {
			continue
		}
		m[prompt.ID] = i + 1
	}
	return m
}

func setupEvalFolder(folderPath, prompt string) error {
	if err := os.MkdirAll(folderPath, 0755); err != nil {
		return err
	}

	packageJSON := map[string]interface{}{
		"name":    strings.ReplaceAll(folderPath, "/", "-"),
		"type":    "module",
		"private": true,
	}
	packageData, _ := json.MarshalIndent(packageJSON, "", "  ")
	if err := os.WriteFile(filepath.Join(folderPath, "package.json"), packageData, 0644); err != nil {
		return err
	}

	if err := os.WriteFile(filepath.Join(folderPath, "prompt.txt"), []byte(prompt), 0644); err != nil {
		return err
	}

	return nil
}

func saveEvalResult(folderPath string, result EvalResult, model string) {
	rf := EvalResultFile{
		Prompt:            result.Prompt,
		PromptID:          result.PromptID,
		PromptTitle:       result.PromptTitle,
		PromptNumber:      result.PromptNumber,
		Model:             model,
		Track:             result.Track,
		Success:           result.Success,
		AgentSuccess:      result.AgentSuccess,
		ValidationSuccess: result.Validation.ValidationSuccess(),
		Error:             result.Error,
		DurationSeconds:   int(result.Duration.Seconds()),
		CompletedAt:       time.Now().Format(time.RFC3339),
		PreviewMode:       result.Validation.PreviewMode,
		RunMode:           result.Validation.RunMode,
		Violations:        cloneStringSlice(result.Validation.Violations),
		Checks:            cloneBoolMap(result.Validation.Checks),
	}
	data, err := json.MarshalIndent(rf, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(folderPath, "result.json"), data, 0644)
}

func scanEvalFolders() ([]EvalFolder, error) {
	entries, err := os.ReadDir("evals")
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	promptNumberByText := buildPromptNumberByPrompt()
	promptNumberByID := buildPromptNumberByID()
	var folders []EvalFolder
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join("evals", entry.Name())

		promptData, err := os.ReadFile(filepath.Join(path, "prompt.txt"))
		if err != nil {
			continue
		}

		ef := EvalFolder{
			Path:   path,
			Prompt: string(promptData),
		}

		resultData, err := os.ReadFile(filepath.Join(path, "result.json"))
		if err == nil {
			var rf EvalResultFile
			if json.Unmarshal(resultData, &rf) == nil {
				ef.Result = &rf
				ef.PromptID = rf.PromptID
				ef.PromptTitle = rf.PromptTitle
				ef.Track = rf.Track
				if rf.PromptNumber > 0 {
					ef.PromptNumber = rf.PromptNumber
				}
			}
		}
		if ef.PromptNumber == 0 && ef.PromptID != "" {
			if n, ok := promptNumberByID[ef.PromptID]; ok {
				ef.PromptNumber = n
			}
		}
		if ef.PromptNumber == 0 {
			ef.PromptNumber = parsePromptNumberFromFolder(filepath.Base(path))
		}
		if ef.PromptNumber == 0 {
			if n, ok := promptNumberByText[ef.Prompt]; ok {
				ef.PromptNumber = n
			}
		}

		folders = append(folders, ef)
	}

	return folders, nil
}

type EvalTask struct {
	Definition   *PromptDefinition
	Prompt       string
	PromptID     string
	PromptTitle  string
	Track        PromptTrack
	PromptNumber int
	Folder       string // empty = create new folder
	Model        string
}

func buildResumeTasks(folders []EvalFolder, selectedIndices []int, overrideModel string) []EvalTask {
	tasks := make([]EvalTask, len(selectedIndices))
	trimmedOverride := strings.TrimSpace(overrideModel)

	for i, idx := range selectedIndices {
		ef := folders[idx]
		taskModel := trimmedOverride
		if taskModel == "" && ef.Result != nil {
			taskModel = strings.TrimSpace(ef.Result.Model)
		}

		tasks[i] = EvalTask{
			Prompt:       ef.Prompt,
			PromptID:     ef.PromptID,
			PromptTitle:  ef.PromptTitle,
			Track:        ef.Track,
			PromptNumber: ef.PromptNumber,
			Folder:       ef.Path,
			Model:        taskModel,
		}
	}

	return tasks
}

func resolveTaskModel(task EvalTask, fallbackModel string) string {
	if strings.TrimSpace(task.Model) != "" {
		return strings.TrimSpace(task.Model)
	}
	return fallbackModel
}

func resolveTaskTrack(task EvalTask) PromptTrack {
	if task.Definition != nil {
		return normalizePromptTrack(task.Definition.Track, task.Definition.Prompt)
	}
	if task.Track != "" {
		return normalizePromptTrack(task.Track, task.Prompt)
	}
	return inferPromptTrack(task.Prompt)
}

func runAllEvalsParallel(tasks []EvalTask, model string) []EvalResult {
	var wg sync.WaitGroup
	results := make([]EvalResult, len(tasks))
	resultMutex := &sync.Mutex{}

	for i, task := range tasks {
		wg.Add(1)
		go func(index int, t EvalTask) {
			defer wg.Done()
			result := runAgentWithRetry(t, index, resolveTaskModel(t, model))
			resultMutex.Lock()
			results[index] = result
			resultMutex.Unlock()
		}(i, task)
	}

	wg.Wait()
	return results
}

func runAllEvalsSequential(tasks []EvalTask, model string) []EvalResult {
	results := make([]EvalResult, len(tasks))
	defaultModel := model

	for i, task := range tasks {
		taskModel := resolveTaskModel(task, defaultModel)
		results[i] = runAgentWithRetry(task, i, taskModel)

		// On model-not-found, prompt user to correct and re-run this eval
		if !results[i].Success {
			isModelErr, suggestions := isModelNotFoundError(results[i].Error)
			if isModelErr {
				fmt.Printf("\n[%d] Model not found: %s\n", i, taskModel)
				corrected, correctionAborted := promptModelCorrection(taskModel, suggestions)
				if correctionAborted || corrected == "" {
					fmt.Println("No model selected, aborting remaining evals.")
					return results
				}
				if strings.TrimSpace(task.Model) == "" {
					defaultModel = corrected
				} else {
					taskModel = corrected
				}
				fmt.Printf("[%d] Retrying with model: %s\n", i, corrected)
				results[i] = runAgentWithRetry(task, i, corrected)
			}
		}
	}
	return results
}

func runAgentWithRetry(task EvalTask, index int, modelStr string) EvalResult {
	maxAttempts := transientRetries + 1
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	folder := task.Folder
	var result EvalResult

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 {
			fmt.Printf("[%d] Retry attempt %d/%d after transient failure\n", index, attempt-1, transientRetries)
		}

		task.Folder = folder
		result = runAgent(task, index, modelStr)
		folder = result.Folder

		if result.Success || !isTransientEvalError(result.Error) || attempt == maxAttempts {
			return result
		}
	}

	return result
}

func runAgent(task EvalTask, index int, modelStr string) EvalResult {
	startTime := time.Now()

	folderPath := task.Folder
	if folderPath == "" {
		folderPath = createTimestampFolder(index, task.PromptNumber, modelStr)
	} else if task.PromptNumber < 1 {
		task.PromptNumber = parsePromptNumberFromFolder(filepath.Base(folderPath))
	}

	fmt.Printf("[%d] Starting eval in %s\n", index, folderPath)

	renderedPrompt := task.Prompt
	if strings.TrimSpace(renderedPrompt) == "" && task.Definition != nil {
		renderedPrompt = renderPrompt(*task.Definition, modelStr, folderPath)
	}

	result := EvalResult{
		Prompt:       renderedPrompt,
		PromptID:     task.PromptID,
		PromptTitle:  task.PromptTitle,
		Track:        resolveTaskTrack(task),
		PromptNumber: task.PromptNumber,
		Folder:       folderPath,
		Success:      false,
		Duration:     0,
		Validation: ValidationReport{
			Track:   resolveTaskTrack(task),
			RunMode: detectRunMode(folderPath, resolveTaskTrack(task)),
		},
	}

	if task.Folder == "" {
		if err := setupEvalFolder(folderPath, renderedPrompt); err != nil {
			result.Error = fmt.Sprintf("Failed to setup folder: %v", err)
			result.Duration = time.Since(startTime)
			saveEvalResult(folderPath, result, modelStr)
			return result
		}
	}

	port := basePort + index
	providerID, modelID := parseModel(modelStr)
	fmt.Printf("[%d] Sending prompt via Bun SDK...\n", index)

	sdkResult, err := runEvalWithBunSDK(folderPath, sdkRunEvalRequest{
		Title:                    fmt.Sprintf("Eval %d", index),
		Prompt:                   renderedPrompt,
		ProviderID:               providerID,
		ModelID:                  modelID,
		Hostname:                 "127.0.0.1",
		Port:                     port,
		InactivityTimeoutSeconds: int(inactivityTimeout.Seconds()),
	})
	if err != nil {
		result.Error = fmt.Sprintf("Failed to run eval via Bun SDK: %v", err)
		result.Duration = time.Since(startTime)
		saveEvalResult(folderPath, result, modelStr)
		return result
	}

	if sdkResult.SessionID != "" {
		fmt.Printf("[%d] Session created: %s\n", index, sdkResult.SessionID)
	}

	result.Duration = time.Since(startTime)
	if sdkResult.DurationMs > 0 {
		result.Duration = time.Duration(sdkResult.DurationMs) * time.Millisecond
	}

	fmt.Printf("[%d] Completed in %ds\n", index, int(result.Duration.Seconds()))

	result.AgentSuccess = sdkResult.Success && sdkResult.Error == ""
	if sdkResult.Error != "" {
		result.Error = sdkResult.Error
	} else if !sdkResult.Success {
		result.Error = "agent did not reach idle state"
	}
	result.Validation = validateEvalFolder(folderPath, result.Track)
	if !result.Validation.ValidationSuccess() {
		if result.Error == "" {
			result.Error = strings.Join(result.Validation.Violations, "; ")
		} else {
			result.Error = fmt.Sprintf("%s | validation: %s", result.Error, strings.Join(result.Validation.Violations, "; "))
		}
	}
	result.Success = result.AgentSuccess && result.Validation.ValidationSuccess()

	saveEvalResult(folderPath, result, modelStr)
	return result
}

func isTransientEvalError(errMsg string) bool {
	if errMsg == "" {
		return false
	}
	return strings.Contains(errMsg, "no agent activity for") ||
		strings.Contains(errMsg, "event stream error:") ||
		strings.Contains(errMsg, "agent did not reach idle state")
}

func applyRuntimeOptions(timeoutSeconds, retries int) {
	if timeoutSeconds < 1 {
		timeoutSeconds = int(defaultInactivityTimeout.Seconds())
	}
	if retries < 0 {
		retries = defaultTransientRetries
	}

	inactivityTimeout = time.Duration(timeoutSeconds) * time.Second
	transientRetries = retries
}

func isModelNotFoundError(errMsg string) (bool, []string) {
	if !strings.Contains(errMsg, "Model not found") {
		return false, nil
	}
	idx := strings.Index(errMsg, "Did you mean: ")
	if idx == -1 {
		return true, nil
	}
	suggestionsStr := errMsg[idx+len("Did you mean: "):]
	suggestionsStr = strings.TrimSuffix(suggestionsStr, "?")
	suggestions := strings.Split(suggestionsStr, ", ")
	for i := range suggestions {
		suggestions[i] = strings.TrimSpace(suggestions[i])
	}
	return true, suggestions
}

func promptModelSelector(description string, allowEmpty bool) (string, bool) {
	savedModels, _ := loadSavedModels()

	if len(savedModels) == 0 {
		var modelStr string
		form := newEscBackForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Model to use").
					Description(description).
					Placeholder("e.g. openrouter/z-ai/glm-5").
					Value(&modelStr),
			),
		)
		aborted, err := runFormWithBack(form)
		if err != nil {
			return "", true
		}
		if aborted {
			return "", true
		}
		return strings.TrimSpace(modelStr), false
	}

	// Show saved models as a filterable select with custom option
	options := make([]huh.Option[string], 0, len(savedModels)+2)
	if allowEmpty {
		options = append(options, huh.NewOption("Keep stored model(s)", "__keep__"))
	}
	for _, m := range savedModels {
		options = append(options, huh.NewOption(m, m))
	}
	options = append(options, huh.NewOption("Type a different model...", "__custom__"))

	var selected string
	form := newEscBackForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Model to use").
				Description(description).
				Options(options...).
				Value(&selected),
		),
	)
	aborted, err := runFormWithBack(form)
	if err != nil {
		return "", true
	}
	if aborted {
		return "", true
	}

	if selected == "__keep__" {
		return "", false
	}

	if selected != "__custom__" {
		return selected, false
	}

	// Custom model input
	var modelStr string
	inputForm := newEscBackForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Enter model ID").
				Placeholder("e.g. openrouter/z-ai/glm-5").
				Value(&modelStr),
		),
	)
	aborted, err = runFormWithBack(inputForm)
	if err != nil {
		return "", true
	}
	if aborted {
		return "", true
	}
	return strings.TrimSpace(modelStr), false
}

func promptModelCorrection(currentModel string, suggestions []string) (string, bool) {
	savedModels, _ := loadSavedModels()

	// Build options: suggestions first, then saved models, then custom
	seen := make(map[string]bool)
	options := make([]huh.Option[string], 0)

	for _, s := range suggestions {
		if s != "" && !seen[s] {
			options = append(options, huh.NewOption(s+" (suggested)", s))
			seen[s] = true
		}
	}
	for _, m := range savedModels {
		if !seen[m] {
			options = append(options, huh.NewOption(m+" (saved)", m))
			seen[m] = true
		}
	}
	options = append(options, huh.NewOption("Type a different model...", "__custom__"))

	var selected string
	form := newEscBackForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Pick the correct model").
				Description(fmt.Sprintf("'%s' was not found", currentModel)).
				Options(options...).
				Value(&selected),
		),
	)
	aborted, err := runFormWithBack(form)
	if err != nil {
		return "", true
	}
	if aborted {
		return "", true
	}

	if selected != "__custom__" {
		return selected, false
	}

	var modelStr string
	inputForm := newEscBackForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Enter model ID").
				Placeholder("e.g. openrouter/z-ai/glm-5").
				Value(&modelStr),
		),
	)
	aborted, err = runFormWithBack(inputForm)
	if err != nil {
		return "", true
	}
	if aborted {
		return "", true
	}
	return strings.TrimSpace(modelStr), false
}
