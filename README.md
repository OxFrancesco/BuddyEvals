# High-Evals

## TL;DR

`high-evals` is a terminal app for running repeatable coding-agent evaluations on a library of prompts.

- You store reusable prompts in `prompts.json`.
- You browse/check/save model IDs from `opencode`.
- You run evals in `parallel` or `sequential` mode.
- Every run is persisted under `evals/<timestamp>_p<prompt-number>_<index>_<model>/` with:
  - `prompt.txt`
  - `result.json`
  - local `package.json` scaffold
- You can resume/re-run previous eval folders without rebuilding the prompt set from scratch.

Fast start:

```bash
go build -o high-evals
./high-evals
```

This opens the interactive home menu (`Run evals`, `Resume evals`, `Manage models`, prompt CRUD, `Exit`).

Open the local eval dashboard:

```bash
bun run evals:dashboard
```

## Technical Analysis

### 1) System Purpose

High-Evals is built to evaluate prompt quality and model behavior with an explicit artifact trail per run.
It is opinionated around:

- Prompt-driven benchmark execution.
- Model discovery through `opencode` provider metadata.
- Local, inspectable run folders for auditing and reruns.
- Failure-aware execution with inactivity timeouts and transient retries.

### 2) Runtime Architecture

The app is a single Go binary (`main.go`) with three major layers:

1. Command router (CLI entrypoint)
- Commands: `run`, `resume`, `models`, `list`, `add`, `edit`, `remove`, `help`.
- If no command is provided, an interactive menu is shown.

2. Data layer (local JSON files)
- `prompts.json`: array of prompt strings.
- `saved-models.json`: array of saved model IDs (for pinning in selection UIs).
- `evals/`: run artifacts and final status snapshots.

3. Execution engine (opencode-backed evaluator)
- Spawns one `opencode` server per active eval (`basePort + index`).
- Creates a session, posts prompt asynchronously, listens to SSE events.
- Marks success on `session.idle`/idle status events.
- Persists deterministic result metadata to disk.

### 3) Command Surface and Behavior

#### `run`

Two modes:

- Interactive:
  - multi-select prompts,
  - choose execution mode (`parallel`/`sequential`),
  - select a saved model or type custom model ID.
- Non-interactive:
  - requires `-m` and `-p` together.
  - example:
    ```bash
    ./high-evals run -m openrouter/z-ai/glm-5 -p 1,3,5 --mode parallel
    ```

Flags:

- `--mode`: `parallel` or `sequential` (default `sequential`).
- `--inactivity-timeout`: seconds of inactivity before failing an eval (default `180`).
- `--retries`: transient retry attempts per eval (default `1`).

#### `resume`

- Scans `evals/` folders.
- Displays each prior eval with status:
  - `success`,
  - `failed`,
  - `?` incomplete/no `result.json`.
- Lets you pick one or many runs to re-execute.
- Lets you override model, or keep using stored model/default fallback.
- Supports the same reliability flags:
  - `--inactivity-timeout`,
  - `--retries`.

#### `models`

- `./high-evals models`: interactive search + multi-select save flow.
- `./high-evals models list`: print provider/model tree.
- `./high-evals models check <provider/model>`: availability check + closest-match suggestions.
- `./high-evals models save <provider/model>`: save one model directly.
- `./high-evals models saved`: print saved model list.

Model IDs entered without provider are normalized to `openrouter/<model>`.

#### Prompt CRUD

- `list`: preview prompts with count.
- `add`: interactive text entry (trimmed, non-empty, max 2000 chars).
- `edit`: choose by index, rewrite prompt, save.
- `remove`: choose by index + explicit confirmation.

### 4) Execution Lifecycle (Per Eval)

For each selected eval task:

1. Resolve folder:
- new run: create timestamped+model folder.
- resume run: reuse existing folder.

2. Setup artifacts (new runs only):
- write `prompt.txt`.
- write `package.json` (`type: module`, `private: true`).

3. Start local `opencode` process on assigned port.

4. Create session via HTTP.

5. Subscribe to `/event` SSE stream first (prevents race before prompt send).

6. Send prompt to `/session/<id>/prompt_async` with model/provider payload.

7. Wait for completion:
- success on idle event.
- fail on session error event.
- fail on inactivity timeout.
- fail on stream scanner errors.

8. Persist `result.json`:
- prompt text,
- model,
- success boolean,
- error (optional),
- duration seconds,
- completion timestamp (`RFC3339`).

### 5) Reliability and Error Handling

Transient retry policy (`runAgentWithRetry`) retries only these conditions:

- inactivity timeout (`no agent activity for ...`),
- event stream failure (`event stream error: ...`),
- missing final idle state (`agent did not reach idle state`).

Notable behavior:

