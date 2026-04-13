import * as Prompt from "@effect/cli/Prompt"
import * as Console from "effect/Console"
import * as Context from "effect/Context"
import * as Effect from "effect/Effect"
import * as Layer from "effect/Layer"

export type SelectChoice = {
  title: string
  value: string
  description?: string
}

export interface TuiIO {
  readonly select: (message: string, choices: ReadonlyArray<SelectChoice>) => Effect.Effect<string>
  readonly text: (
    message: string,
    options?: {
      default?: string
    },
  ) => Effect.Effect<string>
  readonly confirm: (message: string, initial?: boolean) => Effect.Effect<boolean>
  readonly info: (message: string) => Effect.Effect<void>
}

export const TuiIO = Context.GenericTag<TuiIO>("BuddyEvals/TuiIO")

export const PromptTuiIOLayer = Layer.succeed(TuiIO, {
  select: (message, choices) =>
    Prompt.select({
      message,
      choices: choices.map((choice) => ({
        title: choice.title,
        value: choice.value,
        description: choice.description,
      })),
    }) as unknown as Effect.Effect<string>,
  text: (message, options) =>
    Prompt.text({
      message,
      default: options?.default,
    }) as unknown as Effect.Effect<string>,
  confirm: (message, initial) =>
    Prompt.confirm({
      message,
      initial,
    }) as unknown as Effect.Effect<boolean>,
  info: (message) => Console.log(message),
} satisfies TuiIO)
