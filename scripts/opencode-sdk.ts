#!/usr/bin/env bun
import { createOpencode, createOpencodeClient } from "@opencode-ai/sdk";

type JSONRecord = Record<string, unknown>;

export type ProvidersOutput = {
  providers: unknown[];
  default: Record<string, string>;
};

export type RunEvalInput = {
  title: string;
  prompt: string;
  providerID: string;
  modelID: string;
  hostname?: string;
  port?: number;
  inactivityTimeoutSeconds?: number;
};

export type RunEvalOutput = {
  success: boolean;
  sessionID?: string;
  error?: string;
  completedBy?: string;
  durationMs: number;
};

type PromptState = {
  error: string;
};

type EventLike = {
  type?: unknown;
  properties?: unknown;
};

type SessionOutcome = {
  success: boolean;
  error?: string;
  completedBy?: string;
};

type AsyncEventStream = AsyncIterable<unknown>;

type SDKClientLike = {
  config: {
    providers: () => Promise<unknown>;
  };
  session: {
    create: (input: { body: { title: string } }) => Promise<unknown>;
    prompt: (input: {
      path: { id: string };
      body: {
        model: { providerID: string; modelID: string };
        parts: Array<{ type: "text"; text: string }>;
      };
    }) => Promise<unknown>;
    abort: (input: { path: { id: string } }) => Promise<unknown>;
  };
  event: {
    subscribe: () => Promise<{ stream: AsyncEventStream }>;
  };
};

type OpencodeServerLike = {
  url?: string;
  close: () => void | Promise<void>;
};

type OpencodeInstanceLike = {
  client: SDKClientLike;
  server: OpencodeServerLike;
};

type BridgeDeps = {
  createOpencode: typeof createOpencode;
  createOpencodeClient: typeof createOpencodeClient;
  now: () => number;
  sleep: (ms: number) => Promise<void>;
};

const defaultDeps: BridgeDeps = {
  createOpencode,
  createOpencodeClient,
  now: () => Date.now(),
  sleep: (ms) => new Promise((resolve) => setTimeout(resolve, ms)),
};

const DEFAULT_HOSTNAME = "127.0.0.1";
const DEFAULT_PORT = 4096;
const DEFAULT_INACTIVITY_TIMEOUT_SECONDS = 180;
const EVENT_POLL_INTERVAL_MS = 1000;

function isRecord(value: unknown): value is JSONRecord {
  return typeof value === "object" && value !== null;
}

function unwrapData<T>(value: unknown): T {
  if (isRecord(value) && "data" in value) {
    return value.data as T;
  }
  return value as T;
}

export function extractErrorMessage(error: unknown): string {
  if (error instanceof Error && error.message) {
    return error.message;
  }

  if (typeof error === "string") {
    return error;
  }

  if (isRecord(error)) {
    if (isRecord(error.data) && typeof error.data.message === "string") {
      return error.data.message;
    }
    if (typeof error.message === "string") {
      return error.message;
    }
    if (isRecord(error.error)) {
      return extractErrorMessage(error.error);
    }
    if (typeof error.name === "string") {
      return error.name;
    }
  }

  return String(error);
}

function normalizeProvidersOutput(value: unknown): ProvidersOutput {
  const data = unwrapData<JSONRecord>(value);
  const providers = Array.isArray(data.providers) ? data.providers : [];
  const defaults = isRecord(data.default) ? data.default : {};
  const normalizedDefaults: Record<string, string> = {};

  for (const [key, rawValue] of Object.entries(defaults)) {
    if (typeof rawValue === "string") {
      normalizedDefaults[key] = rawValue;
    }
  }

  return {
    providers,
    default: normalizedDefaults,
  };
}

function extractSessionID(value: unknown): string {
  const data = unwrapData<JSONRecord>(value);
  return typeof data.id === "string" ? data.id : "";
}

function extractPromptError(value: unknown): string {
  if (!isRecord(value)) {
    return "";
  }

  if (isRecord(value.info) && value.info.error !== undefined) {
    return extractErrorMessage(value.info.error);
  }

  if (isRecord(value.data) && isRecord(value.data.info) && value.data.info.error !== undefined) {
    return extractErrorMessage(value.data.info.error);
  }

  if (value.error !== undefined) {
    return extractErrorMessage(value.error);
  }

  return "";
}

function getEventType(event: unknown): string {
  return isRecord(event) && typeof event.type === "string" ? event.type : "";
}

function getEventProperties(event: unknown): JSONRecord {
  if (isRecord(event) && isRecord(event.properties)) {
    return event.properties;
  }
  return {};
}

