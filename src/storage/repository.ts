import { SqlClient } from "@effect/sql"
import { Effect } from "effect"
import type { CaseStatus, NormalizedError } from "@/domain/outcome"
import type { RunRow, CaseRunRow, RecentModelRow } from "@/storage/database"
import { withDatabase } from "@/storage/database"

export type RunRecord = {
  id: string
  suitePath: string
  startedAt: number
  completedAt: number | null
  status: CaseStatus
  totalCases: number
  passed: number
  failed: number
  cancelled: number
}

export type CaseRunRecord = {
  id: string
  runId: string
  caseId: string
  title: string
  model: string
  agent?: string
  directory: string
  sessionId?: string
  status: CaseStatus
  startedAt: number
  completedAt: number | null
  durationMs: number | null
  tokensInput: number | null
  tokensOutput: number | null
  tokensReasoning: number | null
  tokensTotal: number | null
  cost: number | null
  errorName: string | null
  errorMessage: string | null
  artifactDir: string
}

function mapRunRow(row: RunRow): RunRecord {
  return {
    id: row.id,
    suitePath: row.suite_path,
    startedAt: row.started_at,
    completedAt: row.completed_at,
    status: row.status as CaseStatus,
    totalCases: row.total_cases,
    passed: row.passed,
    failed: row.failed,
    cancelled: row.cancelled,
  }
}

function mapCaseRunRow(row: CaseRunRow): CaseRunRecord {
  return {
    id: row.id,
    runId: row.run_id,
    caseId: row.case_id,
    title: row.title,
    model: row.model,
    agent: row.agent ?? undefined,
    directory: row.directory,
    sessionId: row.session_id ?? undefined,
    status: row.status as CaseStatus,
    startedAt: row.started_at,
    completedAt: row.completed_at,
    durationMs: row.duration_ms,
    tokensInput: row.tokens_input,
    tokensOutput: row.tokens_output,
    tokensReasoning: row.tokens_reasoning,
    tokensTotal: row.tokens_total,
    cost: row.cost,
    errorName: row.error_name,
    errorMessage: row.error_message,
    artifactDir: row.artifact_dir,
  }
}

