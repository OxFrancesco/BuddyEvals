import { join, relative } from "node:path";

import {
  findMainPyDir,
  findRunnablePythonFiles,
  findScript,
  normalizeScriptTarget,
} from "./filesystem.ts";

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
    return availableTargets[0];
  }
  return availableTargets.includes(requestedTarget) ? requestedTarget : null;
}

export async function runAndCapture(command: string[], cwd: string): Promise<{ stdout: string; stderr: string; exitCode: number }> {
  const proc = Bun.spawn(command, {
    cwd,
    stdout: "pipe",
    stderr: "pipe",
  });
  const [stdout, stderr] = await Promise.all([
    new Response(proc.stdout).text(),
    new Response(proc.stderr).text(),
  ]);
  const exitCode = await proc.exited;
  return { stdout, stderr, exitCode };
}

export function formatCommandOutput(stdout: string, stderr: string): string {
  return (stdout + (stderr ? `\n--- stderr ---\n${stderr}` : "")).trim();
}
