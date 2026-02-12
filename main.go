package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/huh"
)

const (
	inactivityTimeout = 60 * time.Second
	basePort          = 4096
	promptsFile       = "prompts.json"
	savedModelsFile   = "saved-models.json"
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
}

type SessionResponse struct {
	Data Session `json:"data"`
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
		showHelp()
		os.Exit(0)
	}

	command := os.Args[1]

	switch command {
	case "run":
		runCommand()
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

func showHelp() {
	fmt.Println(`High-Evals - Run coding agent evaluations

Usage:
  high-evals <command> [options]

Commands:
  run      Interactively select prompts and model, then run evals
  models   Manage provider/model discovery and saved model IDs
  list     List all prompts in prompts.json
  add      Add a new prompt to prompts.json
  edit     Edit an existing prompt
  remove   Remove a prompt from prompts.json
  help     Show this help message

Examples:
  high-evals run
  high-evals models
  high-evals models save openrouter/glm5
  high-evals models saved
  high-evals add
  high-evals list
  high-evals edit`)
}

func modelsCommand(args []string) {
	if len(args) == 0 {
		providersData, err := getProvidersData()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error fetching providers/models: %v\n", err)
			os.Exit(1)
		}
		printProviders(providersData)
		fmt.Println("\nUse 'high-evals models save <provider/model>' to store a model for reuse.")
		return
	}

	switch args[0] {
	case "save":
		saveModelsCommand(args[1:])
	case "saved":
		listSavedModelsCommand()
	default:
		fmt.Fprintf(os.Stderr, "Unknown models subcommand: %s\n", args[0])
		fmt.Fprintln(os.Stderr, "Usage: high-evals models [save <provider/model>|saved]")
		os.Exit(1)
	}
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

	var selectedIndices []int
	var modelStr string
	var runMode string

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
			huh.NewInput().
				Title("Model to use").
				Description("Format: provider/model. Run 'high-evals models' to view available IDs (e.g., openrouter/glm5).").
				Placeholder("gemini-2.5-pro").
				Value(&modelStr),
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

	if modelStr == "" {
		modelStr = "gemini-2.5-pro"
	}

	selectedPrompts := make([]string, len(selectedIndices))
	for i, idx := range selectedIndices {
		selectedPrompts[i] = prompts[idx]
	}

	fmt.Printf("\nStarting %d eval(s) with model: %s\n", len(selectedPrompts), modelStr)
	fmt.Printf("Mode: %s\n", runMode)
	fmt.Println(strings.Repeat("─", 50))

	var results []EvalResult
	if runMode == "parallel" {
		results = runAllEvalsParallel(selectedPrompts, modelStr)
	} else {
		results = runAllEvalsSequential(selectedPrompts, modelStr)
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

func printProviders(data ProvidersData) {
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

		defaultModel := data.Default[provider.ID]
		if defaultModel != "" {
			fmt.Printf("\n- %s (%d model(s), default: %s)\n", provider.ID, len(modelIDs), defaultModel)
		} else {
			fmt.Printf("\n- %s (%d model(s))\n", provider.ID, len(modelIDs))
		}

		for _, modelID := range modelIDs {
			fmt.Printf("  %s/%s\n", provider.ID, modelID)
		}
	}
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

		var selected []string
		options := make([]huh.Option[string], len(allModels))
		for i, model := range allModels {
			options[i] = huh.NewOption(model, model)
		}

		form := huh.NewForm(
			huh.NewGroup(
				huh.NewMultiSelect[string]().
					Title("Select model(s) to save").
					Options(options...).
					Value(&selected).
					Filterable(true),
			),
		)

		if err := form.Run(); err != nil {
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

func runAllEvalsParallel(prompts PromptJSON, model string) []EvalResult {
	var wg sync.WaitGroup
	results := make([]EvalResult, len(prompts))
	resultMutex := &sync.Mutex{}

	for i, prompt := range prompts {
		wg.Add(1)
		go func(index int, p string) {
			defer wg.Done()
			result := runAgent(p, index, model)
			resultMutex.Lock()
			results[index] = result
			resultMutex.Unlock()
		}(i, prompt)
	}

	wg.Wait()
	return results
}

func runAllEvalsSequential(prompts PromptJSON, model string) []EvalResult {
	results := make([]EvalResult, len(prompts))
	for i, prompt := range prompts {
		results[i] = runAgent(prompt, i, model)
	}
	return results
}

func runAgent(prompt string, index int, modelStr string) EvalResult {
	startTime := time.Now()
	folderPath := createTimestampFolder(index)

	fmt.Printf("[%d] Starting eval in %s\n", index, folderPath)

	result := EvalResult{
		Prompt:   prompt,
		Folder:   folderPath,
		Success:  false,
		Duration: 0,
	}

	if err := setupEvalFolder(folderPath, prompt); err != nil {
		result.Error = fmt.Sprintf("Failed to setup folder: %v", err)
		result.Duration = time.Since(startTime)
		return result
	}

	port := basePort + index
	providerID, modelID := parseModel(modelStr)

	cmd := exec.Command("opencode", "--port", fmt.Sprintf("%d", port))
	cmd.Dir = folderPath
	if err := cmd.Start(); err != nil {
		result.Error = fmt.Sprintf("Failed to start opencode: %v", err)
		result.Duration = time.Since(startTime)
		return result
	}
	defer cmd.Process.Kill()

	time.Sleep(2 * time.Second)

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	client := &http.Client{Timeout: 10 * time.Second}

	session, err := createSession(client, baseURL, fmt.Sprintf("Eval %d", index))
	if err != nil {
		result.Error = fmt.Sprintf("Failed to create session: %v", err)
		result.Duration = time.Since(startTime)
		return result
	}

	fmt.Printf("[%d] Session created: %s\n", index, session.ID)
	fmt.Printf("[%d] Sending prompt...\n", index)

	if err := sendPrompt(client, baseURL, session.ID, providerID, modelID, prompt); err != nil {
		result.Error = fmt.Sprintf("Failed to send prompt: %v", err)
		result.Duration = time.Since(startTime)
		return result
	}

	completed, errMsg := waitForCompletion(client, baseURL, session.ID, index)

	result.Duration = time.Since(startTime)
	fmt.Printf("[%d] Completed in %ds\n", index, int(result.Duration.Seconds()))

	result.Success = completed && errMsg == ""
	if errMsg != "" {
		result.Error = errMsg
	}

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

	var sessionResp SessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&sessionResp); err != nil {
		return nil, err
	}

	return &sessionResp.Data, nil
}

func sendPrompt(client *http.Client, baseURL, sessionID, providerID, modelID, prompt string) error {
	reqBody := PromptRequest{
		Model: Model{ProviderID: providerID, ModelID: modelID},
		Parts: []PromptPart{{Type: "text", Text: prompt}},
	}
	body, _ := json.Marshal(reqBody)

	url := fmt.Sprintf("%s/session/%s/prompt", baseURL, sessionID)
	req, _ := http.NewRequest("POST", url, strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

func waitForCompletion(client *http.Client, baseURL, sessionID string, index int) (bool, string) {
	resp, err := http.Get(baseURL + "/event")
	if err != nil {
		fmt.Printf("[%d] Error subscribing to events: %v\n", index, err)
		return false, err.Error()
	}
	defer resp.Body.Close()

	lastActivity := time.Now()
	completed := false
	var errorMsg string

	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if time.Since(lastActivity) > inactivityTimeout {
					fmt.Printf("[%d] No activity for %ds, completing...\n", index, int(inactivityTimeout.Seconds()))
					completed = true
					close(done)
					return
				}
			}
		}
	}()

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		select {
		case <-done:
			return completed, errorMsg
		default:
		}

		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		lastActivity = time.Now()
		data := strings.TrimPrefix(line, "data: ")

		var event Event
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}

		if eventSessionID, ok := event.Properties["sessionID"].(string); ok {
			if eventSessionID != sessionID {
				continue
			}
		}

		switch event.Type {
		case "session.idle":
			fmt.Printf("[%d] Session idle - agent completed\n", index)
			close(done)
			return true, ""
		case "session.error":
			fmt.Printf("[%d] Session error detected\n", index)
			if err, ok := event.Properties["error"]; ok {
				errorMsg = fmt.Sprintf("%v", err)
			}
			close(done)
			return false, errorMsg
		default:
			fmt.Printf("[%d] Event: %s\n", index, event.Type)
		}
	}

	if err := scanner.Err(); err != nil && err != io.EOF {
		fmt.Printf("[%d] Event stream error: %v\n", index, err)
	}

	close(done)
	return true, errorMsg
}
