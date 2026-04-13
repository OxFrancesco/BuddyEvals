import { mkdir, readFile, writeFile } from "node:fs/promises"
import path from "node:path"
import type { Event, Message } from "@opencode-ai/sdk/v2"

export async function ensureDirectory(directory: string): Promise<void> {
  await mkdir(directory, { recursive: true })
}

export async function writeJsonFile(filePath: string, value: unknown): Promise<void> {
  await ensureDirectory(path.dirname(filePath))
  await writeFile(filePath, `${JSON.stringify(value, null, 2)}\n`, "utf8")
}

export async function writeJsonLinesFile(filePath: string, rows: ReadonlyArray<unknown>): Promise<void> {
  await ensureDirectory(path.dirname(filePath))
  const payload = rows.map((item) => JSON.stringify(item)).join("\n")
  await writeFile(filePath, payload.length > 0 ? `${payload}\n` : "", "utf8")
}

export async function writeCaseArtifacts(input: {
  artifactDir: string
  resolvedCase: unknown
  events: ReadonlyArray<Event>
  messages: Array<{ info: Message; parts: Array<unknown> }>
  summary: unknown
}): Promise<void> {
  await ensureDirectory(input.artifactDir)
  await Promise.all([
    writeJsonFile(path.join(input.artifactDir, "input.json"), input.resolvedCase),
    writeJsonLinesFile(path.join(input.artifactDir, "events.jsonl"), input.events),
    writeJsonFile(path.join(input.artifactDir, "messages.json"), input.messages),
    writeJsonFile(path.join(input.artifactDir, "summary.json"), input.summary),
  ])
}

export async function readSummaryArtifact<T>(artifactDir: string): Promise<T | undefined> {
  try {
    const content = await readFile(path.join(artifactDir, "summary.json"), "utf8")
    return JSON.parse(content) as T
  } catch (error) {
    const value = error as NodeJS.ErrnoException
    if (value.code === "ENOENT") return undefined
    throw error
  }
}
