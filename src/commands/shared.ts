import { mkdir, readFile, writeFile } from "node:fs/promises"
import path from "node:path"
import { parsePromptSuite, stringifyPromptSuite, type PromptSuite } from "@/domain/suite"
import { getWorkspacePaths } from "@/config/workspace"

export function createSampleSuite(): PromptSuite {
  return {
    version: 1,
    provider: "openrouter",
    defaults: {
      model: "qwen/qwen3.6-plus-preview:free",
      timeoutMs: 120_000,
    },
    cases: [
      {
        id: "hello-world",
        title: "Hello World",
        prompt: "Reply with a short greeting and mention which model you are running on.",
      },
    ],
  }
}

export async function loadSuite(suitePath: string): Promise<PromptSuite> {
  const content = await readFile(suitePath, "utf8")
  return parsePromptSuite(content)
}

export async function saveSuite(suitePath: string, suite: PromptSuite): Promise<void> {
  await mkdir(path.dirname(suitePath), { recursive: true })
  await writeFile(suitePath, stringifyPromptSuite(suite), "utf8")
}

export async function ensureWorkspaceState(root = process.cwd()): Promise<ReturnType<typeof getWorkspacePaths>> {
  const paths = getWorkspacePaths(root)
  await mkdir(paths.dataDir, { recursive: true })
  await mkdir(paths.artifactsDir, { recursive: true })
  await mkdir(paths.workspacesDir, { recursive: true })
  return paths
}

export function resolveSuitePath(inputPath: string): string {
  return path.isAbsolute(inputPath) ? inputPath : path.join(process.cwd(), inputPath)
}
