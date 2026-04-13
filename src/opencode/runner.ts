import { createOpencodeServer } from "@opencode-ai/sdk/v2"
import { createOpencodeClient, type Event, type GlobalEvent, type Message, type OpencodeClient, type OutputFormat } from "@opencode-ai/sdk/v2/client"
import { Effect } from "effect"
import type { GlobalConfig } from "@/config/global-config"
import { deriveCaseOutcome, eventSessionId, normalizeError, toAssistantMessage, type CaseStatus, type NormalizedError } from "@/domain/outcome"
import type { ResolvedPromptCase } from "@/domain/suite"

export type RunnerClient = Pick<OpencodeClient, "global" | "session">

export type CaseRunResult = {
  sessionId?: string
  status: CaseStatus
  startedAt: number
  completedAt: number
  durationMs: number
  error?: NormalizedError
  events: Array<Event>
  messages: Array<{ info: Message; parts: Array<unknown> }>
  tokens?: {
    input?: number
    output?: number
    reasoning?: number
    total?: number
  }
  cost?: number
}

export type CaseRunProgress =
  | {
      type: "session-created"
      sessionId: string
    }
  | {
      type: "event"
      sessionId: string
      eventType: Event["type"]
      eventCount: number
      elapsedMs: number
      detail?: string
    }
  | {
      type: "heartbeat"
      sessionId: string
      eventCount: number
      elapsedMs: number
      lastEventType?: Event["type"]
      status?: string
    }
  | {
      type: "wait-finished"
      sessionId: string
      result: "idle" | "timeout" | "cancelled"
      eventCount: number
      elapsedMs: number
      lastEventType?: Event["type"]
      status?: string
    }

export function buildOpenCodeConfig(apiKey: string, globalConfig: GlobalConfig) {
  return {
    enabled_providers: ["openrouter"],
    default_agent: globalConfig.defaultAgent,
    provider: {
      openrouter: {
        options: {
          apiKey,
        },
      },
    },
  }
}

export async function withOpenCodeRunner<A>(
  input: {
    apiKey: string
    config: GlobalConfig
  },
  fn: (client: RunnerClient) => Promise<A>,
): Promise<A> {
  const server = await createOpencodeServer({
    port: 0,
    config: buildOpenCodeConfig(input.apiKey, input.config),
  })
  const client = createOpencodeClient({
    baseUrl: server.url,
    throwOnError: true,
  })

  try {
    return await fn(client)
  } finally {
    server.close()
  }
}

function belongsToSession(event: Event, sessionID: string): boolean {
  return eventSessionId(event) === sessionID
}

function sessionStatusLabel(event: Event | undefined): string | undefined {
  if (!event || event.type !== "session.status") return undefined
  const status = event.properties.status
  if (status.type === "retry") {
    return `retry:${status.attempt}`
  }
  return status.type
}

function eventDetail(event: Event): string | undefined {
  if (event.type === "session.status") {
    const status = event.properties.status
    if (status.type === "retry") {
      return `retry attempt ${status.attempt}: ${status.message}`
    }
    return undefined
  }

  if (event.type === "session.error") {
    const error = normalizeError(event.properties.error)
    return error ? `${error.name}: ${error.message}` : "session error"
  }

  if (event.type === "permission.asked") {
    const toolCall = event.properties.tool ? ` tool-call=${event.properties.tool.callID}` : ""
    const patterns = event.properties.patterns.length > 0 ? ` patterns=${event.properties.patterns.join(",")}` : ""
    return `permission=${event.properties.permission}${patterns}${toolCall}`
  }

  if (event.type === "question.asked") {
    const firstQuestion = event.properties.questions[0]
    if (!firstQuestion) return "question asked"
    return firstQuestion.header ? `${firstQuestion.header}: ${firstQuestion.question}` : firstQuestion.question
  }

  return undefined
}

function startSessionObserver(
  client: RunnerClient,
  sessionID: string,
  startedAt: number,
  onProgress?: (progress: CaseRunProgress) => void,
) {
  const controller = new AbortController()
  const events: Array<Event> = []
  let sessionError: unknown
  let lastEvent: Event | undefined

  const idle = (async () => {
    try {
      const source = await client.global.event({
        signal: controller.signal,
      })

      for await (const item of source.stream) {
        const event = (item as GlobalEvent).payload
        if (!belongsToSession(event, sessionID)) continue
        events.push(event)
        lastEvent = event
        if (event.type === "session.error") {
          sessionError = event.properties.error
        }
        const detail = eventDetail(event)
        if (detail) {
          onProgress?.({
            type: "event",
            sessionId: sessionID,
            eventType: event.type,
            eventCount: events.length,
            elapsedMs: Date.now() - startedAt,
            detail,
          })
        }
        if (event.type === "session.status" && event.properties.status.type === "idle") {
          return
        }
      }
    } catch (error) {
      if (!controller.signal.aborted) {
        throw error
      }
    }
  })()

  return {
    events,
    waitForIdle: () => idle,
    stop() {
      controller.abort()
    },
    getSessionError() {
      return sessionError
    },
    snapshot() {
      return {
        eventCount: events.length,
        lastEventType: lastEvent?.type,
        status: sessionStatusLabel(lastEvent),
      }
    },
  }
}

async function fetchMessages(client: RunnerClient, sessionID: string, directory: string) {
  const result = await client.session.messages(
    {
      sessionID,
      directory,
    },
    {
      throwOnError: true,
    },
  )
  return result.data ?? []
}

