import { z } from "zod"
import path from "node:path"

const JsonRecordSchema: z.ZodType<Record<string, unknown>> = z.record(z.string(), z.unknown())

export const OutputFormatSchema = z.union([
  z.object({
    type: z.literal("text"),
  }),
  z.object({
    type: z.literal("json_schema"),
    schema: JsonRecordSchema,
    retryCount: z.number().int().positive().optional(),
  }),
])

export const PromptDefaultsSchema = z
  .object({
    model: z.string().min(1).optional(),
    agent: z.string().min(1).optional(),
    system: z.string().optional(),
    directory: z.string().min(1).optional(),
    timeoutMs: z.number().int().positive().optional(),
    format: OutputFormatSchema.optional(),
  })
  .strict()

export const PromptCaseSchema = z
  .object({
    id: z.string().min(1),
    title: z.string().min(1),
    prompt: z.string().min(1),
    model: z.string().min(1).optional(),
    agent: z.string().min(1).optional(),
    system: z.string().optional(),
    directory: z.string().min(1).optional(),
    timeoutMs: z.number().int().positive().optional(),
    format: OutputFormatSchema.optional(),
  })
  .strict()

export const PromptSuiteSchema = z
  .object({
    version: z.literal(1),
    provider: z.literal("openrouter"),
    defaults: PromptDefaultsSchema.optional(),
    cases: z.array(PromptCaseSchema).min(1),
  })
  .strict()
  .superRefine((suite, ctx) => {
    const seen = new Set<string>()
    for (const item of suite.cases) {
      if (seen.has(item.id)) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          message: `Duplicate case id "${item.id}"`,
          path: ["cases"],
        })
      }
      seen.add(item.id)
    }
  })

export type OutputFormat = z.infer<typeof OutputFormatSchema>
export type PromptDefaults = z.infer<typeof PromptDefaultsSchema>
export type PromptCase = z.infer<typeof PromptCaseSchema>
export type PromptSuite = z.infer<typeof PromptSuiteSchema>

export type GlobalConfig = {
  openrouterApiKey?: string
  defaultAgent?: string
  defaultDirectory?: string
}

export type ResolvedPromptCase = {
  id: string
  title: string
  prompt: string
  model: string
  agent?: string
  system?: string
  directory: string
  timeoutMs: number
  format?: OutputFormat
}

const DEFAULT_TIMEOUT_MS = 300_000

export function parsePromptSuite(input: string): PromptSuite {
  return PromptSuiteSchema.parse(JSON.parse(input))
}

export function stringifyPromptSuite(input: PromptSuite): string {
  return `${JSON.stringify(input, null, 2)}\n`
}

export function resolvePromptCase(
  suite: PromptSuite,
  item: PromptCase,
  globalConfig: GlobalConfig,
  cwd = process.cwd(),
): ResolvedPromptCase {
  const defaults = suite.defaults ?? {}
  const model = item.model ?? defaults.model
  if (!model) {
    throw new Error(`Case "${item.id}" does not define a model and no suite default model exists`)
  }

  return {
    id: item.id,
    title: item.title,
    prompt: item.prompt,
    model,
    agent: item.agent ?? defaults.agent ?? globalConfig.defaultAgent,
    system: item.system ?? defaults.system,
    directory: item.directory ?? defaults.directory ?? globalConfig.defaultDirectory ?? cwd,
    timeoutMs: item.timeoutMs ?? defaults.timeoutMs ?? DEFAULT_TIMEOUT_MS,
    format: item.format ?? defaults.format,
  }
}

export function makeCaseId(title: string): string {
  const slug = title
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
  return slug || "case"
}

export function makePathSegment(value: string): string {
  const slug = value
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
  return slug || "value"
}

export function buildCaseWorkspaceDirectory(input: {
  baseDirectory: string
  caseId: string
  model: string
  cwd?: string
  workspaceBaseDirectory: string
}): string {
  const cwd = input.cwd ?? process.cwd()
  const normalizedBase = path.resolve(input.baseDirectory)
  const fallbackBase = normalizedBase === path.resolve(cwd) ? input.workspaceBaseDirectory : normalizedBase
  return path.join(
    fallbackBase,
    makePathSegment(input.caseId),
    makePathSegment(input.model),
  )
}

export function upsertSuiteCase(
  suite: PromptSuite,
  nextCase: PromptCase,
  previousId?: string,
): PromptSuite {
  const targetId = previousId ?? nextCase.id
  const cases = suite.cases.some((item) => item.id === targetId)
    ? suite.cases.map((item) => (item.id === targetId ? nextCase : item))
    : [...suite.cases, nextCase]
  return {
    ...suite,
    cases,
  }
}

export function removeSuiteCase(suite: PromptSuite, caseId: string): PromptSuite {
  return {
    ...suite,
    cases: suite.cases.filter((item) => item.id !== caseId),
  }
}

export function collectModelSuggestions(suite: PromptSuite, recentModels: ReadonlyArray<string>): Array<string> {
  const models = new Set<string>()
  for (const value of recentModels) models.add(value)
  if (suite.defaults?.model) models.add(suite.defaults.model)
  for (const item of suite.cases) {
    if (item.model) models.add(item.model)
  }
  return [...models]
}