function getEventSessionID(properties: JSONRecord): string {
  if (typeof properties.sessionID === "string") {
    return properties.sessionID;
  }
  if (typeof properties.sessionId === "string") {
    return properties.sessionId;
  }
  return "";
}

export function isRelevantSessionEvent(event: unknown, sessionID: string): boolean {
  const type = getEventType(event);
  if (type.startsWith("server.")) {
    return false;
  }

  const eventSessionID = getEventSessionID(getEventProperties(event));
  if (!eventSessionID) {
    return true;
  }

  return eventSessionID === sessionID;
}

function getStatusType(properties: JSONRecord): string {
  if (isRecord(properties.status) && typeof properties.status.type === "string") {
    return properties.status.type;
  }
  return "";
}

function isUnavailableError(message: string): boolean {
  const normalized = message.toLowerCase();
  return normalized.includes("econnrefused") ||
    normalized.includes("fetch") ||
    normalized.includes("network") ||
    normalized.includes("connect") ||
    normalized.includes("socket") ||
    normalized.includes("timed out");
}

async function closeServer(server: OpencodeServerLike | undefined): Promise<void> {
  if (!server) {
    return;
  }
  await server.close();
}

async function abortSession(client: SDKClientLike, sessionID: string): Promise<void> {
  try {
    await client.session.abort({ path: { id: sessionID } });
  } catch {
    // Best-effort cleanup only.
  }
}

async function readStdinText(): Promise<string> {
  const chunks: Uint8Array[] = [];
  for await (const chunk of process.stdin) {
    chunks.push(typeof chunk === "string" ? Buffer.from(chunk) : chunk);
  }
  return Buffer.concat(chunks).toString("utf8");
}

async function readRunEvalInput(): Promise<RunEvalInput> {
  const body = await readStdinText();
  const parsed = JSON.parse(body) as Partial<RunEvalInput>;

  if (!parsed.title || !parsed.prompt || !parsed.providerID || !parsed.modelID) {
    throw new Error("run-eval requires title, prompt, providerID, and modelID");
  }

  return {
    title: parsed.title,
    prompt: parsed.prompt,
    providerID: parsed.providerID,
    modelID: parsed.modelID,
    hostname: parsed.hostname ?? DEFAULT_HOSTNAME,
    port: parsed.port ?? DEFAULT_PORT,
    inactivityTimeoutSeconds: parsed.inactivityTimeoutSeconds ?? DEFAULT_INACTIVITY_TIMEOUT_SECONDS,
  };
}

function readProvidersArgs(argv: string[]): { hostname: string; port: number } {
  const hostnameIndex = argv.indexOf("--hostname");
  const portIndex = argv.indexOf("--port");

  const hostname = hostnameIndex !== -1 ? argv[hostnameIndex + 1] : undefined;
  const portValue = portIndex !== -1 ? argv[portIndex + 1] : undefined;
  const port = portValue ? Number.parseInt(portValue, 10) : DEFAULT_PORT;

  return {
    hostname: hostname || DEFAULT_HOSTNAME,
    port: Number.isFinite(port) ? port : DEFAULT_PORT,
  };
}

export async function getProviders(
  input: { hostname?: string; port?: number } = {},
  deps: BridgeDeps = defaultDeps,
): Promise<ProvidersOutput> {
  const hostname = input.hostname ?? DEFAULT_HOSTNAME;
  const port = input.port ?? DEFAULT_PORT;
  const baseUrl = `http://${hostname}:${port}`;

  try {
    const client = deps.createOpencodeClient({
      baseUrl,
      throwOnError: true,
    }) as unknown as SDKClientLike;
    return normalizeProvidersOutput(await client.config.providers());
  } catch (error) {
    const message = extractErrorMessage(error);
    if (!isUnavailableError(message)) {
      throw error;
    }
  }

  let opencode: OpencodeInstanceLike | undefined;
  try {
    opencode = await deps.createOpencode({
      hostname,
      port,
    }) as unknown as OpencodeInstanceLike;
    return normalizeProvidersOutput(await opencode.client.config.providers());
  } finally {
    await closeServer(opencode?.server);
  }
}

