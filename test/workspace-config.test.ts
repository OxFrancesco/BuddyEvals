import { mkdtemp, mkdir, rm, writeFile } from "node:fs/promises"
import os from "node:os"
import path from "node:path"
import { describe, expect, it } from "@effect/vitest"
import { createSampleSettings, readEffectiveConfig, readWorkspaceConfig } from "@/config/global-config"

describe("workspace settings", () => {
  it("provides a stable sample settings template", () => {
    expect(createSampleSettings()).toEqual({
      defaultAgent: "build",
      defaultDirectory: ".",
    })
  })

  it("merges workspace settings over the legacy home config", async () => {
    const tempHome = await mkdtemp(path.join(os.tmpdir(), "buddyevals-home-"))
    const tempCwd = await mkdtemp(path.join(os.tmpdir(), "buddyevals-cwd-"))
    const previousHome = process.env.HOME

    try {
      process.env.HOME = tempHome
      await mkdir(path.join(tempHome, ".config", "buddyevals"), { recursive: true })
      await writeFile(
        path.join(tempHome, ".config", "buddyevals", "config.json"),
        `${JSON.stringify({ defaultAgent: "planner", openrouterApiKey: "global-key" }, null, 2)}\n`,
      )
      await writeFile(
        path.join(tempCwd, "buddyevals.settings.json"),
        `${JSON.stringify({ defaultAgent: "build", defaultDirectory: "." }, null, 2)}\n`,
      )

      expect(await readWorkspaceConfig(tempCwd)).toEqual({
        defaultAgent: "build",
        defaultDirectory: ".",
      })
      expect(await readEffectiveConfig(tempCwd)).toEqual({
        defaultAgent: "build",
        defaultDirectory: ".",
        openrouterApiKey: "global-key",
      })
    } finally {
      process.env.HOME = previousHome
      await rm(tempHome, { recursive: true, force: true })
      await rm(tempCwd, { recursive: true, force: true })
    }
  })
})
