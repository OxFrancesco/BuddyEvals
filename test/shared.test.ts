import { mkdtemp, readFile, rm } from "node:fs/promises"
import os from "node:os"
import path from "node:path"
import { describe, expect, it } from "@effect/vitest"
import { saveSuite } from "@/commands/shared"
import type { PromptSuite } from "@/domain/suite"

describe("shared command helpers", () => {
  it("creates parent directories when saving a suite", async () => {
    const temp = await mkdtemp(path.join(os.tmpdir(), "buddyevals-suite-"))
    const suitePath = path.join(temp, "nested", "demo.suite.json")
    const suite: PromptSuite = {
      version: 1,
      provider: "openrouter",
      cases: [
        {
          id: "case-1",
          title: "Case 1",
          prompt: "Hello",
        },
      ],
    }

    try {
      await saveSuite(suitePath, suite)
      const content = await readFile(suitePath, "utf8")
      expect(JSON.parse(content)).toEqual(suite)
    } finally {
      await rm(temp, { recursive: true, force: true })
    }
  })
})
