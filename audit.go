package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func auditCommand(args []string) {
	fs := flag.NewFlagSet("audit", flag.ExitOnError)
	writeResults := fs.Bool("write", false, "Persist validation metadata back into each result.json")
	if args != nil {
		_ = fs.Parse(args)
	}

	folders, err := scanEvalFolders()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error scanning eval folders: %v\n", err)
		os.Exit(1)
	}

	if len(folders) == 0 {
		fmt.Println("No eval folders found in evals/.")
		return
	}

	invalidCount := 0
	for _, folder := range folders {
		if folder.Result == nil {
			fmt.Printf("✗ %s — missing result.json\n", filepath.Base(folder.Path))
			invalidCount++
			continue
		}

		track := folder.Track
		if track == "" {
			track = inferPromptTrack(folder.Prompt)
		}
		report := validateEvalFolder(folder.Path, track)
		agentSuccess := folder.Result.AgentSuccess || folder.Result.Success
		success := agentSuccess && report.ValidationSuccess()

		status := "✓"
		if !success {
			status = "✗"
			invalidCount++
		}

		fmt.Printf("%s %s [%s] agent=%t validation=%t preview=%s run=%s\n",
			status,
			filepath.Base(folder.Path),
			track,
			agentSuccess,
			report.ValidationSuccess(),
			report.PreviewMode,
			report.RunMode,
		)
		if len(report.Violations) > 0 {
			fmt.Printf("  Violations: %s\n", strings.Join(report.Violations, "; "))
		}

		if *writeResults {
			updated := *folder.Result
			updated.Track = track
			updated.AgentSuccess = agentSuccess
			updated.ValidationSuccess = report.ValidationSuccess()
			updated.PreviewMode = report.PreviewMode
			updated.RunMode = report.RunMode
			updated.Violations = cloneStringSlice(report.Violations)
			updated.Checks = cloneBoolMap(report.Checks)
			updated.Success = success
			if !success && updated.Error == "" && len(report.Violations) > 0 {
				updated.Error = strings.Join(report.Violations, "; ")
			}

			if err := writeEvalResultFile(folder.Path, updated); err != nil {
				fmt.Fprintf(os.Stderr, "Error updating %s: %v\n", folder.Path, err)
				os.Exit(1)
			}
		}
	}

	fmt.Printf("\nAudit complete: %d/%d invalid\n", invalidCount, len(folders))
	if invalidCount > 0 {
		os.Exit(1)
	}
}

func writeEvalResultFile(folderPath string, result EvalResultFile) error {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(filepath.Join(folderPath, "result.json"), data, 0644)
}
