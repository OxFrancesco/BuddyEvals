import * as Effect from "effect/Effect"
import { collectModelSuggestions, makeCaseId, removeSuiteCase, resolvePromptCase, stringifyPromptSuite, upsertSuiteCase, type PromptCase, type PromptSuite } from "@/domain/suite"
import { readWorkspaceConfig, writeWorkspaceConfig, type GlobalConfig } from "@/config/global-config"
import { loadSuite } from "@/commands/shared"
import { TuiIO } from "@/tui/io"
import { writeFile } from "node:fs/promises"

function parseOptionalNumber(input: string): number | undefined {
  const trimmed = input.trim()
  if (!trimmed) return undefined
  const value = Number(trimmed)
  if (!Number.isFinite(value) || value <= 0) {
    throw new Error(`Expected a positive integer timeout, received "${input}"`)
  }
  return Math.floor(value)
}

function promptCaseTemplate(): PromptCase {
  return {
    id: "new-case",
    title: "New case",
    prompt: "Describe what this case should ask the model to do.",
  }
}

export const chooseModel = (suggestions: ReadonlyArray<string>, current?: string) =>
  Effect.gen(function* () {
    const io = yield* TuiIO
    const unique = [...new Set(suggestions.filter(Boolean))]
    const choices = [
      ...(current
        ? [
            {
              title: `Keep current (${current})`,
              value: "__keep__",
            },
          ]
        : []),
      ...unique.slice(0, 10).map((item) => ({
        title: item,
        value: item,
      })),
      {
        title: "Custom value",
        value: "__custom__",
      },
      {
        title: "Clear model",
        value: "__clear__",
      },
    ]

    const selection = yield* io.select("Choose the model for this case", choices)
    if (selection === "__keep__") return current
    if (selection === "__clear__") return undefined
    if (selection === "__custom__") {
      const value = yield* io.text("Enter the OpenRouter model id", {
        default: current,
      })
      return value.trim() || undefined
    }
    return selection
  })

export const editCaseFlow = (input: {
  suite: PromptSuite
  item: PromptCase
  recentModels: ReadonlyArray<string>
}) =>
  Effect.gen(function* () {
    const io = yield* TuiIO

    let draft: PromptCase = { ...input.item }
    while (true) {
      const choice = yield* io.select(`Edit case "${draft.title}"`, [
        { title: `Title: ${draft.title}`, value: "title" },
        { title: `ID: ${draft.id}`, value: "id" },
        { title: `Prompt`, value: "prompt" },
        { title: `Model: ${draft.model ?? input.suite.defaults?.model ?? "<suite default>"}`, value: "model" },
        { title: `Agent: ${draft.agent ?? input.suite.defaults?.agent ?? "<default>"}`, value: "agent" },
        { title: `System: ${draft.system ? "set" : "unset"}`, value: "system" },
        { title: `Directory: ${draft.directory ?? input.suite.defaults?.directory ?? "<default>"}`, value: "directory" },
        {
          title: `Timeout: ${String(draft.timeoutMs ?? input.suite.defaults?.timeoutMs ?? 300000)}ms`,
          value: "timeout",
        },
        { title: "Save case", value: "save" },
        { title: "Delete case", value: "delete" },
        { title: "Back", value: "back" },
      ])

      if (choice === "title") {
        const title = yield* io.text("Case title", { default: draft.title })
        draft = {
          ...draft,
          title,
          id: draft.id === input.item.id ? makeCaseId(title) : draft.id,
        }
      } else if (choice === "id") {
        draft = {
          ...draft,
          id: yield* io.text("Case id", { default: draft.id }),
        }
      } else if (choice === "prompt") {
        draft = {
          ...draft,
          prompt: yield* io.text("Prompt text", { default: draft.prompt }),
        }
      } else if (choice === "model") {
        draft = {
          ...draft,
          model: yield* chooseModel(collectModelSuggestions(input.suite, input.recentModels), draft.model),
        }
      } else if (choice === "agent") {
        const value = yield* io.text("Agent (blank to clear)", { default: draft.agent })
        draft = {
          ...draft,
          agent: value.trim() || undefined,
        }
      } else if (choice === "system") {
        const value = yield* io.text("System prompt (blank to clear)", { default: draft.system })
        draft = {
          ...draft,
          system: value.trim() || undefined,
        }
      } else if (choice === "directory") {
        const value = yield* io.text("Working directory (blank to clear)", { default: draft.directory })
        draft = {
          ...draft,
          directory: value.trim() || undefined,
        }
      } else if (choice === "timeout") {
        const value = yield* io.text("Timeout in milliseconds (blank to clear)", {
          default: draft.timeoutMs?.toString(),
        })
        draft = {
          ...draft,
          timeoutMs: parseOptionalNumber(value),
        }
      } else if (choice === "save") {
        return {
          action: "save" as const,
          caseItem: draft,
        }
      } else if (choice === "delete") {
        const confirmed = yield* io.confirm(`Delete case "${draft.title}"?`, false)
        if (confirmed) {
          return {
            action: "delete" as const,
            caseId: draft.id,
          }
        }
      } else {
        return {
          action: "back" as const,
        }
      }
    }
  })