export async function waitForSessionOutcome(
  client: SDKClientLike,
  stream: AsyncEventStream,
  sessionID: string,
  inactivityTimeoutSeconds: number,
  promptState: PromptState,
  deps: BridgeDeps = defaultDeps,
): Promise<SessionOutcome> {
  const timeoutMs = Math.max(1, inactivityTimeoutSeconds) * 1000;
  let lastActivityAt = deps.now();
  const iterator = stream[Symbol.asyncIterator]();
  let nextEvent = iterator.next()
    .then((value) => ({ kind: "event" as const, value }))
    .catch((error) => ({ kind: "stream_error" as const, error }));

  try {
    for (;;) {
      if (promptState.error) {
        return { success: false, error: promptState.error };
      }

      if (deps.now() - lastActivityAt > timeoutMs) {
        await abortSession(client, sessionID);
        return {
          success: false,
          error: `no agent activity for ${Math.max(1, inactivityTimeoutSeconds)}s`,
        };
      }

      const next = await Promise.race([
        nextEvent,
        deps.sleep(EVENT_POLL_INTERVAL_MS).then(() => ({ kind: "tick" as const })),
      ]);

      if (next.kind === "tick") {
        continue;
      }

      if (next.kind === "stream_error") {
        return {
          success: false,
          error: `event stream error: ${extractErrorMessage(next.error)}`,
        };
      }

      if (next.value.done) {
        if (promptState.error) {
          return { success: false, error: promptState.error };
        }
        return { success: false, error: "agent did not reach idle state" };
      }

      nextEvent = iterator.next()
        .then((value) => ({ kind: "event" as const, value }))
        .catch((error) => ({ kind: "stream_error" as const, error }));

      const event = next.value.value as EventLike;
      if (!isRelevantSessionEvent(event, sessionID)) {
        continue;
      }

      lastActivityAt = deps.now();

      const type = getEventType(event);
      const properties = getEventProperties(event);

      if (type === "session.idle") {
        return { success: true, completedBy: "session.idle" };
      }

      if (type === "session.status") {
        const statusType = getStatusType(properties);
        if (statusType === "idle") {
          return { success: true, completedBy: "session.status.idle" };
        }
        continue;
      }

      if (type === "session.error") {
        return {
          success: false,
          error: extractErrorMessage(properties.error ?? "unknown session error"),
        };
      }
    }
  } finally {
    if (typeof iterator.return === "function") {
      await iterator.return();
    }
  }
}

export async function runEval(
  input: RunEvalInput,
  deps: BridgeDeps = defaultDeps,
): Promise<RunEvalOutput> {
  const startedAt = deps.now();
  let opencode: OpencodeInstanceLike | undefined;
  let sessionID = "";
  const promptState: PromptState = { error: "" };

  try {
    opencode = await deps.createOpencode({
      hostname: input.hostname ?? DEFAULT_HOSTNAME,
      port: input.port ?? DEFAULT_PORT,
    }) as unknown as OpencodeInstanceLike;

    const session = await opencode.client.session.create({
      body: { title: input.title },
    });
    sessionID = extractSessionID(session);
    if (!sessionID) {
      throw new Error("session.create returned no session id");
    }

    const subscription = await opencode.client.event.subscribe();
    const promptPromise = opencode.client.session.prompt({
      path: { id: sessionID },
      body: {
        model: {
          providerID: input.providerID,
          modelID: input.modelID,
        },
        parts: [{ type: "text", text: input.prompt }],
      },
    }).then((response) => {
      const promptError = extractPromptError(response);
      if (promptError) {
        promptState.error = promptError;
      }
    }).catch((error) => {
      promptState.error = extractErrorMessage(error);
    });

    const outcome = await waitForSessionOutcome(
      opencode.client,
      subscription.stream,
      sessionID,
      input.inactivityTimeoutSeconds ?? DEFAULT_INACTIVITY_TIMEOUT_SECONDS,
      promptState,
      deps,
    );

    if (outcome.success) {
      await promptPromise;
      if (promptState.error) {
        return {
          success: false,
          sessionID,
          error: promptState.error,
          durationMs: deps.now() - startedAt,
        };
      }
    }

    return {
      success: outcome.success,
      sessionID,
      error: outcome.error,
      completedBy: outcome.completedBy,
      durationMs: deps.now() - startedAt,
    };
  } catch (error) {
    return {
      success: false,
      sessionID,
      error: extractErrorMessage(error),
      durationMs: deps.now() - startedAt,
    };
  } finally {
    await closeServer(opencode?.server);
  }
}

async function main(argv: string[]): Promise<void> {
  const command = argv[0];

  if (command === "providers") {
    const output = await getProviders(readProvidersArgs(argv.slice(1)));
    process.stdout.write(JSON.stringify(output));
    return;
  }

  if (command === "run-eval") {
    const input = await readRunEvalInput();
    const output = await runEval(input);
    process.stdout.write(JSON.stringify(output));
    return;
  }

  throw new Error(`unknown command: ${command || "<empty>"}`);
}

if (import.meta.main) {
  main(process.argv.slice(2)).catch((error) => {
    console.error(extractErrorMessage(error));
    process.exit(1);
  });
}
