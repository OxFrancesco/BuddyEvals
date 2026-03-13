import { mkdirSync, mkdtempSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { tmpdir } from "node:os";

import { expect, test } from "bun:test";

import { collectReportData } from "./report.ts";

test("collectReportData flags legacy broken runs and preserves project-server previews", async () => {
  const root = mkdtempSync(join(tmpdir(), "high-evals-dashboard-"));
  const evalsDir = join(root, "evals");
  mkdirSync(evalsDir, { recursive: true });

  const promptsPath = join(root, "prompts.json");
  writeFileSync(promptsPath, JSON.stringify([
    { id: "web-1", title: "Web 1", prompt: "Build a Bun page", track: "web", deterministic: true },
    { id: "web-2", title: "Web 2", prompt: "Build another Bun page", track: "web", deterministic: true },
    { id: "mobile-1", title: "Mobile 1", prompt: "Build an Expo app", track: "mobile", deterministic: false },
  ], null, 2));

  createEval(root, "run-vite", {
    "result.json": JSON.stringify({
      prompt: "Build a Bun page",
      model: "openrouter/glm-5",
      success: true,
      duration_seconds: 12,
      completed_at: "2026-03-13T10:00:00Z",
    }, null, 2),
    "index.html": `<html><body><script type="module" src="/src/main.tsx"></script></body></html>`,
    "package-lock.json": "{}",
    "vite.config.ts": "export default {};",
    "package.json": `{"devDependencies":{"@tailwindcss/vite":"^4.0.0"}}`,
  });

  createEval(root, "run-assets", {
    "result.json": JSON.stringify({
      prompt: "Build another Bun page",
      model: "openrouter/glm-5",
      success: true,
      duration_seconds: 8,
      completed_at: "2026-03-13T10:05:00Z",
    }, null, 2),
    ".run": "#!/bin/sh\nexit 0\n",
    "index.html": `<html><body><img src="./missing.png" /></body></html>`,
  });

  createEval(root, "run-bundled", {
    "result.json": JSON.stringify({
      prompt: "Build a Bun page",
      model: "openrouter/gpt-5",
      success: true,
      duration_seconds: 15,
      completed_at: "2026-03-13T10:10:00Z",
    }, null, 2),
    ".run": "#!/bin/sh\nexit 0\n",
    "index.html": `<html><body><script type="module" src="./frontend.tsx"></script></body></html>`,
    "frontend.tsx": `console.log("hello")`,
  });

  createEval(root, "run-expo", {
    "result.json": JSON.stringify({
      prompt: "Build an Expo app",
      model: "openrouter/glm-5",
      track: "mobile",
      success: true,
      duration_seconds: 22,
      completed_at: "2026-03-13T10:15:00Z",
    }, null, 2),
    "App.tsx": `export default function App() { return "Open up App.tsx to start working on your app!"; }`,
  });

  createEval(root, "run-missing-run", {
    "result.json": JSON.stringify({
      prompt: "Build another Bun page",
      model: "openrouter/gpt-5",
      success: true,
      duration_seconds: 6,
      completed_at: "2026-03-13T10:20:00Z",
    }, null, 2),
    "package.json": `{"name":"legacy-folder"}`,
  });

  const report = await collectReportData(evalsDir, promptsPath);
  const rowsByFolder = new Map(report.rows.map((row) => [row.folder, row]));

  expect(report.totalEvals).toBe(5);
  expect(report.headlineEvals).toBe(4);

  const viteRow = rowsByFolder.get("run-vite");
  expect(viteRow).toBeDefined();
  expect(viteRow?.legacy).toBe(true);
  expect(viteRow?.success).toBe(false);
  expect(viteRow?.violations).toContain("Forbidden toolchain file detected: package-lock.json");
  expect(viteRow?.violations).toContain("Forbidden Vite configuration detected");
  expect(viteRow?.violations).toContain("Forbidden Vite plugin references detected");
  expect(viteRow?.violations).toContain("Absolute /src browser imports are forbidden on Bun tracks");

  const assetRow = rowsByFolder.get("run-assets");
  expect(assetRow?.success).toBe(false);
  expect(assetRow?.runMode).toBe(".run");
  expect(assetRow?.violations.some((value) => value.includes("Missing local asset references"))).toBe(true);

  const bundledRow = rowsByFolder.get("run-bundled");
  expect(bundledRow?.success).toBe(true);
  expect(bundledRow?.previewMode).toBe("project_server");
  expect(bundledRow?.runMode).toBe(".run");

  const expoRow = rowsByFolder.get("run-expo");
  expect(expoRow?.success).toBe(false);
  expect(expoRow?.headlineEligible).toBe(false);
  expect(expoRow?.violations.some((value) => value.includes("Starter template content detected"))).toBe(true);

  const missingRunRow = rowsByFolder.get("run-missing-run");
  expect(missingRunRow?.success).toBe(false);
  expect(missingRunRow?.violations).toContain("Missing root .run contract");
});

function createEval(root: string, folder: string, files: Record<string, string>): void {
  const folderDir = join(root, "evals", folder);
  mkdirSync(folderDir, { recursive: true });

  for (const [relativePath, contents] of Object.entries(files)) {
    const fullPath = join(folderDir, relativePath);
    mkdirSync(dirname(fullPath), { recursive: true });
    writeFileSync(fullPath, contents);
  }
}
