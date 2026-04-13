import { BunContext, BunRuntime } from "@effect/platform-bun"
import * as Effect from "effect/Effect"
import { cli } from "@/cli"

const program = Effect.suspend(() => cli(process.argv)).pipe(Effect.provide(BunContext.layer))

BunRuntime.runMain(program as Effect.Effect<void>)
