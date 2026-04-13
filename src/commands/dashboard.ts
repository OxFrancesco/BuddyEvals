import { Command } from "@effect/cli"
import * as Effect from "effect/Effect"
import { ensureWorkspaceState } from "@/commands/shared"
import { runDashboard } from "@/tui/dashboard"
import { PromptTuiIOLayer } from "@/tui/io"

export const dashboardCommand = Command.make("dashboard", {}, () =>
  Effect.gen(function* () {
    const paths = yield* Effect.tryPromise(() => ensureWorkspaceState())
    yield* runDashboard(paths.dbPath).pipe(Effect.provide(PromptTuiIOLayer))
  }),
).pipe(Command.withDescription("Open the terminal dashboard for persisted BuddyEvals runs"))
