import { describe, expect, it } from "@effect/vitest"
import { buildCaseWorkspaceDirectory, collectInvalidResolvedModels, collectModelSuggestions, makeCaseId, parsePromptSuite, resolvePromptCase, removeSuiteCase, upsertSuiteCase } from "@/domain/suite"

const suite = parsePromptSuite(
  JSON.stringify({
    version: 1,
    provider: "openrouter",
    defaults: {
      model: "openai/gpt-4.1-mini",
      timeoutMs: 1000,
    },
    cases: [
      {
        id: "first",
        title: "First",
        prompt: "Hello",
      },
    ],
  }),
)

describe("suite domain", () => {
  it("resolves defaults and global config", () => {
    const result = resolvePromptCase(
      suite,
      suite.cases[0]!,
      {
        defaultAgent: "planner",
        defaultDirectory: "/tmp/project",
      },
      "/tmp/fallback",
    )
    expect(result.model).toBe("openai/gpt-4.1-mini")
    expect(result.agent).toBe("planner")
    expect(result.directory).toBe("/tmp/project")
    expect(result.timeoutMs).toBe(1000)
  })

  it("collects model suggestions from suite and history", () => {
    const models = collectModelSuggestions(
      {
        ...suite,
        cases: [
          ...suite.cases,
          {
            id: "second",
            title: "Second",
            prompt: "Hi",
            model: "google/gemini-2.5-flash",
          },
        ],
      },
      ["anthropic/claude-3.7-sonnet"],
    )
    expect(models).toEqual(
      expect.arrayContaining([
        "openai/gpt-4.1-mini",
        "google/gemini-2.5-flash",
        "anthropic/claude-3.7-sonnet",
      ]),
    )
  })

  it("upserts and removes cases", () => {
    const next = upsertSuiteCase(suite, {
      id: "second",
      title: "Second",
      prompt: "Prompt",
    })
    expect(next.cases).toHaveLength(2)
    const removed = removeSuiteCase(next, "second")
    expect(removed.cases).toHaveLength(1)
  })

  it("slugifies case ids", () => {
    expect(makeCaseId("Hello, World!")).toBe("hello-world")
  })

  it("builds isolated case workspace directories under the workspace folder when base is cwd", () => {
    const result = buildCaseWorkspaceDirectory({
      baseDirectory: "/tmp/project",
      caseId: "My Case",
      model: "openai/gpt-5-mini",
      cwd: "/tmp/project",
      workspaceBaseDirectory: "/tmp/project/.buddyevals/workspaces",
    })

    expect(result).toBe("/tmp/project/.buddyevals/workspaces/my-case/openai-gpt-5-mini")
  })

  it("builds isolated case workspace directories under the configured base directory when set", () => {
    const result = buildCaseWorkspaceDirectory({
      baseDirectory: "/tmp/project/outputs",
      caseId: "My Case",
      model: "anthropic/claude-sonnet-4",
      cwd: "/tmp/project",
      workspaceBaseDirectory: "/tmp/project/.buddyevals/workspaces",
    })

    expect(result).toBe("/tmp/project/outputs/my-case/anthropic-claude-sonnet-4")
  })

  it("reports invalid resolved models with the originating suite field", () => {
    const invalid = collectInvalidResolvedModels(
      suite,
      [
        resolvePromptCase(
          suite,
          suite.cases[0]!,
          {
            defaultAgent: "planner",
            defaultDirectory: "/tmp/project",
          },
          "/tmp/fallback",
        ),
      ],
      ["openai/gpt-4.1-mini", "google/gemini-2.5-flash"],
    )

    expect(invalid).toEqual([])

    const brokenSuite = parsePromptSuite(
      JSON.stringify({
        version: 1,
        provider: "openrouter",
        defaults: {
          model: "elephant-alpha",
        },
        cases: [
          {
            id: "broken",
            title: "Broken",
            prompt: "Hello",
          },
        ],
      }),
    )

    const brokenResolved = resolvePromptCase(brokenSuite, brokenSuite.cases[0]!, {}, "/tmp/fallback")
    expect(
      collectInvalidResolvedModels(brokenSuite, [brokenResolved], ["openai/gpt-4.1-mini"]),
    ).toEqual([
      {
        model: "elephant-alpha",
        source: "buddyevals.suite.json defaults.model",
        caseIds: ["broken"],
      },
    ])
  })
})
