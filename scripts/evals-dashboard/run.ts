import { readFile } from "node:fs/promises";
import { join, relative } from "node:path";

import {
  findMainPyDir,
  findRunnablePythonFiles,
  findScript,
  normalizeScriptTarget,
} from "./filesystem.ts";

export async function buildDotRunCommand(folderPath: string): Promise<string[]> {
  const runPath = join(folderPath, ".run");
  const shebang = await safeReadText(runPath);
  return shebang.startsWith("#!") && shebang.includes("bash")
    ? ["bash", ".run"]
    : ["sh", ".run"];
}

export async function findDotRunTarget(folderPath: string): Promise<string | null> {
  const runPath = join(folderPath, ".run");
  const file = Bun.file(runPath);
  return (await file.exists()) ? ".run" : null;
}

export async function findUvRunTargets(uvProjectDir: string): Promise<string[]> {
  const targets: string[] = [];
  const mainPyDir = await findMainPyDir(uvProjectDir);
  if (mainPyDir) {
    const mainTarget = normalizeScriptTarget(relative(uvProjectDir, join(mainPyDir, "main.py")));
    if (mainTarget && !targets.includes(mainTarget)) {
      targets.push(mainTarget);
    }
  }

  const runnableFiles = await findRunnablePythonFiles(uvProjectDir);
  for (const file of runnableFiles) {
    const normalized = normalizeScriptTarget(file);
    if (normalized && !targets.includes(normalized)) {
      targets.push(normalized);
    }
  }

  return targets;
}

export async function findNonUvRunTargets(folderPath: string): Promise<string[]> {
  const targets = await findRunnablePythonFiles(folderPath);
  if (targets.length > 0) {
    return targets;
  }

  const scriptPath = await findScript(folderPath);
  if (!scriptPath) {
    return [];
  }

  const relativePath = normalizeScriptTarget(relative(folderPath, scriptPath));
  return relativePath ? [relativePath] : [];
}

export function selectTargetOrDefault(requestedTarget: string | null, availableTargets: string[]): string | null {
  if (availableTargets.length === 0) {
    return null;
  }
  if (!requestedTarget) {
    return availableTargets[0] ?? null;
  }
  return availableTargets.includes(requestedTarget) ? requestedTarget : null;
}

export async function runDotRunAndCapture(
  cwd: string,
  env: Record<string, string> = {},
  timeoutMs = 15000,
): Promise<{ stdout: string; stderr: string; exitCode: number }> {
  return runAndCapture(await buildDotRunCommand(cwd), cwd, env, timeoutMs);
}

export async function runAndCapture(
  command: string[],
  cwd: string,
  env: Record<string, string> = {},
  timeoutMs = 0,
): Promise<{ stdout: string; stderr: string; exitCode: number }> {
  const proc = Bun.spawn(command, {
    cwd,
    env: {
      ...process.env,
      ...env,
    },
    stdout: "pipe",
    stderr: "pipe",
  });

  let timedOut = false;
  let timeoutID: ReturnType<typeof setTimeout> | null = null;
  if (timeoutMs > 0) {
    timeoutID = setTimeout(() => {
      timedOut = true;
      try {
        proc.kill();
      } catch {
        // Best-effort termination only.
      }
    }, timeoutMs);
  }

  const [stdout, stderr] = await Promise.all([
    new Response(proc.stdout).text(),
    new Response(proc.stderr).text(),
  ]);

  let exitCode = await proc.exited;
  if (timeoutID) {
    clearTimeout(timeoutID);
  }

  let stderrText = stderr;
  if (timedOut) {
    stderrText = `${stderrText}${stderrText ? "\n" : ""}[process timed out after ${timeoutMs}ms]`;
    if (exitCode === 0) {
      exitCode = 124;
    }
  }

  return { stdout, stderr: stderrText, exitCode };
}

export function formatCommandOutput(stdout: string, stderr: string): string {
  return (stdout + (stderr ? `\n--- stderr ---\n${stderr}` : "")).trim();
}

async function safeReadText(path: string): Promise<string> {
  try {
    return await readFile(path, "utf8");
  } catch {
    return "";
  }
}
