import { join } from "node:path";

import type { DashboardPreviewMode } from "./contracts.ts";
import { findIndexHtml, findUvProjectDir, normalizeScriptTarget } from "./filesystem.ts";
import { collectReportData } from "./report.ts";
import { renderDashboard } from "./render.ts";
import {
  buildDotRunCommand,
  findDotRunTarget,
  findNonUvRunTargets,
  findUvRunTargets,
  formatCommandOutput,
  runAndCapture,
  runDotRunAndCapture,
  selectTargetOrDefault,
} from "./run.ts";

export type DashboardServerConfig = {
  host: string;
  port: number;
  evalsDir: string;
  promptsPath: string;
};

type DashboardRow = Awaited<ReturnType<typeof collectReportData>>["rows"][number];

type ProjectServerSession = {
  folder: string;
  port: number;
  process: Bun.Subprocess;
  startedAt: number;
  lastUsedAt: number;
};

const PROJECT_SERVER_STARTUP_RETRIES = 40;
const PROJECT_SERVER_STARTUP_DELAY_MS = 250;

export function createDashboardServer(config: DashboardServerConfig): Bun.Server<unknown> {
  const { evalsDir, host, port, promptsPath } = config;
  const sessions = new Map<string, ProjectServerSession>();
  let nextProjectServerPort = Math.max(port + 1, 4600);

  return Bun.serve({
    hostname: host,
    port,
    fetch: async (request: Request): Promise<Response> => {
      const url = new URL(request.url);

      if (url.pathname === "/api/evals") {
        const report = await collectReportData(evalsDir, promptsPath);
        return Response.json(report);
      }

      const runOptionsMatch = url.pathname.match(/^\/api\/run-options\/([^/]+)\/?$/);
      if (runOptionsMatch) {
        const folder = runOptionsMatch[1];
        if (!folder) {
          return new Response("Not found", { status: 404 });
        }
        const folderDir = resolveEvalFolder(evalsDir, folder);
        if (!folderDir) {
          return new Response("Forbidden", { status: 403 });
        }

        const row = await findDashboardRow(evalsDir, promptsPath, folder);
        const dotRunTarget = await findDotRunTarget(folderDir);
        if (dotRunTarget) {
          return Response.json({
            ok: true,
            mode: ".run",
            previewMode: row?.previewMode ?? "none",
            defaultTarget: dotRunTarget,
            targets: [dotRunTarget],
          });
        }

        const uvProjectDir = await findUvProjectDir(folderDir);
        if (uvProjectDir) {
          const uvTargets = await findUvRunTargets(uvProjectDir);
          return Response.json({
            ok: uvTargets.length > 0,
            mode: "uv",
            previewMode: row?.previewMode ?? "none",
            defaultTarget: uvTargets.length > 0 ? uvTargets[0] : null,
            targets: uvTargets,
          });
        }

        const pythonTargets = await findNonUvRunTargets(folderDir);
        return Response.json({
          ok: pythonTargets.length > 0,
          mode: pythonTargets.length > 0 ? "legacy" : "none",
          previewMode: row?.previewMode ?? "none",
          defaultTarget: pythonTargets.length > 0 ? pythonTargets[0] : null,
          targets: pythonTargets,
        });
      }

      const runMatch = url.pathname.match(/^\/run\/([^/]+)\/?$/);
      if (runMatch) {
        const folder = runMatch[1];
        if (!folder) {
          return new Response("Not found", { status: 404 });
        }
        const folderDir = resolveEvalFolder(evalsDir, folder);
        if (!folderDir) {
          return new Response("Forbidden", { status: 403 });
        }

        const requestedTarget = normalizeScriptTarget(url.searchParams.get("target"));
        const row = await findDashboardRow(evalsDir, promptsPath, folder);
        try {
          const dotRunTarget = await findDotRunTarget(folderDir);
          if (dotRunTarget) {
            if ((row?.previewMode ?? "none") === "project_server") {
              const session = await ensureProjectServerSession(folder, folderDir);
              return Response.json({
                ok: true,
                output: [
                  `[.run] launched as persistent project server`,
                  `PORT=${session.port}`,
                  `Preview: /preview/${folder}/`,
                ].join("\n"),
              });
            }

            const procResult = await runDotRunAndCapture(folderDir, {
              PORT: String(allocateProjectServerPort()),
            });
            const output = formatCommandOutput(procResult.stdout, procResult.stderr);
            return Response.json({
              ok: procResult.exitCode === 0,
              output: `[.run]\n${output || "(no output)"}`,
            });
          }

          const uvProjectDir = await findUvProjectDir(folderDir);
          if (uvProjectDir) {
            const uvTargets = await findUvRunTargets(uvProjectDir);
            if (uvTargets.length === 0) {
              return Response.json({ ok: false, output: "UV project found, but no runnable .py files were discovered." }, { status: 404 });
            }

            const selectedTarget = selectTargetOrDefault(requestedTarget, uvTargets);
            if (!selectedTarget) {
              return Response.json({
                ok: false,
                output: formatMissingTargetOutput(requestedTarget, uvTargets),
              }, { status: 400 });
            }

            const syncResult = await runAndCapture(["uv", "sync"], uvProjectDir);
            const syncOutput = formatCommandOutput(syncResult.stdout, syncResult.stderr);
            if (syncResult.exitCode !== 0) {
              return Response.json({ ok: false, output: `[uv sync]\n${syncOutput || "(no output)"}` });
            }

            const runResult = await runAndCapture(["uv", "run", selectedTarget], uvProjectDir);
            const runOutput = formatCommandOutput(runResult.stdout, runResult.stderr);
            return Response.json({
              ok: runResult.exitCode === 0,
              output: `[uv sync]\n${syncOutput || "(no output)"}\n\n[uv run ${selectedTarget}]\n${runOutput || "(no output)"}`,
            });
          }

          const pythonTargets = await findNonUvRunTargets(folderDir);
          if (pythonTargets.length === 0) {
            return Response.json({ ok: false, output: "No runnable target found." }, { status: 404 });
          }

          const selectedTarget = selectTargetOrDefault(requestedTarget, pythonTargets);
          if (!selectedTarget) {
            return Response.json({
              ok: false,
              output: formatMissingTargetOutput(requestedTarget, pythonTargets),
            }, { status: 400 });
          }

          const procResult = await runAndCapture(["uv", "run", selectedTarget], folderDir);
          const output = formatCommandOutput(procResult.stdout, procResult.stderr);
          return Response.json({
            ok: procResult.exitCode === 0,
            output: `[uv run ${selectedTarget}]\n${output || "(no output)"}`,
          });
        } catch (err: unknown) {
          const msg = err instanceof Error ? err.message : String(err);
          return Response.json({ ok: false, output: `Failed to run: ${msg}` }, { status: 500 });
        }
      }

      const previewMatch = url.pathname.match(/^\/preview\/([^/]+)(\/.*)?$/);
      if (previewMatch) {
        const folder = previewMatch[1];
        if (!folder) {
          return new Response("Not found", { status: 404 });
        }
        const rest = previewMatch[2] ?? "/";
        const folderDir = resolveEvalFolder(evalsDir, folder);
        if (!folderDir || rest.includes("..")) {
          return new Response("Forbidden", { status: 403 });
        }

        const row = await findDashboardRow(evalsDir, promptsPath, folder);
        const previewMode: DashboardPreviewMode = row?.previewMode ?? "none";
        if (previewMode === "project_server") {
          try {
            const session = await ensureProjectServerSession(folder, folderDir);
            return proxyProjectServerRequest(request, session.port, rest, url.search);
          } catch (err: unknown) {
            const msg = err instanceof Error ? err.message : String(err);
            return new Response(`Preview unavailable: ${msg}`, { status: 502 });
          }
        }

        if (previewMode === "none") {
          return new Response("No preview available", { status: 404 });
        }

        return serveStaticPreview(folder, folderDir, rest, request);
      }

      if (url.pathname === "/") {
        const report = await collectReportData(evalsDir, promptsPath);
        return new Response(renderDashboard(report), {
          headers: {
            "content-type": "text/html; charset=utf-8",
            "cache-control": "no-store",
          },
        });
      }

      return new Response("Not found", { status: 404 });
    },
  });

  function allocateProjectServerPort(): number {
    const portToUse = nextProjectServerPort;
    nextProjectServerPort += 1;
    return portToUse;
  }

  async function ensureProjectServerSession(folder: string, folderDir: string): Promise<ProjectServerSession> {
    const existing = sessions.get(folder);
    if (existing && await projectServerIsReachable(existing.port)) {
      existing.lastUsedAt = Date.now();
      return existing;
    }
    if (existing) {
      terminateSession(existing);
      sessions.delete(folder);
    }

    if (!await findDotRunTarget(folderDir)) {
      throw new Error("project-server preview requires a root .run file");
    }

    const allocatedPort = allocateProjectServerPort();
    const subprocess = Bun.spawn(await buildDotRunCommand(folderDir), {
      cwd: folderDir,
      env: {
        ...process.env,
        PORT: String(allocatedPort),
      },
      stdout: "ignore",
      stderr: "ignore",
    });

    const session: ProjectServerSession = {
      folder,
      port: allocatedPort,
      process: subprocess,
      startedAt: Date.now(),
      lastUsedAt: Date.now(),
    };
    sessions.set(folder, session);

    const ready = await waitForProjectServerReady(allocatedPort, subprocess);
    if (!ready) {
      terminateSession(session);
      sessions.delete(folder);
      throw new Error("the .run command did not start a reachable server on 127.0.0.1:$PORT");
    }

    subprocess.exited.then(() => {
      const active = sessions.get(folder);
      if (active?.process === subprocess) {
        sessions.delete(folder);
      }
    }).catch(() => {
      const active = sessions.get(folder);
      if (active?.process === subprocess) {
        sessions.delete(folder);
      }
    });

    return session;
  }
}

