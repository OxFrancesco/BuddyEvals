import { afterEach, describe, expect, it, vi } from "vitest"
import { waitForSessionResult } from "@/opencode/runner"

afterEach(() => {
  vi.useRealTimers()
})

describe("runner helpers", () => {
  it("cleans up the timeout branch after idle wins the race", async () => {
    vi.useFakeTimers()

    const result = await waitForSessionResult({
      waitForIdle: Promise.resolve(),
      timeoutMs: 5000,
    })

    expect(result).toBe("idle")
    expect(vi.getTimerCount()).toBe(0)
  })
})
