import { Command, Options } from "@effect/cli"
import * as Console from "effect/Console"
import * as Effect from "effect/Effect"
import * as Option from "effect/Option"
import { getWorkspaceConfigPath, readWorkspaceConfig, updateWorkspaceConfig } from "@/config/global-config"

function maskSecret(value: string): string {
  if (value.length <= 4) {
    return "*".repeat(value.length)
  }
  return `${"*".repeat(value.length - 4)}${value.slice(-4)}`
}

export function formatConfigForDisplay(config: {
  openrouterApiKey?: string
  defaultAgent?: string
  defaultDirectory?: string
}) {
  return {
    ...config,
    openrouterApiKey: config.openrouterApiKey ? maskSecret(config.openrouterApiKey) : undefined,
  }
}

const showConfigCommand = Command.make("show", {}, () =>
  Effect.gen(function* () {
    const config = yield* Effect.tryPromise(() => readWorkspaceConfig())
    yield* Console.log(getWorkspaceConfigPath())
    yield* Console.log(JSON.stringify(formatConfigForDisplay(config), null, 2))
  }),
).pipe(Command.withDescription("Show the current workspace BuddyEvals settings"))

const setConfigCommand = Command.make(
  "set",
  {
    openrouterApiKey: Options.text("openrouter-api-key").pipe(Options.optional),
    defaultAgent: Options.text("default-agent").pipe(Options.optional),
    defaultDirectory: Options.text("default-directory").pipe(Options.optional),
  },
  ({ openrouterApiKey, defaultAgent, defaultDirectory }) =>
    Effect.gen(function* () {
      if (
        Option.isNone(openrouterApiKey) &&
        Option.isNone(defaultAgent) &&
        Option.isNone(defaultDirectory)
      ) {
        throw new Error("Provide at least one of --openrouter-api-key, --default-agent, or --default-directory")
      }

      const partial = {
        ...(openrouterApiKey._tag === "Some" ? { openrouterApiKey: openrouterApiKey.value || undefined } : {}),
        ...(defaultAgent._tag === "Some" ? { defaultAgent: defaultAgent.value || undefined } : {}),
        ...(defaultDirectory._tag === "Some" ? { defaultDirectory: defaultDirectory.value || undefined } : {}),
      }
      const config = yield* Effect.tryPromise(() => updateWorkspaceConfig(partial))
      yield* Console.log(JSON.stringify(formatConfigForDisplay(config), null, 2))
    }),
).pipe(Command.withDescription("Update the workspace BuddyEvals settings"))

export const configCommand = Command.make("config").pipe(
  Command.withDescription("Manage the workspace BuddyEvals settings"),
  Command.withSubcommands([showConfigCommand, setConfigCommand]),
)
