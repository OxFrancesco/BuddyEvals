import { join } from "node:path";

import { findIndexHtml, findUvProjectDir, normalizeScriptTarget } from "./filesystem.ts";
import { collectReportData } from "./report.ts";
import { renderDashboard } from "./render.ts";
import {
  findNonUvRunTargets,
  findUvRunTargets,
  formatCommandOutput,
  runAndCapture,
  selectTargetOrDefault,
} from "./run.ts";

export type DashboardServerConfig = {
  host: string;
  port: number;
  evalsDir: string;
  promptsPath: string;
};

export function createDashboardServer(config: DashboardServerConfig): Bun.Server {
  const { evalsDir, host, port, promptsPath } = config;

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
        if (folder.includes("..")) {
          return new Response("Forbidden", { status: 403 });
        }

        const folderDir = join(evalsDir, folder);
        const uvProjectDir = await findUvProjectDir(folderDir);
        if (uvProjectDir) {
          const uvTargets = await findUvRunTargets(uvProjectDir);
          return Response.json({
            ok: uvTargets.length > 0,
            mode: "uv",
            defaultTarget: uvTargets.length > 0 ? uvTargets[0] : null,
            targets: uvTargets,
          });
        }

        const pythonTargets = await findNonUvRunTargets(folderDir);
        return Response.json({
          ok: pythonTargets.length > 0,
          mode: "python",
          defaultTarget: pythonTargets.length > 0 ? pythonTargets[0] : null,
          targets: pythonTargets,
        });
      }

      const runMatch = url.pathname.match(/^\/run\/([^/]+)\/?$/);
      if (runMatch) {
        const folder = runMatch[1];
        if (folder.includes("..")) {
          return new Response("Forbidden", { status: 403 });
        }

        const folderDir = join(evalsDir, folder);
        const requestedTarget = normalizeScriptTarget(url.searchParams.get("target"));
        try {
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
            return Response.json({ ok: false, output: "No .py script found in folder." }, { status: 404 });
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
        const rest = previewMatch[2] ?? "/";

        if (folder.includes("..") || rest.includes("..")) {
          return new Response("Forbidden", { status: 403 });
        }

        const folderDir = join(evalsDir, folder);
        if (rest === "/") {
          const indexPath = await findIndexHtml(folderDir);
          if (indexPath) {
            return new Response(Bun.file(join(folderDir, indexPath)));
          }
          return new Response("No index.html found", { status: 404 });
        }

        const file = Bun.file(join(folderDir, rest.slice(1)));
        if (await file.exists()) {
          return new Response(file);
        }
        return new Response("Not found", { status: 404 });
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
}

function formatMissingTargetOutput(requestedTarget: string | null, targets: string[]): string {
  return [
    `Requested target "${requestedTarget}" is not available.`,
    "",
    "Available targets:",
    targets.map((target) => `- ${target}`).join("\n"),
  ].join("\n");
}
