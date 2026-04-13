import { mkdtemp, rm } from "node:fs/promises"
import os from "node:os"
import path from "node:path"
import { spawn } from "node:child_process"
import { pathToFileURL } from "node:url"
import { describe, expect, it } from "@effect/vitest"

describe("repository", () => {
  it("persists runs, case runs, and recent models", async () => {
    const temp = await mkdtemp(path.join(os.tmpdir(), "buddyevals-db-"))
    const dbPath = path.join(temp, "runs.sqlite")
    const repositoryUrl = pathToFileURL(path.join(process.cwd(), "src/storage/repository.ts")).href

    const script = `
      import * as Effect from "effect/Effect";
      import { Repository, buildCaseRunRecord } from ${JSON.stringify(repositoryUrl)};

      await Effect.runPromise(Repository.insertRun(${JSON.stringify(dbPath)}, {
        id: "run-1",
        suitePath: "/tmp/suite.json",
        startedAt: 1,
        completedAt: null,
        status: "running",
        totalCases: 1,
        passed: 0,
        failed: 0,
        cancelled: 0,
      }));

      await Effect.runPromise(Repository.upsertCaseRun(
        ${JSON.stringify(dbPath)},
        buildCaseRunRecord({
          id: "case-run-1",
          runId: "run-1",
          caseId: "case-1",
          title: "Case 1",
          model: "openai/gpt-4.1-mini",
          directory: process.cwd(),
          status: "passed",
          startedAt: 1,
          completedAt: 2,
          durationMs: 1,
          tokens: { input: 1, output: 2, reasoning: 0, total: 3 },
          artifactDir: "/tmp/artifacts",
        }),
      ));

      await Effect.runPromise(Repository.touchRecentModel(${JSON.stringify(dbPath)}, "openai/gpt-4.1-mini"));

      const runs = await Effect.runPromise(Repository.listRuns(${JSON.stringify(dbPath)}));
      const caseRuns = await Effect.runPromise(Repository.listCaseRuns(${JSON.stringify(dbPath)}, "run-1"));
      const models = await Effect.runPromise(Repository.listRecentModels(${JSON.stringify(dbPath)}));

      console.log(JSON.stringify({ runs, caseRuns, models }));
    `

    const [stdout, stderr, exitCode] = await new Promise<[string, string, number]>((resolve, reject) => {
      const child = spawn("bun", ["--eval", script], {
        cwd: process.cwd(),
        stdio: ["ignore", "pipe", "pipe"],
      })

      let out = ""
      let err = ""

      child.stdout.on("data", (chunk) => {
        out += chunk.toString()
      })
      child.stderr.on("data", (chunk) => {
        err += chunk.toString()
      })
      child.on("error", reject)
      child.on("close", (code) => {
        resolve([out, err, code ?? 1])
      })
    })

    try {
      expect(exitCode).toBe(0)
      if (exitCode !== 0) {
        throw new Error(stderr)
      }

      const parsed = JSON.parse(stdout.trim()) as {
        runs: Array<unknown>
        caseRuns: Array<unknown>
        models: Array<string>
      }

      expect(parsed.runs).toHaveLength(1)
      expect(parsed.caseRuns).toHaveLength(1)
      expect(parsed.models).toEqual(["openai/gpt-4.1-mini"])
    } finally {
      await rm(temp, { recursive: true, force: true })
    }
  })
})
