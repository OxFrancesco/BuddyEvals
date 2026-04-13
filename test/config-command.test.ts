import { mkdtemp, mkdir, readFile, rm, writeFile } from "node:fs/promises"
import os from "node:os"
import path from "node:path"
import { spawn } from "node:child_process"
import { describe, expect, it } from "@effect/vitest"

async function runCli(args: Array<string>, home: string): Promise<{ stdout: string; stderr: string; exitCode: number }> {
  return await new Promise((resolve, reject) => {
    const child = spawn("bun", [path.join(process.cwd(), "src/main.ts"), ...args], {
      cwd: process.cwd(),
      env: {
        ...process.env,
        HOME: home,
      },
      stdio: ["ignore", "pipe", "pipe"],
    })

    let stdout = ""
    let stderr = ""

    child.stdout.on("data", (chunk) => {
      stdout += chunk.toString()
    })
    child.stderr.on("data", (chunk) => {
      stderr += chunk.toString()
    })
    child.on("error", reject)
    child.on("close", (code) => {
      resolve({
        stdout,
        stderr,
        exitCode: code ?? 1,
      })
    })
  })
}

describe("config command", () => {
  it("requires at least one flag for config set", async () => {
    const home = await mkdtemp(path.join(os.tmpdir(), "buddyevals-home-"))
    const cwd = await mkdtemp(path.join(os.tmpdir(), "buddyevals-cwd-"))

    try {
      const result = await new Promise<{ stdout: string; stderr: string; exitCode: number }>((resolve, reject) => {
        const child = spawn("bun", [path.join(process.cwd(), "src/main.ts"), "config", "set"], {
          cwd,
          env: {
            ...process.env,
            HOME: home,
          },
          stdio: ["ignore", "pipe", "pipe"],
        })

        let stdout = ""
        let stderr = ""
        child.stdout.on("data", (chunk) => {
          stdout += chunk.toString()
        })
        child.stderr.on("data", (chunk) => {
          stderr += chunk.toString()
        })
        child.on("error", reject)
        child.on("close", (code) => {
          resolve({ stdout, stderr, exitCode: code ?? 1 })
        })
      })
      expect(result.exitCode).not.toBe(0)
      expect(`${result.stdout}\n${result.stderr}`).toContain("Provide at least one")
    } finally {
      await rm(home, { recursive: true, force: true })
      await rm(cwd, { recursive: true, force: true })
    }
  })

  it("masks the API key in config output", async () => {
    const home = await mkdtemp(path.join(os.tmpdir(), "buddyevals-home-"))
    const cwd = await mkdtemp(path.join(os.tmpdir(), "buddyevals-cwd-"))
    const configPath = path.join(cwd, "buddyevals.settings.json")
    const secret = "sk-or-test-secret"

    try {
      await mkdir(cwd, { recursive: true })
      await writeFile(
        configPath,
        `${JSON.stringify({ openrouterApiKey: secret, defaultAgent: "planner" }, null, 2)}\n`,
      )

      const result = await new Promise<{ stdout: string; stderr: string; exitCode: number }>((resolve, reject) => {
        const child = spawn("bun", [path.join(process.cwd(), "src/main.ts"), "config", "show"], {
          cwd,
          env: {
            ...process.env,
            HOME: home,
          },
          stdio: ["ignore", "pipe", "pipe"],
        })

        let stdout = ""
        let stderr = ""
        child.stdout.on("data", (chunk) => {
          stdout += chunk.toString()
        })
        child.stderr.on("data", (chunk) => {
          stderr += chunk.toString()
        })
        child.on("error", reject)
        child.on("close", (code) => {
          resolve({ stdout, stderr, exitCode: code ?? 1 })
        })
      })
      expect(result.exitCode).toBe(0)
      expect(result.stdout).not.toContain(secret)
      expect(result.stdout).toContain("planner")
      expect(result.stdout).toContain("****cret")

      const fileContent = await readFile(configPath, "utf8")
      expect(fileContent).toContain(secret)
    } finally {
      await rm(home, { recursive: true, force: true })
      await rm(cwd, { recursive: true, force: true })
    }
  })
})
