# buddyevals

To install dependencies:

```bash
bun install
```

To start the interactive editor:

```bash
bun run buddyevals
```

Repo files:

- `buddyevals.suite.json`: prompt suite cases
- `buddyevals.settings.json`: workspace settings used by the TUI and runtime

Useful commands:

```bash
bun run buddyevals init --suite buddyevals.suite.json
bun run buddyevals tui --suite buddyevals.suite.json
bun run buddyevals run --suite buddyevals.suite.json
bun run buddyevals config show
```

Set `OPENROUTER_API_KEY` in your shell, or store it in `buddyevals.settings.json` if you explicitly want the key in the workspace file.
