import { mkdtemp, readFile, rm } from "node:fs/promises"
import os from "node:os"
import path from "node:path"
import { describe, expect, it } from "@effect/vitest"
import { prepareLauncher, resolveLauncherArgv } from "@/launcher"

describe("launcher", () => {
  it("defaults to the TUI and conventional suite path when no args are provided", () => {
    expect(resolveLauncherArgv(["bun", "index.ts"])).toEqual([
      "bun",
      "index.ts",
      "tui",
      "--suite",
      "buddyevals.suite.json",
    ])
  })

  it("preserves explicit CLI arguments", () => {
    expect(resolveLauncherArgv(["bun", "index.ts", "init", "--suite", "demo.json"])).toEqual([
      "bun",
      "index.ts",
      "init",
      "--suite",
      "demo.json",
    ])
  })

  it("creates the default suite before launching the TUI", async () => {
    const cwd = process.cwd()
    const temp = await mkdtemp(path.join(os.tmpdir(), "buddyevals-launcher-"))

    try {
      process.chdir(temp)
      const argv = await prepareLauncher(["bun", "index.ts"])
      const suitePath = path.join(temp, "buddyevals.suite.json")
      const settingsPath = path.join(temp, "buddyevals.settings.json")
      const content = JSON.parse(await readFile(suitePath, "utf8")) as {
        cases: Array<{ id: string }>
        defaults?: { model?: string }
      }
      const settings = JSON.parse(await readFile(settingsPath, "utf8")) as {
        defaultAgent?: string
        defaultDirectory?: string
      }

      expect(argv).toEqual(["bun", "index.ts", "tui", "--suite", "buddyevals.suite.json"])
      expect(content.defaults?.model).toBe("qwen/qwen3.6-plus-preview:free")
      expect(content.cases[0]?.id).toBe("hello-world")
      expect(settings.defaultAgent).toBe("build")
      expect(settings.defaultDirectory).toBe(".")
    } finally {
      process.chdir(cwd)
      await rm(temp, { recursive: true, force: true })
    }
  })
})