async function findDashboardRow(
  evalsDir: string,
  promptsPath: string,
  folder: string,
): Promise<DashboardRow | null> {
  const report = await collectReportData(evalsDir, promptsPath);
  return report.rows.find((row) => row.folder === folder) ?? null;
}

function resolveEvalFolder(evalsDir: string, folder: string): string | null {
  if (folder.includes("..")) {
    return null;
  }
  return join(evalsDir, folder);
}

async function serveStaticPreview(folder: string, folderDir: string, rest: string, request: Request): Promise<Response> {
  if (rest === "/") {
    const indexPath = await findIndexHtml(folderDir);
    if (!indexPath) {
      return new Response("No index.html found", { status: 404 });
    }
    if (indexPath !== "index.html") {
      const redirectURL = new URL(request.url);
      redirectURL.pathname = `/preview/${folder}/${indexPath.replace(/index\.html$/i, "")}`;
      return Response.redirect(redirectURL.toString());
    }
    return new Response(Bun.file(join(folderDir, indexPath)));
  }

  const normalized = rest.slice(1);
  const candidatePaths = normalized.endsWith("/")
    ? [join(folderDir, normalized, "index.html")]
    : [join(folderDir, normalized), join(folderDir, normalized, "index.html")];

  for (const candidate of candidatePaths) {
    const file = Bun.file(candidate);
    if (await file.exists()) {
      return new Response(file);
    }
  }

  return new Response("Not found", { status: 404 });
}

