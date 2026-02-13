package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/charmbracelet/huh"
)

const (
	defaultInactivityTimeout = 180 * time.Second
	defaultTransientRetries  = 1
	eventScannerMaxTokenSize = 8 * 1024 * 1024
	basePort                 = 4096
	promptsFile              = "prompts.json"
	savedModelsFile          = "saved-models.json"
)

var (
	inactivityTimeout = defaultInactivityTimeout
	transientRetries  = defaultTransientRetries
)

type EvalResult struct {
	Prompt   string
	Folder   string
	Success  bool
	Error    string
	Duration time.Duration
}

type PromptJSON []string

type Session struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Slug  string `json:"slug"`
}

type Event struct {
	Type       string                 `json:"type"`
	Properties map[string]interface{} `json:"properties"`
}

type Model struct {
	ProviderID string `json:"providerID"`
	ModelID    string `json:"modelID"`
}

type PromptPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type PromptRequest struct {
	Model Model        `json:"model"`
	Parts []PromptPart `json:"parts"`
}

type EvalResultFile struct {
	Prompt          string `json:"prompt"`
	Model           string `json:"model"`
	Success         bool   `json:"success"`
	Error           string `json:"error,omitempty"`
	DurationSeconds int    `json:"duration_seconds"`
	CompletedAt     string `json:"completed_at"`
}

