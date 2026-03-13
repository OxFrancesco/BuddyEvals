import { expect, test } from "bun:test";

import { getProviders, runEval, waitForSessionOutcome } from "./opencode-sdk";

function asyncStream(events: unknown[]): AsyncIterable<unknown> {
  return {
    async *[Symbol.asyncIterator]() {
      for (const event of events) {
        yield event;
      }
    },
  };
}

function neverStream(): AsyncIterable<unknown> {
  return {
    [Symbol.asyncIterator]() {
      return {
        next: () => new Promise<IteratorResult<unknown>>(() => {}),
        return: async () => ({ done: true, value: undefined }),
      };
    },
  };
}

test("getProviders uses an existing client when available", async () => {
  let createOpencodeCalls = 0;

  const result = await getProviders(
    { hostname: "127.0.0.1", port: 4096 },
    {
      createOpencode: async () => {
        createOpencodeCalls += 1;
        throw new Error("should not start a server");
      },
      createOpencodeClient: () => ({
        config: {
          providers: async () => ({
            providers: [{ id: "openrouter", models: { "glm-5": {} } }],
            default: { openrouter: "glm-5" },
          }),
        },
      }),
      now: () => Date.now(),
      sleep: (ms: number) => new Promise((resolve) => setTimeout(resolve, ms)),
    } as any,
  );

  expect(result.providers).toHaveLength(1);
  expect(result.default.openrouter).toBe("glm-5");
  expect(createOpencodeCalls).toBe(0);
});

test("getProviders falls back to a temporary server when the local client is unavailable", async () => {
  let closeCalls = 0;

  const result = await getProviders(
    { hostname: "127.0.0.1", port: 4096 },
    {
      createOpencode: async () => ({
        client: {
          config: {
            providers: async () => ({
              providers: [{ id: "anthropic", models: { "claude-sonnet-4": {} } }],
              default: { anthropic: "claude-sonnet-4" },
            }),
          },
        },
        server: {
          close: async () => {
            closeCalls += 1;
          },
        },
      }),
      createOpencodeClient: () => ({
        config: {
          providers: async () => {
            throw new Error("connect ECONNREFUSED 127.0.0.1:4096");
          },
        },
      }),
      now: () => Date.now(),
      sleep: (ms: number) => new Promise((resolve) => setTimeout(resolve, ms)),
    } as any,
  );

  expect(result.providers).toHaveLength(1);
  expect(result.default.anthropic).toBe("claude-sonnet-4");
  expect(closeCalls).toBe(1);
});

test("runEval completes successfully when the session reaches idle", async () => {
  let closeCalls = 0;

  const result = await runEval(
    {
      title: "Eval 0",
      prompt: "hello",
      providerID: "openrouter",
      modelID: "glm-5",
      inactivityTimeoutSeconds: 5,
    },
    {
      createOpencode: async () => ({
        client: {
          config: {
            providers: async () => ({ providers: [], default: {} }),
          },
          event: {
            subscribe: async () => ({
              stream: asyncStream([
                { type: "session.status", properties: { sessionID: "session-1", status: { type: "busy" } } },
                { type: "session.idle", properties: { sessionID: "session-1" } },
              ]),
            }),
          },
          session: {
            create: async () => ({ id: "session-1" }),
            prompt: async () => ({}),
            abort: async () => true,
          },
        },
        server: {
          close: async () => {
            closeCalls += 1;
          },
        },
      }),
      createOpencodeClient: () => {
        throw new Error("unused");
      },
      now: () => Date.now(),
      sleep: (ms: number) => new Promise((resolve) => setTimeout(resolve, ms)),
    } as any,
  );

  expect(result.success).toBe(true);
  expect(result.sessionID).toBe("session-1");
  expect(result.completedBy).toBe("session.idle");
  expect(closeCalls).toBe(1);
});

test("runEval surfaces session errors and still closes the server", async () => {
  let closeCalls = 0;

  const result = await runEval(
    {
      title: "Eval 1",
      prompt: "hello",
      providerID: "openrouter",
      modelID: "glm-5",
      inactivityTimeoutSeconds: 5,
    },
    {
      createOpencode: async () => ({
        client: {
          config: {
            providers: async () => ({ providers: [], default: {} }),
          },
          event: {
            subscribe: async () => ({
              stream: asyncStream([
                {
                  type: "session.error",
                  properties: { sessionID: "session-2", error: { data: { message: "Model not found. Did you mean: openrouter/glm-5?" } } },
                },
              ]),
            }),
          },
          session: {
            create: async () => ({ id: "session-2" }),
            prompt: async () => ({}),
            abort: async () => true,
          },
        },
        server: {
          close: async () => {
            closeCalls += 1;
          },
        },
      }),
      createOpencodeClient: () => {
        throw new Error("unused");
      },
      now: () => Date.now(),
      sleep: (ms: number) => new Promise((resolve) => setTimeout(resolve, ms)),
    } as any,
  );

  expect(result.success).toBe(false);
  expect(result.error).toBe("Model not found. Did you mean: openrouter/glm-5?");
  expect(closeCalls).toBe(1);
});

test("waitForSessionOutcome aborts the session when inactivity exceeds the timeout", async () => {
  let abortedSessionID = "";
  let now = 0;

  const outcome = await waitForSessionOutcome(
    {
      session: {
        abort: async ({ path }: { path: { id: string } }) => {
          abortedSessionID = path.id;
          return true;
        },
      },
    } as any,
    neverStream(),
    "session-timeout",
    1,
    { error: "" },
    {
      createOpencode: async () => {
        throw new Error("unused");
      },
      createOpencodeClient: () => {
        throw new Error("unused");
      },
      now: () => now,
      sleep: async (ms: number) => {
        now += ms;
      },
    } as any,
  );

  expect(outcome.success).toBe(false);
  expect(outcome.error).toBe("no agent activity for 1s");
  expect(abortedSessionID).toBe("session-timeout");
});
