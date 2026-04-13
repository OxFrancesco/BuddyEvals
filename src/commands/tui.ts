import { Command, Options } from "@effect/cli"
import * as Effect from "effect/Effect"
import { ensureWorkspaceState, resolveSuitePath } from "@/commands/shared"
import { Repository } from "@/storage/repository"
import { runSuiteEditor } from "@/tui/editor"
import { PromptTuiIOLayer } from "@/tui/io"

export const tuiCommand = Command.make(
  "tui",
  {
    suite: Options.text("suite"),
  },
  ({ suite }) =>
    Effect.gen(function* () {
      const suitePath = resolveSuitePath(suite)
      const paths = yield* Effect.tryPromise(() => ensureWorkspaceState())
      const recentModels = yield* Effect.tryPromise(() => Effect.runPromise(Repository.listRecentModels(paths.dbPath)))
      yield* runSuiteEditor(suitePath, recentModels).pipe(Effect.provide(PromptTuiIOLayer))
    }),
).pipe(Command.withDescription("Open the BuddyEvals terminal editor for suite cases and settings"))
