import { mkdir, readFile, writeFile } from "node:fs/promises"
import os from "node:os"
import path from "node:path"
import { z } from "zod"

const ConfigSchema = z
  .object({
    openrouterApiKey: z.string().min(1).optional(),
    defaultAgent: z.string().min(1).optional(),
    defaultDirectory: z.string().min(1).optional(),
  })
  .strict()

const WORKSPACE_CONFIG_FILE = "buddyevals.settings.json"

export type GlobalConfig = z.infer<typeof ConfigSchema>
export type WorkspaceConfig = GlobalConfig

export function createSampleSettings(): WorkspaceConfig {
  return {
    defaultAgent: "build",
    defaultDirectory: ".",
  }
}

async function readConfigFile(filePath: string): Promise<GlobalConfig> {
  try {
    const content = await readFile(filePath, "utf8")
    return ConfigSchema.parse(JSON.parse(content))
  } catch (error) {
    const value = error as NodeJS.ErrnoException
    if (value.code === "ENOENT") return {}
    throw error
  }
}

async function writeConfigFile(filePath: string, config: GlobalConfig): Promise<void> {
  const next = ConfigSchema.parse(config)
  await mkdir(path.dirname(filePath), { recursive: true })
  await writeFile(filePath, `${JSON.stringify(next, null, 2)}\n`, "utf8")
}

export function getConfigDirectory(): string {
  return path.join(os.homedir(), ".config", "buddyevals")
}

export function getGlobalConfigPath(): string {
  return path.join(getConfigDirectory(), "config.json")
}

export function getWorkspaceConfigPath(root = process.cwd()): string {
  return path.join(root, WORKSPACE_CONFIG_FILE)
}

export async function readGlobalConfig(): Promise<GlobalConfig> {
  return readConfigFile(getGlobalConfigPath())
}

export async function writeGlobalConfig(config: GlobalConfig): Promise<void> {
  await writeConfigFile(getGlobalConfigPath(), config)
}

export async function updateGlobalConfig(
  partial: Partial<GlobalConfig>,
): Promise<GlobalConfig> {
  const current = await readGlobalConfig()
  const next = ConfigSchema.parse({
    ...current,
    ...partial,
  })
  await writeGlobalConfig(next)
  return next
}

export async function readWorkspaceConfig(root = process.cwd()): Promise<WorkspaceConfig> {
  return readConfigFile(getWorkspaceConfigPath(root))
}

export async function writeWorkspaceConfig(config: WorkspaceConfig, root = process.cwd()): Promise<void> {
  await writeConfigFile(getWorkspaceConfigPath(root), config)
}

export async function updateWorkspaceConfig(
  partial: Partial<WorkspaceConfig>,
  root = process.cwd(),
): Promise<WorkspaceConfig> {
  const current = await readWorkspaceConfig(root)
  const next = ConfigSchema.parse({
    ...current,
    ...partial,
  })
  await writeWorkspaceConfig(next, root)
  return next
}

export async function readEffectiveConfig(root = process.cwd()): Promise<GlobalConfig> {
  const [globalConfig, workspaceConfig] = await Promise.all([
    readGlobalConfig(),
    readWorkspaceConfig(root),
  ])
  return {
    ...globalConfig,
    ...workspaceConfig,
  }
}

export function resolveOpenRouterApiKey(config: GlobalConfig): string | undefined {
  return config.openrouterApiKey ?? process.env.OPENROUTER_API_KEY
}
