import { Command, Options } from "@effect/cli"
import * as Console from "effect/Console"
import * as Effect from "effect/Effect"
import * as Option from "effect/Option"
import { mkdir } from "node:fs/promises"
import path from "node:path"
import { ensureWorkspaceState, loadSuite, resolveSuitePath } from "@/commands/shared"
import { readEffectiveConfig, resolveOpenRouterApiKey } from "@/config/global-config"
import type { CaseStatus } from "@/domain/outcome"
import { buildCaseWorkspaceDirectory, collectInvalidResolvedModels, resolvePromptCase, type PromptSuite, type ResolvedPromptCase } from "@/domain/suite"
import { OpenCodeRunner, runPromptCase, type CaseRunProgress, type RunnerClient } from "@/opencode/runner"
import { writeCaseArtifacts } from "@/storage/artifacts"
import { Repository, buildCaseRunRecord } from "@/storage/repository"
import { runDashboard } from "@/tui/dashboard"
import { PromptTuiIOLayer } from "@/tui/io"

const suiteOption = Options.text("suite")
const caseOption = Options.text("case").pipe(Options.optional)
const concurrencyOption = Options.integer("concurrency").pipe(Options.withDefault(1))
const dashboardOption = Options.boolean("dashboard")

function aggregateRunStatus(counts: { passed: number; failed: number; cancelled: number }): CaseStatus {
  if (counts.failed > 0) return "failed"
  if (counts.cancelled > 0) return "cancelled"
  return "passed"
}

function formatDuration(durationMs: number): string {
  if (durationMs < 1000) return `${durationMs}ms`
  const seconds = Math.floor(durationMs / 1000)
  const minutes = Math.floor(seconds / 60)
  if (minutes === 0) return `${seconds}s`
  return `${minutes}m ${seconds % 60}s`
}

function toWorkspacePath(value: string): string {
  const relative = path.relative(process.cwd(), value)
  return relative.length > 0 ? relative : "."
}

function formatProgress(caseId: string, progress: CaseRunProgress): string | undefined {
  if (progress.type === "session-created") {
    return `[session] ${caseId} · session=${progress.sessionId}`
  }

  if (progress.type === "event") {
    return `[event] ${caseId} · ${progress.eventType} · ${progress.detail}`
  }

  if (progress.type === "heartbeat") {
    const status = progress.status ? ` · status=${progress.status}` : ""
    const lastEvent = progress.lastEventType ? ` · last=${progress.lastEventType}` : ""
    return `[progress] ${caseId} · elapsed=${formatDuration(progress.elapsedMs)} · events=${progress.eventCount}${status}${lastEvent}`
  }

  if (progress.type === "wait-finished" && progress.result !== "idle") {
    const status = progress.status ? ` · status=${progress.status}` : ""
    const lastEvent = progress.lastEventType ? ` · last=${progress.lastEventType}` : ""
    return `[wait] ${caseId} · result=${progress.result} · elapsed=${formatDuration(progress.elapsedMs)} · events=${progress.eventCount}${status}${lastEvent}`
  }

  return undefined
}

async function validateResolvedCaseModels(
  client: RunnerClient,
  suite: PromptSuite,
  resolvedCases: ReadonlyArray<ResolvedPromptCase>,
): Promise<void> {
  const result = await client.config.providers(undefined, { throwOnError: true })
  const provider = result.data?.providers.find((item: { id: string; models: Record<string, unknown> }) => item.id === "openrouter")
  if (!provider) return

  const invalid = collectInvalidResolvedModels(suite, resolvedCases, Object.keys(provider.models))
  if (invalid.length === 0) return

  const details = invalid
    .map((item) => `- ${item.model} from ${item.source} (cases: ${item.caseIds.join(", ")})`)
    .join("\n")

  throw new Error(
    [
      'Invalid model configuration for provider "openrouter".',
      "These model IDs are not available in the current OpenCode provider config:",
      details,
      'Update "buddyevals.suite.json" to a valid model ID and rerun.',
    ].join("\n"),
  )
}

