import type { Event, Message } from "@opencode-ai/sdk/v2"

export type CaseStatus = "queued" | "running" | "passed" | "failed" | "cancelled"

export type NormalizedError = {
  name: string
  message: string
}

export type FinalAssistantMessage = Extract<Message, { role: "assistant" }>

export type CaseOutcomeInput = {
  promptError?: unknown
  sessionError?: unknown
  timedOut?: boolean
  cancelled?: boolean
  lastAssistant?: FinalAssistantMessage
}

export function normalizeError(error: unknown): NormalizedError | undefined {
  if (!error) return undefined
  if (typeof error === "string") {
    return {
      name: "Error",
      message: error,
    }
  }
  if (typeof error === "object") {
    const record = error as Record<string, unknown>
    const name = typeof record.name === "string" ? record.name : "Error"
    if (typeof record.message === "string") {
      return { name, message: record.message }
    }
    if (typeof record.data === "object" && record.data && "message" in record.data) {
      const message = (record.data as Record<string, unknown>).message
      if (typeof message === "string") {
        return { name, message }
      }
    }
  }
  if (error instanceof Error) {
    return {
      name: error.name,
      message: error.message,
    }
  }
  return {
    name: "UnknownError",
    message: String(error),
  }
}

export function toAssistantMessage(
  messages: Array<{ info: Message; parts: Array<unknown> }>,
): FinalAssistantMessage | undefined {
  for (const entry of [...messages].reverse()) {
    if (entry.info.role === "assistant") {
      return entry.info
    }
  }
  return undefined
}

export function deriveCaseOutcome(input: CaseOutcomeInput): {
  status: CaseStatus
  error?: NormalizedError
} {
  if (input.cancelled) {
    return {
      status: "cancelled",
      error: normalizeError(input.sessionError ?? input.promptError),
    }
  }

  if (input.timedOut) {
    return {
      status: "failed",
      error: {
        name: "TimeoutError",
        message: "Case timed out before the session returned to idle",
      },
    }
  }

  const promptError = normalizeError(input.promptError)
  if (promptError) {
    return {
      status: "failed",
      error: promptError,
    }
  }

  const sessionError = normalizeError(input.sessionError ?? input.lastAssistant?.error)
  if (sessionError) {
    return {
      status: "failed",
      error: sessionError,
    }
  }

  if (!input.lastAssistant) {
    return {
      status: "failed",
      error: {
        name: "MissingAssistantReply",
        message: "OpenCode returned to idle without producing an assistant message",
      },
    }
  }

  return {
    status: "passed",
  }
}

export function eventSessionId(event: Event): string | undefined {
  const properties = event.properties as Record<string, unknown>
  const sessionId = properties.sessionID
  return typeof sessionId === "string" ? sessionId : undefined
}
