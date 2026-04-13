import { describe, expect, it } from "@effect/vitest"
import * as Effect from "effect/Effect"
import * as Layer from "effect/Layer"
import { TuiIO, type TuiIO as TuiIOService } from "@/tui/io"
import { chooseModel, editCaseFlow } from "@/tui/editor"
import { parsePromptSuite } from "@/domain/suite"

function makeFakeTuiIO(answers: Array<string | boolean>) {
  const service: TuiIOService = {
    select: () => Effect.succeed(String(answers.shift())),
    text: (_message, options) => Effect.succeed(String(answers.shift() ?? options?.default ?? "")),
    confirm: () => Effect.succeed(Boolean(answers.shift())),
    info: () => Effect.void,
  }
  return Layer.succeed(TuiIO, service)
}

const suite = parsePromptSuite(
  JSON.stringify({
    version: 1,
    provider: "openrouter",
    cases: [
      {
        id: "case-1",
        title: "Case 1",
        prompt: "Hello",
        model: "openai/gpt-4.1-mini",
      },
    ],
  }),
)

describe("tui editor helpers", () => {
  it("uses a suggested model when selected", async () => {
    const model = await Effect.runPromise(
      chooseModel(["google/gemini-2.5-flash"]).pipe(
        Effect.provide(makeFakeTuiIO(["google/gemini-2.5-flash"])),
      ),
    )
    expect(model).toBe("google/gemini-2.5-flash")
  })

  it("edits and saves a case", async () => {
    const result = await Effect.runPromise(
      editCaseFlow({
        suite,
        item: suite.cases[0]!,
        recentModels: ["anthropic/claude-3.7-sonnet"],
      }).pipe(
        Effect.provide(
          makeFakeTuiIO([
            "title",
            "Renamed case",
            "save",
          ]),
        ),
      ),
    )
    expect(result.action).toBe("save")
    if (result.action !== "save") {
      throw new Error("expected save action")
    }
    expect(result.caseItem.title).toBe("Renamed case")
  })
})