function extractPromptFormat(format?: OutputFormat): OutputFormat | undefined {
  return format
}

export async function waitForSessionResult(input: {
  waitForIdle: Promise<void>
  timeoutMs: number
  signal?: AbortSignal
}): Promise<"idle" | "timeout" | "cancelled"> {
  return await new Promise((resolve, reject) => {
    let settled = false
    const cleanups: Array<() => void> = []

    const complete = (result: "idle" | "timeout" | "cancelled") => {
      if (settled) return
      settled = true
      for (const cleanup of cleanups) cleanup()
      resolve(result)
    }

    const fail = (error: unknown) => {
      if (settled) return
      settled = true
      for (const cleanup of cleanups) cleanup()
      reject(error)
    }

    input.waitForIdle.then(
      () => complete("idle"),
      (error) => fail(error),
    )

    const timeout = setTimeout(() => {
      complete("timeout")
    }, input.timeoutMs)
    timeout.unref?.()
    cleanups.push(() => clearTimeout(timeout))

    if (!input.signal) {
      return
    }

    if (input.signal.aborted) {
      complete("cancelled")
      return
    }

    const onAbort = () => {
      complete("cancelled")
    }
    input.signal.addEventListener("abort", onAbort, { once: true })
    cleanups.push(() => input.signal?.removeEventListener("abort", onAbort))
  })
}

export async function runPromptCase(
  client: RunnerClient,
  resolvedCase: ResolvedPromptCase,
  signal?: AbortSignal,
  onProgress?: (progress: CaseRunProgress) => void,
): Promise<CaseRunResult> {
  const startedAt = Date.now()
  const session = await client.session.create(
    {
      directory: resolvedCase.directory,
      title: resolvedCase.title,
    },
    {
      throwOnError: true,
      signal,
    },
  )

  const sessionId = session.data?.id
  if (!sessionId) {
    throw new Error(`Failed to create OpenCode session for case "${resolvedCase.id}"`)
  }

  onProgress?.({
    type: "session-created",
    sessionId,
  })

  const observer = startSessionObserver(client, sessionId, startedAt, onProgress)
  let promptError: unknown
  let timedOut = false
  let cancelled = false
  const heartbeat = setInterval(() => {
    const snapshot = observer.snapshot()
    onProgress?.({
      type: "heartbeat",
      sessionId,
      eventCount: snapshot.eventCount,
      elapsedMs: Date.now() - startedAt,
      lastEventType: snapshot.lastEventType,
      status: snapshot.status,
    })
  }, 10_000)
  heartbeat.unref?.()

  try {
    await client.session.promptAsync(
      {
        sessionID: sessionId,
        directory: resolvedCase.directory,
        model: {
          providerID: "openrouter",
          modelID: resolvedCase.model,
        },
        agent: resolvedCase.agent,
        system: resolvedCase.system,
        format: extractPromptFormat(resolvedCase.format as OutputFormat | undefined),
        parts: [
          {
            type: "text",
            text: resolvedCase.prompt,
          },
        ],
      },
      {
        throwOnError: true,
        signal,
      },
    )

    const result = await waitForSessionResult({
      waitForIdle: observer.waitForIdle(),
      timeoutMs: resolvedCase.timeoutMs,
      signal,
    })

    const snapshot = observer.snapshot()
    onProgress?.({
      type: "wait-finished",
      sessionId,
      result,
      eventCount: snapshot.eventCount,
      elapsedMs: Date.now() - startedAt,
      lastEventType: snapshot.lastEventType,
      status: snapshot.status,
    })

    if (result === "timeout") {
      timedOut = true
      await client.session.abort(
        {
          sessionID: sessionId,
          directory: resolvedCase.directory,
        },
        { throwOnError: false },
      )
    }

    if (result === "cancelled") {
      cancelled = true
      await client.session.abort(
        {
          sessionID: sessionId,
          directory: resolvedCase.directory,
        },
        { throwOnError: false },
      )
    }
  } catch (error) {
    promptError = error
  } finally {
    clearInterval(heartbeat)
    observer.stop()
    await observer.waitForIdle().catch(() => undefined)
  }

  const messages = await fetchMessages(client, sessionId, resolvedCase.directory).catch(() => [])
  const lastAssistant = toAssistantMessage(messages)
  const outcome = deriveCaseOutcome({
    promptError,
    sessionError: observer.getSessionError(),
    timedOut,
    cancelled,
    lastAssistant,
  })

  return {
    sessionId,
    status: outcome.status,
    startedAt,
    completedAt: Date.now(),
    durationMs: Date.now() - startedAt,
    error: outcome.error ?? normalizeError(promptError) ?? normalizeError(observer.getSessionError()),
    events: observer.events,
    messages,
    tokens: lastAssistant
      ? {
          input: lastAssistant.tokens.input,
          output: lastAssistant.tokens.output,
          reasoning: lastAssistant.tokens.reasoning,
          total: lastAssistant.tokens.total,
        }
      : undefined,
    cost: lastAssistant?.cost,
  }
}

export const OpenCodeRunner = {
  withRunner: (
    input: {
      apiKey: string
      config: GlobalConfig
    },
    fn: (client: RunnerClient) => Promise<unknown>,
  ) => Effect.tryPromise(() => withOpenCodeRunner(input, fn)),
}