type EvalFolder struct {
	Path   string
	Prompt string
	Result *EvalResultFile
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

type ProvidersResponse struct {
	Data      ProvidersData     `json:"data"`
	Providers []Provider        `json:"providers"`
	Default   map[string]string `json:"default"`
}

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
	case "models":
		modelsCommand(os.Args[2:])
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

		form := huh.NewForm(
			huh.NewGroup(
				huh.NewSelect[string]().
					Title("High-Evals").
					Description(fmt.Sprintf("%d prompt(s) · %d eval(s) · %d saved model(s)", promptCount, evalCount, savedCount)).
					Options(
						huh.NewOption("Run evals        select prompts and model, then run", "run"),
						huh.NewOption("Resume evals     re-run previous evals from evals/", "resume"),
						huh.NewOption("Manage models    browse, search and save models", "models"),
						huh.NewOption("List prompts     show all prompts in prompts.json", "list"),
						huh.NewOption("Add prompt       create a new prompt", "add"),
						huh.NewOption("Edit prompt      modify an existing prompt", "edit"),
						huh.NewOption("Remove prompt    delete a prompt", "remove"),
						huh.NewOption("Exit", "exit"),
					).
					Value(&action),
			),
		)

		if err := form.Run(); err != nil {
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
		case "models":
			interactiveModelsCommand()
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
  models   Interactively browse and save models for reuse
  list     List all prompts in prompts.json
  add      Add a new prompt to prompts.json
  edit     Edit an existing prompt
  remove   Remove a prompt from prompts.json
  help     Show this help message

Examples:
  high-evals run
  high-evals resume
  high-evals models
  high-evals models list
  high-evals models check openrouter/glm-5
  high-evals models saved
  high-evals add
  high-evals list`)
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

	var prompts PromptJSON
	if err := json.Unmarshal(data, &prompts); err != nil {
		return nil, err
	}

	return prompts, nil
}

func savePrompts(prompts PromptJSON) error {
	data, err := json.MarshalIndent(prompts, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(promptsFile, data, 0644)
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
		preview := p
		if len(preview) > 80 {
			preview = preview[:77] + "..."
		}
		fmt.Printf("  %d. %s\n", i+1, preview)
	}
	fmt.Printf("\nTotal: %d prompt(s)\n", len(prompts))
}

func addCommand() {
	var newPrompt string

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewText().
				Title("Enter the new prompt").
				Description("Write a coding task for the agent to complete").
				Value(&newPrompt).
				CharLimit(2000),
		),
	)

	if err := form.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
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

	prompts = append(prompts, newPrompt)

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
		preview := p
		if len(preview) > 60 {
			preview = preview[:57] + "..."
		}
		options[i] = huh.NewOption(fmt.Sprintf("%d. %s", i+1, preview), i)
	}

	selectForm := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[int]().
				Title("Select a prompt to edit").
				Options(options...).
				Value(&selectedIdx),
		),
	)

	if err := selectForm.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	editedPrompt := prompts[selectedIdx]

	editForm := huh.NewForm(
		huh.NewGroup(
			huh.NewText().
				Title("Edit the prompt").
				Value(&editedPrompt).
				CharLimit(2000),
		),
	)

	if err := editForm.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	editedPrompt = strings.TrimSpace(editedPrompt)
	if editedPrompt == "" {
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
		preview := p
		if len(preview) > 60 {
			preview = preview[:57] + "..."
		}
		options[i] = huh.NewOption(fmt.Sprintf("%d. %s", i+1, preview), i)
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[int]().
				Title("Select a prompt to remove").
				Options(options...).
				Value(&selectedIdx),
		),
	)

	if err := form.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	var confirmRemove bool
	confirmForm := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(fmt.Sprintf("Remove prompt #%d?", selectedIdx+1)).
				Value(&confirmRemove),
		),
	)

	if err := confirmForm.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
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
	flagMode := fs.String("mode", "sequential", "Execution mode: parallel or sequential")
	flagInactivityTimeout := fs.Int("inactivity-timeout", int(defaultInactivityTimeout.Seconds()), "Inactivity timeout in seconds before failing a run")
	flagRetries := fs.Int("retries", defaultTransientRetries, "Retries for transient failures (timeout/stream errors)")
	if len(os.Args) > 2 {
		fs.Parse(os.Args[2:])
	}
	applyRuntimeOptions(*flagInactivityTimeout, *flagRetries)

	var selectedIndices []int
	var modelStr string
	var runMode string

	if *flagModel != "" && *flagPrompts != "" {
		// Non-interactive mode
		modelStr = *flagModel
		runMode = *flagMode
		for _, s := range strings.Split(*flagPrompts, ",") {
			s = strings.TrimSpace(s)
			idx, err := strconv.Atoi(s)
			if err != nil || idx < 1 || idx > len(prompts) {
				fmt.Fprintf(os.Stderr, "Invalid prompt index: %s (must be 1-%d)\n", s, len(prompts))
				os.Exit(1)
			}
			selectedIndices = append(selectedIndices, idx-1)
		}
	} else {
		// Interactive mode
		promptOptions := make([]huh.Option[int], len(prompts))
		for i, p := range prompts {
			preview := p
			if len(preview) > 60 {
				preview = preview[:57] + "..."
			}
			promptOptions[i] = huh.NewOption(fmt.Sprintf("%d. %s", i+1, preview), i)
		}

		form := huh.NewForm(
			huh.NewGroup(
				huh.NewMultiSelect[int]().
					Title("Select prompts to run").
					Options(promptOptions...).
					Value(&selectedIndices).
					Filterable(true),
			),
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

		if err := form.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		if len(selectedIndices) == 0 {
			fmt.Println("No prompts selected.")
			return
		}

		modelStr = promptModelSelector("Select or type a model ID")
		if modelStr == "" {
			modelStr = "opencode/kimi-k2.5-free"
		}
	}

	tasks := make([]EvalTask, len(selectedIndices))
	for i, idx := range selectedIndices {
		tasks[i] = EvalTask{Prompt: prompts[idx]}
	}

	fmt.Printf("\nStarting %d eval(s) with model: %s\n", len(tasks), modelStr)
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
		if result.Error != "" {
			fmt.Printf("  Error: %s\n", result.Error)
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

		label := fmt.Sprintf("%s %s — %s%s", status, filepath.Base(ef.Path), preview, extra)
		options[i] = huh.NewOption(label, i)
	}

	var selectedIndices []int
	var modelStr string
	var runMode string

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[int]().
				Title("Select evals to resume").
				Description("✓ = succeeded, ✗ = failed, ? = incomplete").
				Options(options...).
				Value(&selectedIndices).
				Filterable(true),
		),
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

	if err := form.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if len(selectedIndices) == 0 {
		fmt.Println("No evals selected.")
		return
	}

	modelStr = promptModelSelector("Select a model, or leave empty to re-use original")
	// modelStr may be empty — handled below per-eval

	tasks := make([]EvalTask, len(selectedIndices))
	for i, idx := range selectedIndices {
		ef := folders[idx]
		tasks[i] = EvalTask{
			Prompt: ef.Prompt,
			Folder: ef.Path,
		}

		if modelStr == "" && ef.Result != nil && ef.Result.Model != "" {
			if i == 0 {
				modelStr = ef.Result.Model
			}
		}
	}

	if modelStr == "" {
		modelStr = "opencode/kimi-k2.5-free"
	}

	// If user set a model, use it for all. If not, we already picked one above.
	// For per-eval model tracking, the model is saved in result.json per folder.

	fmt.Printf("\nResuming %d eval(s) with model: %s\n", len(tasks), modelStr)
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
		if result.Error != "" {
			fmt.Printf("  Error: %s\n", result.Error)
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

func fetchProviders(client *http.Client, baseURL string) (ProvidersData, error) {
	resp, err := client.Get(baseURL + "/config/providers")
	if err != nil {
		return ProvidersData{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return ProvidersData{}, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var payload ProvidersResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return ProvidersData{}, err
	}

	if len(payload.Data.Providers) > 0 || len(payload.Data.Default) > 0 {
		return payload.Data, nil
	}

	return ProvidersData{
		Providers: payload.Providers,
		Default:   payload.Default,
	}, nil
}

func getProvidersData() (ProvidersData, error) {
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", basePort)
	client := &http.Client{Timeout: 10 * time.Second}

	providersData, err := fetchProviders(client, baseURL)
	if err == nil {
		return providersData, nil
	}

	fmt.Printf("No opencode server detected on %s. Starting a temporary server...\n", baseURL)

	cmd := exec.Command("opencode", "--port", fmt.Sprintf("%d", basePort))
	cmd.Dir = "."
	if err := cmd.Start(); err != nil {
		return ProvidersData{}, fmt.Errorf("starting opencode: %w", err)
	}
	defer cmd.Process.Kill()

	if err := waitForProvidersEndpoint(client, baseURL, 5*time.Second); err != nil {
		return ProvidersData{}, fmt.Errorf("waiting for opencode server: %w", err)
	}

	providersData, err = fetchProviders(client, baseURL)
	if err != nil {
		return ProvidersData{}, err
	}

	return providersData, nil
}

func waitForProvidersEndpoint(client *http.Client, baseURL string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := fetchProviders(client, baseURL); err == nil {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("timed out after %s", timeout)
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
		inputForm := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Search models").
					Description("Type part of provider/model (leave empty to show all models)").
					Placeholder("e.g. openrouter/glm").
					Value(&searchQuery),
			),
		)
		if err := inputForm.Run(); err != nil {
			return nil, err
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

		selectForm := huh.NewForm(
			huh.NewGroup(
				huh.NewMultiSelect[string]().
					Title("Select model(s) to save").
					Description(fmt.Sprintf("Search: %s (%d/%d shown). Saved models are pinned first. Use space to select, enter to confirm.", searchLabel, len(filtered), len(allModels))).
					Options(options...).
					Value(&selected).
					Filterable(false),
			),
		)
		if err := selectForm.Run(); err != nil {
			return nil, err
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

func createTimestampFolder(index int) string {
	now := time.Now()
	return fmt.Sprintf("evals/%d-%02d-%02d_%02d-%02d-%02d_%d",
		now.Year(), now.Month(), now.Day(),
		now.Hour(), now.Minute(), now.Second(), index)
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
		Prompt:          result.Prompt,
		Model:           model,
		Success:         result.Success,
		Error:           result.Error,
		DurationSeconds: int(result.Duration.Seconds()),
		CompletedAt:     time.Now().Format(time.RFC3339),
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
			}
		}

		folders = append(folders, ef)
	}

	return folders, nil
}

type EvalTask struct {
	Prompt string
	Folder string // empty = create new folder
}

func runAllEvalsParallel(tasks []EvalTask, model string) []EvalResult {
	var wg sync.WaitGroup
	results := make([]EvalResult, len(tasks))
	resultMutex := &sync.Mutex{}

	for i, task := range tasks {
		wg.Add(1)
		go func(index int, t EvalTask) {
			defer wg.Done()
			result := runAgentWithRetry(t.Prompt, index, model, t.Folder)
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
	currentModel := model

	for i, task := range tasks {
		results[i] = runAgentWithRetry(task.Prompt, i, currentModel, task.Folder)

		// On model-not-found, prompt user to correct and re-run this eval
		if !results[i].Success {
			isModelErr, suggestions := isModelNotFoundError(results[i].Error)
			if isModelErr {
				fmt.Printf("\n[%d] Model not found: %s\n", i, currentModel)
				corrected := promptModelCorrection(currentModel, suggestions)
				if corrected == "" {
					fmt.Println("No model selected, aborting remaining evals.")
					return results
				}
				currentModel = corrected
				fmt.Printf("[%d] Retrying with model: %s\n", i, currentModel)
				results[i] = runAgentWithRetry(task.Prompt, i, currentModel, task.Folder)
			}
		}
	}
	return results
}

func runAgentWithRetry(prompt string, index int, modelStr string, existingFolder string) EvalResult {
	maxAttempts := transientRetries + 1
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	folder := existingFolder
	var result EvalResult

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 {
			fmt.Printf("[%d] Retry attempt %d/%d after transient failure\n", index, attempt-1, transientRetries)
		}

		result = runAgent(prompt, index, modelStr, folder)
		folder = result.Folder

		if result.Success || !isTransientEvalError(result.Error) || attempt == maxAttempts {
			return result
		}
	}

	return result
}

func runAgent(prompt string, index int, modelStr string, existingFolder string) EvalResult {
	startTime := time.Now()

	folderPath := existingFolder
	if folderPath == "" {
		folderPath = createTimestampFolder(index)
	}

	fmt.Printf("[%d] Starting eval in %s\n", index, folderPath)

	result := EvalResult{
		Prompt:   prompt,
		Folder:   folderPath,
		Success:  false,
		Duration: 0,
	}

	if existingFolder == "" {
		if err := setupEvalFolder(folderPath, prompt); err != nil {
			result.Error = fmt.Sprintf("Failed to setup folder: %v", err)
			result.Duration = time.Since(startTime)
			saveEvalResult(folderPath, result, modelStr)
			return result
		}
	}

	port := basePort + index
	providerID, modelID := parseModel(modelStr)

	cmd := exec.Command("opencode", "--port", fmt.Sprintf("%d", port))
	cmd.Dir = folderPath
	if err := cmd.Start(); err != nil {
		result.Error = fmt.Sprintf("Failed to start opencode: %v", err)
		result.Duration = time.Since(startTime)
		saveEvalResult(folderPath, result, modelStr)
		return result
	}
	defer cmd.Process.Kill()

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	client := &http.Client{Timeout: 10 * time.Second}

	// Wait for server to be ready by polling session creation
	var session *Session
	var sessionErr error
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		session, sessionErr = createSession(client, baseURL, fmt.Sprintf("Eval %d", index))
		if sessionErr == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if session == nil {
		result.Error = fmt.Sprintf("Server not ready after 15s: %v", sessionErr)
		result.Duration = time.Since(startTime)
		saveEvalResult(folderPath, result, modelStr)
		return result
	}

	fmt.Printf("[%d] Session created: %s\n", index, session.ID)

	// Subscribe to SSE events BEFORE sending the prompt to avoid race condition
	eventResp, err := http.Get(baseURL + "/event")
	if err != nil {
		result.Error = fmt.Sprintf("Failed to subscribe to events: %v", err)
		result.Duration = time.Since(startTime)
		saveEvalResult(folderPath, result, modelStr)
		return result
	}
	defer eventResp.Body.Close()

	fmt.Printf("[%d] Sending prompt...\n", index)

	if err := sendPrompt(client, baseURL, session.ID, providerID, modelID, prompt); err != nil {
		result.Error = fmt.Sprintf("Failed to send prompt: %v", err)
		result.Duration = time.Since(startTime)
		saveEvalResult(folderPath, result, modelStr)
		return result
	}

	completed, errMsg := waitForCompletion(eventResp.Body, session.ID, index)

	result.Duration = time.Since(startTime)
	fmt.Printf("[%d] Completed in %ds\n", index, int(result.Duration.Seconds()))

	result.Success = completed && errMsg == ""
	if errMsg != "" {
		result.Error = errMsg
	} else if !completed {
		result.Error = "agent did not reach idle state"
	}

	saveEvalResult(folderPath, result, modelStr)
	return result
}

func createSession(client *http.Client, baseURL, title string) (*Session, error) {
	reqBody := map[string]string{"title": title}
	body, _ := json.Marshal(reqBody)

	resp, err := client.Post(baseURL+"/session", "application/json", strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 8192))
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	// Try direct format: {"id": "...", "title": "..."}
	var session Session
	if err := json.Unmarshal(respBody, &session); err != nil {
		return nil, fmt.Errorf("parsing session response: %w", err)
	}

	if session.ID != "" {
		return &session, nil
	}

	// Try data-wrapped format: {"data": {"id": "...", "title": "..."}}
	var wrapped struct {
		Data Session `json:"data"`
	}
	if err := json.Unmarshal(respBody, &wrapped); err == nil && wrapped.Data.ID != "" {
		return &wrapped.Data, nil
	}

	return nil, fmt.Errorf("empty session ID in response: %s", string(respBody))
}

func sendPrompt(client *http.Client, baseURL, sessionID, providerID, modelID, prompt string) error {
	reqBody := PromptRequest{
		Model: Model{ProviderID: providerID, ModelID: modelID},
		Parts: []PromptPart{{Type: "text", Text: prompt}},
	}
	body, _ := json.Marshal(reqBody)

	// Use prompt_async endpoint — returns 204 immediately, agent runs in background
	url := fmt.Sprintf("%s/session/%s/prompt_async", baseURL, sessionID)
	req, _ := http.NewRequest("POST", url, strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	return nil
}

func waitForCompletion(eventStream io.ReadCloser, sessionID string, index int) (bool, string) {
	completed := false
	var errorMsg string
	lastActivity := time.Now()
	stateMu := sync.Mutex{}

	done := make(chan struct{})
	var closeOnce sync.Once
	closeDone := func() { closeOnce.Do(func() { close(done) }) }

	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				stateMu.Lock()
				inactiveFor := time.Since(lastActivity)
				alreadyFailed := errorMsg != ""
				stateMu.Unlock()
				if !alreadyFailed && inactiveFor > inactivityTimeout {
					fmt.Printf("[%d] Timed out: no agent activity for %ds\n", index, int(inactivityTimeout.Seconds()))
					stateMu.Lock()
					errorMsg = fmt.Sprintf("no agent activity for %ds", int(inactivityTimeout.Seconds()))
					stateMu.Unlock()
					closeDone()
					return
				}
			}
		}
	}()

	scanner := bufio.NewScanner(eventStream)
	scanner.Buffer(make([]byte, 64*1024), eventScannerMaxTokenSize)
	for scanner.Scan() {
		select {
		case <-done:
			stateMu.Lock()
			doneCompleted := completed
			doneErr := errorMsg
			stateMu.Unlock()
			return doneCompleted, doneErr
		default:
		}

		line := scanner.Text()
		if strings.TrimSpace(line) != "" {
			stateMu.Lock()
			lastActivity = time.Now()
			stateMu.Unlock()
		}

		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")

		var event Event
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}

		// Skip server-level events (heartbeats, etc.) — don't count as activity
		if strings.HasPrefix(event.Type, "server.") {
			continue
		}

		// Filter events by session ID
		if eventSessionID, ok := event.Properties["sessionID"].(string); ok {
			if eventSessionID != sessionID {
				continue
			}
		}

		switch event.Type {
		case "session.idle":
			fmt.Printf("[%d] Session idle - agent completed\n", index)
			stateMu.Lock()
			completed = true
			stateMu.Unlock()
			closeDone()
			return true, ""

		case "session.status":
			// Newer event format: {sessionID, status: {type: "idle"|"busy"|"retry"}}
			if status, ok := event.Properties["status"].(map[string]interface{}); ok {
				if statusType, ok := status["type"].(string); ok {
					switch statusType {
					case "idle":
						fmt.Printf("[%d] Session idle - agent completed\n", index)
						stateMu.Lock()
						completed = true
						stateMu.Unlock()
						closeDone()
						return true, ""
					case "busy":
						fmt.Printf("[%d] Agent working...\n", index)
					case "retry":
						msg := ""
						if m, ok := status["message"].(string); ok {
							msg = m
						}
						fmt.Printf("[%d] Retrying: %s\n", index, msg)
					}
				}
			}

		case "session.error":
			fmt.Printf("[%d] Session error detected\n", index)
			stateMu.Lock()
			if errVal, ok := event.Properties["error"]; ok {
				errorMsg = extractErrorMessage(errVal)
			} else {
				errorMsg = "unknown session error"
			}
			stateMu.Unlock()
			closeDone()
			stateMu.Lock()
			sessionErr := errorMsg
			stateMu.Unlock()
			return false, sessionErr

		case "message.updated", "message.part.updated":
			// Agent is actively generating — don't spam the log

		default:
			fmt.Printf("[%d] Event: %s\n", index, event.Type)
		}
	}

	if err := scanner.Err(); err != nil && err != io.EOF {
		fmt.Printf("[%d] Event stream error: %v\n", index, err)
		stateMu.Lock()
		if errorMsg == "" {
			errorMsg = fmt.Sprintf("event stream error: %v", err)
		}
		stateMu.Unlock()
	}

	closeDone()
	stateMu.Lock()
	finalCompleted := completed
	finalErr := errorMsg
	stateMu.Unlock()
	return finalCompleted, finalErr
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

func extractErrorMessage(errVal interface{}) string {
	if errMap, ok := errVal.(map[string]interface{}); ok {
		// Try nested: {data: {message: "..."}}
		if data, ok := errMap["data"].(map[string]interface{}); ok {
			if msg, ok := data["message"].(string); ok {
				return msg
			}
		}
		// Try flat: {message: "..."}
		if msg, ok := errMap["message"].(string); ok {
			return msg
		}
		if name, ok := errMap["name"].(string); ok {
			return name
		}
	}
	if s, ok := errVal.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", errVal)
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

func promptModelSelector(description string) string {
	savedModels, _ := loadSavedModels()

	if len(savedModels) == 0 {
		var modelStr string
		form := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Model to use").
					Description(description).
					Placeholder("e.g. openrouter/z-ai/glm-5").
					Value(&modelStr),
			),
		)
		if err := form.Run(); err != nil {
			return ""
		}
		return strings.TrimSpace(modelStr)
	}

	// Show saved models as a filterable select with custom option
	options := make([]huh.Option[string], 0, len(savedModels)+1)
	for _, m := range savedModels {
		options = append(options, huh.NewOption(m, m))
	}
	options = append(options, huh.NewOption("Type a different model...", "__custom__"))

	var selected string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Model to use").
				Description(description).
				Options(options...).
				Value(&selected),
		),
	)
	if err := form.Run(); err != nil {
		return ""
	}

	if selected != "__custom__" {
		return selected
	}

	// Custom model input
	var modelStr string
	inputForm := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Enter model ID").
				Placeholder("e.g. openrouter/z-ai/glm-5").
				Value(&modelStr),
		),
	)
	if err := inputForm.Run(); err != nil {
		return ""
	}
	return strings.TrimSpace(modelStr)
}

func promptModelCorrection(currentModel string, suggestions []string) string {
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
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Pick the correct model").
				Description(fmt.Sprintf("'%s' was not found", currentModel)).
				Options(options...).
				Value(&selected),
		),
	)
	if err := form.Run(); err != nil {
		return ""
	}

	if selected != "__custom__" {
		return selected
	}

	var modelStr string
	inputForm := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Enter model ID").
				Placeholder("e.g. openrouter/z-ai/glm-5").
				Value(&modelStr),
		),
	)
	if err := inputForm.Run(); err != nil {
		return ""
	}
	return strings.TrimSpace(modelStr)
}
