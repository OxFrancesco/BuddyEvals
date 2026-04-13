import { SqlClient } from "@effect/sql"
import { SqliteClient } from "@effect/sql-sqlite-bun"
import { Effect, Layer } from "effect"

export type RunRow = {
  id: string
  suite_path: string
  started_at: number
  completed_at: number | null
  status: string
  total_cases: number
  passed: number
  failed: number
  cancelled: number
}

export type CaseRunRow = {
  id: string
  run_id: string
  case_id: string
  title: string
  model: string
  agent: string | null
  directory: string
  session_id: string | null
  status: string
  started_at: number
  completed_at: number | null
  duration_ms: number | null
  tokens_input: number | null
  tokens_output: number | null
  tokens_reasoning: number | null
  tokens_total: number | null
  cost: number | null
  error_name: string | null
  error_message: string | null
  artifact_dir: string
}

export type RecentModelRow = {
  model: string
  last_used_at: number
}

export function makeDatabaseLayer(filename: string) {
  return SqliteClient.layer({
    filename,
  })
}

export const ensureSchema = Effect.gen(function* () {
  const sql = yield* SqlClient.SqlClient

  yield* sql`
    CREATE TABLE IF NOT EXISTS runs (
      id TEXT PRIMARY KEY,
      suite_path TEXT NOT NULL,
      started_at INTEGER NOT NULL,
      completed_at INTEGER,
      status TEXT NOT NULL,
      total_cases INTEGER NOT NULL,
      passed INTEGER NOT NULL,
      failed INTEGER NOT NULL,
      cancelled INTEGER NOT NULL
    )
  `

  yield* sql`
    CREATE TABLE IF NOT EXISTS case_runs (
      id TEXT PRIMARY KEY,
      run_id TEXT NOT NULL,
      case_id TEXT NOT NULL,
      title TEXT NOT NULL,
      model TEXT NOT NULL,
      agent TEXT,
      directory TEXT NOT NULL,
      session_id TEXT,
      status TEXT NOT NULL,
      started_at INTEGER NOT NULL,
      completed_at INTEGER,
      duration_ms INTEGER,
      tokens_input INTEGER,
      tokens_output INTEGER,
      tokens_reasoning INTEGER,
      tokens_total INTEGER,
      cost REAL,
      error_name TEXT,
      error_message TEXT,
      artifact_dir TEXT NOT NULL
    )
  `

  yield* sql`
    CREATE TABLE IF NOT EXISTS recent_models (
      model TEXT PRIMARY KEY,
      last_used_at INTEGER NOT NULL
    )
  `
})

export function withDatabase<A, E, R>(dbPath: string, effect: Effect.Effect<A, E, R | SqlClient.SqlClient>) {
  return Effect.gen(function* () {
    yield* ensureSchema
    return yield* effect
  }).pipe(Effect.provide(makeDatabaseLayer(dbPath)))
}
