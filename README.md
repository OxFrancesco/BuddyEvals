# High-Evals

A CLI tool for running coding agent evaluations using opencode.

## Installation

Build the binary:

```bash
go build -o high-evals
```

## Usage

```
high-evals <command> [options]
```

## Commands

### `run`

Interactively select prompts and a model, then run evaluations.

```bash
high-evals run
```

Prompts you to:
1. Select one or more prompts (multi-select, filterable)
2. Enter a model ID (format: `provider/model`, e.g., `openrouter/glm5`)
3. Choose execution mode: parallel (all at once) or sequential

Results are saved in `evals/` folders with timestamps.

### `models`

Interactively browse and save models for reuse.

```bash
# Interactive model browser (filterable, multi-select)
high-evals models

# List all providers and models (non-interactive)
high-evals models list

# List saved models
high-evals models saved
```

When run interactively:
- Enter a search query first (blank shows all models)
- The selector shows your current search query and match count
- Use `space` to select/deselect
- Use `enter` to confirm and save

### `list`

List all prompts stored in `prompts.json`.

```bash
high-evals list
```

### `add`

Add a new prompt to `prompts.json` via an interactive form.

```bash
high-evals add
```

### `edit`

Edit an existing prompt. Shows a list to select from, then opens an editor.

```bash
high-evals edit
```

### `remove`

Remove a prompt from `prompts.json`. Shows a list to select from with confirmation.

```bash
high-evals remove
```

### `help`

Show the help message.

```bash
high-evals help
```

## Files

- `prompts.json` - Stored prompts for evaluations
- `saved-models.json` - Saved model IDs for quick reuse
- `evals/` - Output directory for evaluation runs

## Requirements

- [opencode](https://opencode.ai) must be installed and available in PATH
- Models are fetched from opencode's provider API