export const editSettingsFlow = (config: GlobalConfig) =>
  Effect.gen(function* () {
    const io = yield* TuiIO
    let draft = { ...config }

    while (true) {
      const choice = yield* io.select("Edit BuddyEvals settings", [
        {
          title: `OpenRouter API key: ${draft.openrouterApiKey ? "configured" : "unset"}`,
          value: "key",
        },
        {
          title: `Default agent: ${draft.defaultAgent ?? "<unset>"}`,
          value: "agent",
        },
        {
          title: `Default directory: ${draft.defaultDirectory ?? "<unset>"}`,
          value: "directory",
        },
        {
          title: "Save settings",
          value: "save",
        },
        {
          title: "Back",
          value: "back",
        },
      ])

      if (choice === "key") {
        const value = yield* io.text("OpenRouter API key (blank to clear)", {
          default: draft.openrouterApiKey,
        })
        draft.openrouterApiKey = value.trim() || undefined
      } else if (choice === "agent") {
        const value = yield* io.text("Default agent (blank to clear)", {
          default: draft.defaultAgent,
        })
        draft.defaultAgent = value.trim() || undefined
      } else if (choice === "directory") {
        const value = yield* io.text("Default directory (blank to clear)", {
          default: draft.defaultDirectory,
        })
        draft.defaultDirectory = value.trim() || undefined
      } else if (choice === "save") {
        return draft
      } else {
        return undefined
      }
    }
  })

export const runSuiteEditor = (suitePath: string, recentModels: ReadonlyArray<string>) =>
  Effect.gen(function* () {
    const io = yield* TuiIO
    let suite = yield* Effect.tryPromise(() => loadSuite(suitePath))
    let config = yield* Effect.tryPromise(() => readWorkspaceConfig())
    let dirtySuite = false

    while (true) {
      const choice = yield* io.select("BuddyEvals TUI", [
        { title: "Edit suite cases", value: "cases" },
        { title: "Edit settings", value: "settings" },
        { title: "Save and exit", value: "save" },
        { title: "Exit without saving", value: "exit" },
      ])

      if (choice === "cases") {
        const caseChoice = yield* io.select("Select a case", [
          ...suite.cases.map((item) => ({
            title: `${item.title} (${item.id})`,
            value: item.id,
            description: item.model ?? suite.defaults?.model,
          })),
          { title: "Add case", value: "__add__" },
          { title: "Back", value: "__back__" },
        ])

        if (caseChoice === "__add__") {
          const result = yield* editCaseFlow({
            suite,
            item: promptCaseTemplate(),
            recentModels,
          })
          if (result.action === "save") {
            suite = upsertSuiteCase(suite, result.caseItem)
            dirtySuite = true
          }
        } else if (caseChoice !== "__back__") {
          const item = suite.cases.find((entry) => entry.id === caseChoice)
          if (!item) continue
          const result = yield* editCaseFlow({
            suite,
            item,
            recentModels,
          })
          if (result.action === "save") {
            suite = upsertSuiteCase(suite, result.caseItem, item.id)
            dirtySuite = true
          } else if (result.action === "delete" && suite.cases.length > 1) {
            suite = removeSuiteCase(suite, result.caseId)
            dirtySuite = true
          }
        }
      } else if (choice === "settings") {
        const next = yield* editSettingsFlow(config)
        if (next) {
          config = next
          yield* Effect.tryPromise(() => writeWorkspaceConfig(next))
        }
      } else if (choice === "save") {
        if (dirtySuite) {
          yield* Effect.tryPromise(() => writeFile(suitePath, stringifyPromptSuite(suite), "utf8"))
        }
        yield* io.info(`Saved ${suitePath}`)
        return suite
      } else {
        return suite
      }
    }
  })

export function previewResolvedCases(suite: PromptSuite, config: GlobalConfig, cwd = process.cwd()) {
  return suite.cases.map((item) => resolvePromptCase(suite, item, config, cwd))
}