export const runCommand = Command.make(
  "run",
  {
    suite: suiteOption,
    caseId: caseOption,
    concurrency: concurrencyOption,
    dashboard: dashboardOption,
  },
  ({ suite, caseId, concurrency, dashboard }) =>
    Effect.gen(function* () {
      const suitePath = resolveSuitePath(suite)
      const workspace = yield* Effect.tryPromise(() => ensureWorkspaceState())
      const globalConfig = yield* Effect.tryPromise(() => readEffectiveConfig())
      const apiKey = resolveOpenRouterApiKey(globalConfig)
      if (!apiKey) {
        throw new Error("Missing OpenRouter API key. Set it in ~/.config/buddyevals/config.json or OPENROUTER_API_KEY.")
      }

      const parsedSuite = yield* Effect.tryPromise(() => loadSuite(suitePath))
      const selectedCases = parsedSuite.cases.filter((item) => Option.isNone(caseId) || item.id === caseId.value)
      if (selectedCases.length === 0) {
        throw new Error(Option.isSome(caseId) ? `No case found with id "${caseId.value}"` : "Suite has no cases to run")
      }

      const resolvedCases = selectedCases.map((item) => resolvePromptCase(parsedSuite, item, globalConfig))
      const runId = crypto.randomUUID()
      const runStartedAt = Date.now()
      const runAbort = new AbortController()
      const counts = { passed: 0, failed: 0, cancelled: 0 }
      let runRecorded = false
      let runStatus: CaseStatus = "running"
      const onSigInt = () => {
        runAbort.abort("SIGINT")
      }
      process.once("SIGINT", onSigInt)

      try {
        yield* Effect.tryPromise(() =>
          Effect.runPromise(
            Repository.insertRun(workspace.dbPath, {
              id: runId,
              suitePath,
              startedAt: runStartedAt,
              completedAt: null,
              status: "running",
              totalCases: resolvedCases.length,
              passed: 0,
              failed: 0,
              cancelled: 0,
            }),
          ),
        )
        runRecorded = true

        yield* Console.log(
          `Running ${resolvedCases.length} case(s) against OpenCode with concurrency=${concurrency}`,
        )

        yield* OpenCodeRunner.withRunner(
          {
            apiKey,
            config: globalConfig,
          },
          async (client) => {
            await validateResolvedCaseModels(client, parsedSuite, resolvedCases)
            await Effect.runPromise(
              Effect.forEach(
                resolvedCases,
                (item) =>
                  Effect.tryPromise(async () => {
                    const artifactDir = path.join(workspace.artifactsDir, runId, item.id)
                    const caseDirectory = buildCaseWorkspaceDirectory({
                      baseDirectory: item.directory,
                      caseId: item.id,
                      model: item.model,
                      cwd: process.cwd(),
                      workspaceBaseDirectory: workspace.workspacesDir,
                    })
                    const caseRunId = crypto.randomUUID()
                    await mkdir(caseDirectory, { recursive: true })
                    await Effect.runPromise(
                      Repository.upsertCaseRun(
                        workspace.dbPath,
                        buildCaseRunRecord({
                          id: caseRunId,
                          runId,
                          caseId: item.id,
                          title: item.title,
                          model: item.model,
                          agent: item.agent,
                          directory: caseDirectory,
                          status: "running",
                          startedAt: Date.now(),
                          completedAt: null,
                          durationMs: null,
                          artifactDir,
                        }),
                      ),
                    )

                    await Effect.runPromise(Repository.touchRecentModel(workspace.dbPath, item.model))
                    await Console.log(
                      `[running] ${item.id} · model=${item.model} · dir=${toWorkspacePath(caseDirectory)} · artifacts=${toWorkspacePath(artifactDir)}`,
                    )

                    const result = await runPromptCase(
                      client,
                      {
                        ...item,
                        directory: caseDirectory,
                      },
                      runAbort.signal,
                      (progress) => {
                        const line = formatProgress(item.id, progress)
                        if (line) {
                          console.log(line)
                        }
                      },
                    )
                    const summary = {
                      ...result,
                      caseId: item.id,
                      model: item.model,
                      directory: caseDirectory,
                    }
                    await writeCaseArtifacts({
                      artifactDir,
                      resolvedCase: item,
                      events: result.events,
                      messages: result.messages,
                      summary,
                    })

                    if (result.status === "passed") counts.passed += 1
                    if (result.status === "failed") counts.failed += 1
                    if (result.status === "cancelled") counts.cancelled += 1

                    await Effect.runPromise(
                      Repository.upsertCaseRun(
                        workspace.dbPath,
                        buildCaseRunRecord({
                          id: caseRunId,
                          runId,
                          caseId: item.id,
                          title: item.title,
                          model: item.model,
                          agent: item.agent,
                          directory: caseDirectory,
                          sessionId: result.sessionId,
                          status: result.status,
                          startedAt: result.startedAt,
                          completedAt: result.completedAt,
                          durationMs: result.durationMs,
                          tokens: result.tokens,
                          cost: result.cost,
                          error: result.error,
                          artifactDir,
                        }),
                      ),
                    )

                    const errorSummary = result.error ? ` · error=${result.error.name}: ${result.error.message}` : ""
                    await Console.log(
                      `[${result.status}] ${item.id} · duration=${formatDuration(result.durationMs)} · passed=${counts.passed} failed=${counts.failed} cancelled=${counts.cancelled} · artifacts=${toWorkspacePath(artifactDir)}${errorSummary}`,
                    )
                    return {
                      caseId: item.id,
                      status: result.status,
                      model: item.model,
                      durationMs: result.durationMs,
                      error: result.error?.message,
                    }
                  }),
                { concurrency },
              ),
            ).then((rows) => {
              console.table(rows)
            })
          },
        )

        runStatus = aggregateRunStatus(counts)
      } catch (error) {
        runStatus = runAbort.signal.aborted ? "cancelled" : "failed"
        throw error
      } finally {
        process.off("SIGINT", onSigInt)

        if (runRecorded) {
          const finalStatus = runStatus === "running" ? aggregateRunStatus(counts) : runStatus
          const completedAt = Date.now()
          yield* Effect.tryPromise(() =>
            Effect.runPromise(
              Repository.updateRun(workspace.dbPath, {
                id: runId,
                completedAt,
                status: finalStatus,
                passed: counts.passed,
                failed: counts.failed,
                cancelled: counts.cancelled,
              }),
            ),
          )

          yield* Console.log(
            `Run ${runId} finished with status=${finalStatus} passed=${counts.passed} failed=${counts.failed} cancelled=${counts.cancelled}`,
          )
        }
      }

      if (dashboard) {
        yield* runDashboard(workspace.dbPath).pipe(Effect.provide(PromptTuiIOLayer))
      }
    }),
).pipe(Command.withDescription("Execute a prompt suite against OpenCode and persist run results"))
