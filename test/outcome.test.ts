import { describe, expect, it } from "@effect/vitest"
import { deriveCaseOutcome, normalizeError } from "@/domain/outcome"

describe("outcome mapping", () => {
  it("marks passed when assistant completes without error", () => {
    const result = deriveCaseOutcome({
      lastAssistant: {
        id: "a",
        sessionID: "s",
        role: "assistant",
        time: { created: 0, completed: 1 },
        parentID: "u",
        modelID: "x",
        providerID: "openrouter",
        mode: "default",
        agent: "build",
        path: { cwd: "/", root: "/" },
        cost: 0,
        tokens: { input: 1, output: 1, reasoning: 0, total: 2, cache: { read: 0, write: 0 } },
      },
    })
    expect(result.status).toBe("passed")
  })

  it("marks failed when an assistant error exists", () => {
    const result = deriveCaseOutcome({
      lastAssistant: {
        id: "a",
        sessionID: "s",
        role: "assistant",
        time: { created: 0, completed: 1 },
        error: {
          name: "APIError",
          data: {
            message: "Upstream failed",
            isRetryable: false,
          },
        },
        parentID: "u",
        modelID: "x",
        providerID: "openrouter",
        mode: "default",
        agent: "build",
        path: { cwd: "/", root: "/" },
        cost: 0,
        tokens: { input: 1, output: 1, reasoning: 0, total: 2, cache: { read: 0, write: 0 } },
      },
    })
    expect(result.status).toBe("failed")
    expect(result.error?.message).toContain("Upstream failed")
  })

  it("normalizes native errors", () => {
    expect(normalizeError(new Error("boom"))).toEqual({
      name: "Error",
      message: "boom",
    })
  })
})
