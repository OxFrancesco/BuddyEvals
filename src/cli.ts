import { Command } from "@effect/cli"
import { configCommand } from "@/commands/config"
import { dashboardCommand } from "@/commands/dashboard"
import { initCommand } from "@/commands/init"
import { runCommand } from "@/commands/run"
import { tuiCommand } from "@/commands/tui"

export const rootCommand = Command.make("buddyevals").pipe(
  Command.withDescription("Run JSON prompt suites through OpenCode with operational-health tracking and a terminal dashboard"),
  Command.withSubcommands([initCommand, runCommand, tuiCommand, dashboardCommand, configCommand]),
)

export const cli = Command.run(rootCommand, {
  name: "BuddyEvals",
  version: "0.1.0",
})