- Sequential mode has automatic model correction when the error indicates `Model not found`.
- Correction options prioritize:
  - server-provided suggestions,
  - saved models,
  - manual model entry.
- Parallel mode does not do interactive per-task model correction mid-flight.

### 6) Model Search/Relevance Logic

Interactive model filtering uses a weighted ranking strategy:

- direct lowercase substring matches,
- normalized alphanumeric matches (hyphen/space insensitive),
- prefix boosts,
- subsequence matching (e.g. `gm5` can match `glm-5`),
- token hit count and token-order bonuses.

Saved models are pinned to the top after filtering.

### 7) Data Contracts

#### `prompts.json`

```json
[
  "Prompt 1",
  "Prompt 2"
]
```

#### `saved-models.json`

```json
[
  "opencode/kimi-k2.5-free",
  "openrouter/z-ai/glm-5"
]
```

#### `evals/<folder>/result.json`

```json
{
  "prompt": "Create X...",
  "prompt_number": 3,
  "model": "openrouter/z-ai/glm-5",
  "success": true,
  "duration_seconds": 73,
  "completed_at": "2026-02-13T20:15:42Z"
}
```

### 8) Dependencies and Requirements

- Go `1.25.4` (per `go.mod`) to build from source.
- `opencode` binary installed and available on `PATH`.
- Local loopback HTTP access (`127.0.0.1`) for session/provider/event endpoints.

## User Flow (Features and Everything You Can Do)

### A) Home Screen Flow

Run:

```bash
./high-evals
```

You get a menu with live counters:

- number of prompts,
- number of eval folders,
- number of saved models.

Actions available from home:

- Run evals
- Resume evals
- Manage models
- List prompts
- Add prompt
- Edit prompt
- Remove prompt
- Exit

### B) Prompt Management Flow

1. Add prompts (`add`).
2. Verify prompt list (`list`).
3. Refine prompt wording (`edit`).
4. Delete stale prompts (`remove`).

What this enables:

- maintain a benchmark prompt set,
- keep prompts normalized and reusable across multiple models,
- avoid copy/paste drift between runs.

### C) Model Management Flow

1. Discover available models:
   ```bash
   ./high-evals models list
   ```
2. Search/select/pin favorites:
   ```bash
   ./high-evals models
   ```
3. Save directly by ID:
   ```bash
   ./high-evals models save openrouter/z-ai/glm-5
   ```
4. Validate a model before long runs:
   ```bash
   ./high-evals models check openrouter/z-ai/glm-5
   ```
5. Review pinned models:
   ```bash
   ./high-evals models saved
   ```

What this enables:

- faster model picking during eval runs,
- typo-resistant selection via ranking/suggestions,
- reusable model presets in team workflows.

### D) New Evaluation Flow (`run`)

1. Pick prompts.
2. Pick execution mode:
- `parallel`: faster throughput, concurrent opencode sessions.
- `sequential`: easier observation, includes model correction path.
3. Pick model (saved or custom).
4. Run and monitor console status.
5. Inspect summary and artifacts in `evals/`.

What you get after each run:

- success/failure summary across all selected prompts,
- duration per eval,
- persisted run metadata for later audit or replay.

### E) Resume/Rerun Flow (`resume`)

1. Scan previous run folders.
2. Select any mix of:
- successful runs (for reproducibility checks),
- failed runs (for debugging),
- incomplete runs (for recovery).
3. Choose mode and optional model override.
4. Re-execute selected items.

What this enables:

- incremental reruns without rebuilding prompt selections,
- quick regression checks after prompt/model updates,
- controlled retries for unstable sessions.

### F) Reliability Tuning Flow

For both `run` and `resume`, tune runtime resilience:

```bash
./high-evals run --inactivity-timeout 240 --retries 2
./high-evals resume --inactivity-timeout 300 --retries 3
```

Use cases:

- increase timeout for slower models,
- increase retries when event streams are unstable,
- reduce retries for fast fail-feedback loops.

### G) Non-Interactive Automation Flow

For scripting/CI-like usage, provide model + prompt indices:

```bash
./high-evals run -m openrouter/z-ai/glm-5 -p 1,2,4 --mode sequential
```

This bypasses prompt/model UI selection and executes directly.

### H) Operational Outputs You Can Use

Artifacts in each eval folder support:

- debugging failures from stored error metadata,
- comparing run durations across models,
- traceable run history by timestamp+model folder,
- rehydrating eval batches through `resume`.

## Quick Command Reference

```bash
./high-evals
./high-evals help
./high-evals run
./high-evals run -m openrouter/z-ai/glm-5 -p 1,3 --mode parallel
./high-evals resume
./high-evals models
./high-evals models list
./high-evals models check openrouter/glm-5
./high-evals models save openrouter/z-ai/glm-5
./high-evals models saved
./high-evals list
./high-evals add
./high-evals edit
./high-evals remove
```