export const Repository = {
  insertRun: (dbPath: string, input: RunRecord) =>
    withDatabase(
      dbPath,
      Effect.gen(function* () {
        const sql = yield* SqlClient.SqlClient
        yield* sql`
          INSERT INTO runs (
            id, suite_path, started_at, completed_at, status, total_cases, passed, failed, cancelled
          ) VALUES (
            ${input.id},
            ${input.suitePath},
            ${input.startedAt},
            ${input.completedAt},
            ${input.status},
            ${input.totalCases},
            ${input.passed},
            ${input.failed},
            ${input.cancelled}
          )
        `
      }),
    ),

  updateRun: (
    dbPath: string,
    input: Pick<RunRecord, "id" | "completedAt" | "status" | "passed" | "failed" | "cancelled">,
  ) =>
    withDatabase(
      dbPath,
      Effect.gen(function* () {
        const sql = yield* SqlClient.SqlClient
        yield* sql`
          UPDATE runs
          SET
            completed_at = ${input.completedAt},
            status = ${input.status},
            passed = ${input.passed},
            failed = ${input.failed},
            cancelled = ${input.cancelled}
          WHERE id = ${input.id}
        `
      }),
    ),

  upsertCaseRun: (dbPath: string, input: CaseRunRecord) =>
    withDatabase(
      dbPath,
      Effect.gen(function* () {
        const sql = yield* SqlClient.SqlClient
        yield* sql`
          INSERT INTO case_runs (
            id, run_id, case_id, title, model, agent, directory, session_id, status, started_at, completed_at,
            duration_ms, tokens_input, tokens_output, tokens_reasoning, tokens_total, cost, error_name,
            error_message, artifact_dir
          ) VALUES (
            ${input.id},
            ${input.runId},
            ${input.caseId},
            ${input.title},
            ${input.model},
            ${input.agent ?? null},
            ${input.directory},
            ${input.sessionId ?? null},
            ${input.status},
            ${input.startedAt},
            ${input.completedAt},
            ${input.durationMs},
            ${input.tokensInput},
            ${input.tokensOutput},
            ${input.tokensReasoning},
            ${input.tokensTotal},
            ${input.cost},
            ${input.errorName},
            ${input.errorMessage},
            ${input.artifactDir}
          )
          ON CONFLICT(id) DO UPDATE SET
            session_id = excluded.session_id,
            status = excluded.status,
            completed_at = excluded.completed_at,
            duration_ms = excluded.duration_ms,
            tokens_input = excluded.tokens_input,
            tokens_output = excluded.tokens_output,
            tokens_reasoning = excluded.tokens_reasoning,
            tokens_total = excluded.tokens_total,
            cost = excluded.cost,
            error_name = excluded.error_name,
            error_message = excluded.error_message,
            artifact_dir = excluded.artifact_dir
        `
      }),
    ),

  touchRecentModel: (dbPath: string, model: string, lastUsedAt = Date.now()) =>
    withDatabase(
      dbPath,
      Effect.gen(function* () {
        const sql = yield* SqlClient.SqlClient
        yield* sql`
          INSERT INTO recent_models (model, last_used_at)
          VALUES (${model}, ${lastUsedAt})
          ON CONFLICT(model) DO UPDATE SET
            last_used_at = excluded.last_used_at
        `
      }),
    ),

  listRuns: (dbPath: string, limit = 20) =>
    withDatabase(
      dbPath,
      Effect.gen(function* () {
        const sql = yield* SqlClient.SqlClient
        const rows = yield* sql<RunRow>`
          SELECT *
          FROM runs
          ORDER BY started_at DESC
          LIMIT ${limit}
        `
        return rows.map(mapRunRow)
      }),
    ),

  listCaseRuns: (dbPath: string, runId: string) =>
    withDatabase(
      dbPath,
      Effect.gen(function* () {
        const sql = yield* SqlClient.SqlClient
        const rows = yield* sql<CaseRunRow>`
          SELECT *
          FROM case_runs
          WHERE run_id = ${runId}
          ORDER BY started_at ASC
        `
        return rows.map(mapCaseRunRow)
      }),
    ),

  listRecentModels: (dbPath: string, limit = 10) =>
    withDatabase(
      dbPath,
      Effect.gen(function* () {
        const sql = yield* SqlClient.SqlClient
        const rows = yield* sql<RecentModelRow>`
          SELECT model, last_used_at
          FROM recent_models
          ORDER BY last_used_at DESC
          LIMIT ${limit}
        `
        return rows.map((row) => row.model)
      }),
    ),
}

export function buildCaseRunRecord(input: {
  id: string
  runId: string
  caseId: string
  title: string
  model: string
  agent?: string
  directory: string
  sessionId?: string
  status: CaseStatus
  startedAt: number
  completedAt: number | null
  durationMs: number | null
  tokens?: {
    input?: number
    output?: number
    reasoning?: number
    total?: number
  }
  cost?: number
  error?: NormalizedError
  artifactDir: string
}): CaseRunRecord {
  return {
    id: input.id,
    runId: input.runId,
    caseId: input.caseId,
    title: input.title,
    model: input.model,
    agent: input.agent,
    directory: input.directory,
    sessionId: input.sessionId,
    status: input.status,
    startedAt: input.startedAt,
    completedAt: input.completedAt,
    durationMs: input.durationMs,
    tokensInput: input.tokens?.input ?? null,
    tokensOutput: input.tokens?.output ?? null,
    tokensReasoning: input.tokens?.reasoning ?? null,
    tokensTotal: input.tokens?.total ?? null,
    cost: input.cost ?? null,
    errorName: input.error?.name ?? null,
    errorMessage: input.error?.message ?? null,
    artifactDir: input.artifactDir,
  }
}
