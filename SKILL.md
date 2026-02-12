---
name: high-evals
description: Manage and run coding-agent evaluations with the High-Evals CLI. Use when working in this repository to maintain prompts in prompts.json (list, add, edit, remove), choose models, and execute selected prompts in parallel or sequential mode.
---

# High-Evals

Run and maintain coding-agent evaluations for this repository.

## Run Evaluations

1. Run `high-evals run`.
2. Select one or more prompts from `prompts.json`.
3. Enter a model string.
4. Choose `parallel` or `sequential`.
5. Review terminal output and generated artifacts under `evals/`.

## Manage Prompts

- Run `high-evals list` to view prompts with indices.
- Run `high-evals add` to append a prompt.
- Run `high-evals edit` to modify an existing prompt.
- Run `high-evals remove` to delete a prompt.
- Run `high-evals help` to show command usage.

## Model Input

- Use `provider/model` to specify provider explicitly, for example `anthropic/claude-3.5-sonnet`.
- Use a plain model name to default to the `openrouter` provider, for example `gemini-2.5-pro`.
- Run `high-evals models` to query available providers and models from opencode.
- Run `high-evals models save <provider/model>` to validate and save model IDs for reuse.
- Run `high-evals models save` to interactively select models to save.
- Run `high-evals models saved` to list saved model IDs from `saved-models.json`.

## Files and Output

- Keep prompts in `prompts.json` as a JSON array of strings.
- Keep reusable model IDs in `saved-models.json` as a JSON array of strings.
- Expect each evaluation to create a timestamped folder in `evals/`.
- Inspect `prompt.txt` and generated files in each run folder.

## Completion and Failures

- Treat a run as complete when `session.idle` is received or when inactivity reaches 60 seconds.
- Check the final summary for success and failure counts.
- Use non-zero exit codes to detect failures in automation scripts.

## Prerequisites

- Install Go 1.21 or later.
- Install `opencode` and ensure it is available on `PATH`.
- Build the CLI with `go build -o high-evals .` when needed.