async function proxyProjectServerRequest(
  request: Request,
  port: number,
  rest: string,
  search: string,
): Promise<Response> {
  const upstreamURL = new URL(`http://127.0.0.1:${port}${rest}${search}`);
  const response = await fetch(upstreamURL, {
    method: request.method,
    headers: request.headers,
    redirect: "manual",
  });

  const headers = new Headers(response.headers);
  headers.set("cache-control", "no-store");

  return new Response(response.body, {
    status: response.status,
    headers,
  });
}

async function projectServerIsReachable(port: number): Promise<boolean> {
  try {
    const response = await Promise.race([
      fetch(`http://127.0.0.1:${port}/`, {
        method: "GET",
        redirect: "manual",
      }),
      Bun.sleep(500).then(() => null),
    ]);
    if (!response) {
      return false;
    }
    return response.status > 0;
  } catch {
    return false;
  }
}

async function waitForProjectServerReady(port: number, process: Bun.Subprocess): Promise<boolean> {
  for (let attempt = 0; attempt < PROJECT_SERVER_STARTUP_RETRIES; attempt += 1) {
    if (await projectServerIsReachable(port)) {
      return true;
    }

    const exited = await Promise.race([
      process.exited.then(() => true).catch(() => true),
      Bun.sleep(PROJECT_SERVER_STARTUP_DELAY_MS).then(() => false),
    ]);
    if (exited) {
      return false;
    }
  }

  return projectServerIsReachable(port);
}

function terminateSession(session: ProjectServerSession): void {
  try {
    session.process.kill();
  } catch {
    // Best-effort cleanup only.
  }
}

function formatMissingTargetOutput(requestedTarget: string | null, targets: string[]): string {
  return [
    `Requested target "${requestedTarget}" is not available.`,
    "",
    "Available targets:",
    targets.map((target) => `- ${target}`).join("\n"),
  ].join("\n");
}
