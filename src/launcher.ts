import { access } from "node:fs/promises"
import path from "node:path"
import { createSampleSuite, saveSuite } from "@/commands/shared"
import { createSampleSettings, getWorkspaceConfigPath, writeWorkspaceConfig } from "@/config/global-config"

const DEFAULT_SUITE_PATH = "buddyevals.suite.json"

export function resolveLauncherArgv(argv: ReadonlyArray<string>): Array<string> {
  if (argv.length <= 2) {
    return [...argv.slice(0, 2), "tui", "--suite", DEFAULT_SUITE_PATH]
  }
  return [...argv]
}

export async function prepareLauncher(argv: ReadonlyArray<string>): Promise<Array<string>> {
  const nextArgv = resolveLauncherArgv(argv)

  if (argv.length <= 2) {
    const suitePath = path.join(process.cwd(), DEFAULT_SUITE_PATH)
    const settingsPath = getWorkspaceConfigPath()
    try {
      await access(suitePath)
    } catch (error) {
      const value = error as NodeJS.ErrnoException
      if (value.code !== "ENOENT") {
        throw error
      }
      await saveSuite(suitePath, createSampleSuite())
    }

    try {
      await access(settingsPath)
    } catch (error) {
      const value = error as NodeJS.ErrnoException
      if (value.code !== "ENOENT") {
        throw error
      }
      await writeWorkspaceConfig(createSampleSettings())
    }
  }

  return nextArgv
}

export { DEFAULT_SUITE_PATH }
