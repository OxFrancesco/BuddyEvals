import * as Effect from "effect/Effect"
import { Repository } from "@/storage/repository"
import { readSummaryArtifact } from "@/storage/artifacts"
import { TuiIO } from "@/tui/io"

export const runDashboard = (dbPath: string) =>
  Effect.gen(function* () {
    const io = yield* TuiIO
    const runs = yield* Effect.tryPromise(() => Effect.runPromise(Repository.listRuns(dbPath)))

    if (runs.length === 0) {
      yield* io.info("No persisted runs found.")
      return
    }

    const runChoice = yield* io.select("Select a run", runs.map((run) => ({
      title: `${new Date(run.startedAt).toLocaleString()} · ${run.status} · ${run.passed}/${run.totalCases} passed`,
      value: run.id,
      description: run.suitePath,
    })))

    const run = runs.find((item) => item.id === runChoice)
    if (!run) return

    yield* io.info(
      [
        `Run ${run.id}`,
        `Suite: ${run.suitePath}`,
        `Status: ${run.status}`,
        `Started: ${new Date(run.startedAt).toLocaleString()}`,
        `Completed: ${run.completedAt ? new Date(run.completedAt).toLocaleString() : "still running"}`,
        `Counts: passed=${run.passed} failed=${run.failed} cancelled=${run.cancelled}`,
      ].join("\n"),
    )

    const caseRuns = yield* Effect.tryPromise(() => Effect.runPromise(Repository.listCaseRuns(dbPath, run.id)))
    if (caseRuns.length === 0) {
      yield* io.info("This run has no case records.")
      return
    }

    const caseChoice = yield* io.select(
      "Select a case",
      caseRuns.map((item) => ({
        title: `${item.title} · ${item.status}`,
        value: item.id,
        description: item.model,
      })),
    )

    const caseRun = caseRuns.find((item) => item.id === caseChoice)
    if (!caseRun) return

    const summary = yield* Effect.tryPromise(() => readSummaryArtifact<Record<string, unknown>>(caseRun.artifactDir))

    yield* io.info(
      [
        `Case ${caseRun.caseId}`,
        `Title: ${caseRun.title}`,
        `Status: ${caseRun.status}`,
        `Model: ${caseRun.model}`,
        `Agent: ${caseRun.agent ?? "<unset>"}`,
        `Directory: ${caseRun.directory}`,
        `Session: ${caseRun.sessionId ?? "<none>"}`,
        `Duration: ${caseRun.durationMs ?? 0}ms`,
        `Tokens: in=${caseRun.tokensInput ?? 0} out=${caseRun.tokensOutput ?? 0} reasoning=${caseRun.tokensReasoning ?? 0} total=${caseRun.tokensTotal ?? 0}`,
        `Cost: ${caseRun.cost ?? 0}`,
        `Error: ${caseRun.errorName ? `${caseRun.errorName}: ${caseRun.errorMessage}` : "<none>"}`,
        `Artifacts: ${caseRun.artifactDir}`,
        `Summary artifact: ${summary ? JSON.stringify(summary, null, 2) : "<missing>"}`,
      ].join("\n"),
    )
  })
