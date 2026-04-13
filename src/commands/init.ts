import { Command, Options } from "@effect/cli"
import * as Console from "effect/Console"
import * as Effect from "effect/Effect"
import { createSampleSuite, ensureWorkspaceState, resolveSuitePath, saveSuite } from "@/commands/shared"
import { createSampleSettings, getWorkspaceConfigPath, writeWorkspaceConfig } from "@/config/global-config"

const suiteOption = Options.text("suite").pipe(Options.withDefault("buddyevals.suite.json"))

export const initCommand = Command.make(
  "init",
  {
    suite: suiteOption,
  },
  ({ suite }) =>
    Effect.gen(function* () {
      const suitePath = resolveSuitePath(suite)
      const paths = yield* Effect.tryPromise(() => ensureWorkspaceState())
      yield* Effect.tryPromise(() => saveSuite(suitePath, createSampleSuite()))
      yield* Effect.tryPromise(() => writeWorkspaceConfig(createSampleSettings()))
      yield* Console.log(`Created sample suite at ${suitePath}`)
      yield* Console.log(`Created workspace settings at ${getWorkspaceConfigPath()}`)
      yield* Console.log(`Workspace data directory: ${paths.dataDir}`)
    }),
).pipe(Command.withDescription("Create a sample prompt suite and local BuddyEvals state directory"))
