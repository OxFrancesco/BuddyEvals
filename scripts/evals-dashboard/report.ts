import { readdir } from "node:fs/promises";
import { join } from "node:path";

import { analyzeLegacyFolder, normalizeTrack, type DashboardPreviewMode, type DashboardRunMode } from "./contracts.ts";
import { findIndexHtml } from "./filesystem.ts";
import { parsePositiveInt, parsePositiveNumber } from "./parsing.ts";
import type { EvalResultFile, EvalRow, ReportData } from "./types.ts";

export async function collectReportData(evalsDir: string, promptsPath: string): Promise<ReportData> {
  const promptNumberByText = await loadPromptNumberLookup(promptsPath);
  const rows: EvalRow[] = [];

  let folderEntries: string[];
  try {
    folderEntries = await readdir(evalsDir);
  } catch {
    folderEntries = [];
  }

  for (const folder of folderEntries) {
    const resultPath = join(evalsDir, folder, "result.json");
    const promptPath = join(evalsDir, folder, "prompt.txt");

    const resultFile = Bun.file(resultPath);
    if (!(await resultFile.exists())) {
      continue;
    }

    const promptFile = Bun.file(promptPath);
    const promptText = (await promptFile.exists()) ? (await promptFile.text()) : "";

    let parsed: EvalResultFile;
    try {
      parsed = JSON.parse(await resultFile.text()) as EvalResultFile;
    } catch {
      continue;
    }

    const completedAt = typeof parsed.completed_at === "string" ? parsed.completed_at : "";
    const completedAtEpoch = completedAt ? Date.parse(completedAt) : Number.NaN;
    const prompt = typeof parsed.prompt === "string" && parsed.prompt.trim() !== "" ? parsed.prompt : promptText;
    const track = normalizeTrack(typeof parsed.track === "string" ? parsed.track : undefined, prompt);
    const promptNumber = inferPromptNumber(folder, parsed.prompt_number, parsed.prompt, promptText, promptNumberByText);
    const folderFullPath = join(evalsDir, folder);
    const legacy = typeof parsed.validation_success !== "boolean" || typeof parsed.agent_success !== "boolean";
    const legacyAnalysis = legacy ? await analyzeLegacyFolder(folderFullPath, parsed.track, prompt) : null;

    const previewMode = normalizePreviewMode(parsed.preview_mode) ?? legacyAnalysis?.previewMode ?? "none";
    const runMode = normalizeRunMode(parsed.run_mode) ?? legacyAnalysis?.runMode ?? "none";
    const violations = Array.isArray(parsed.violations)
      ? parsed.violations.filter((value): value is string => typeof value === "string" && value.trim() !== "")
      : legacyAnalysis?.violations ?? [];
    const checks = parsed.checks && typeof parsed.checks === "object"
      ? parsed.checks as Record<string, boolean>
      : {};

    const agentSuccess = typeof parsed.agent_success === "boolean" ? parsed.agent_success : parsed.success === true;
    const validationSuccess = typeof parsed.validation_success === "boolean"
      ? parsed.validation_success
      : violations.length === 0;
    const success = agentSuccess && validationSuccess;
    const previewPath = previewMode === "none"
      ? null
      : (await findIndexHtml(folderFullPath)) ? `/preview/${folder}/` : `/preview/${folder}/`;
    const headlineEligible = track !== "integration" && track !== "mobile";

    rows.push({
      folder,
      prompt,
      promptID: typeof parsed.prompt_id === "string" && parsed.prompt_id.trim() !== "" ? parsed.prompt_id : null,
      promptTitle: typeof parsed.prompt_title === "string" && parsed.prompt_title.trim() !== "" ? parsed.prompt_title : null,
      promptNumber,
      model: typeof parsed.model === "string" ? parsed.model : "unknown",
      track,
      success,
      agentSuccess,
      validationSuccess,
      durationSeconds: Number.isFinite(parsed.duration_seconds)
        ? Math.max(0, Number(parsed.duration_seconds))
        : 0,
      completedAt,
      completedAtEpoch,
      costUsd: extractCostUsd(parsed),
      error: typeof parsed.error === "string" ? parsed.error : "",
      previewMode,
      runMode,
      previewPath,
      violations,
      checks,
      legacy,
      headlineEligible,
    });
  }

  rows.sort((a, b) => {
    const aHasDate = Number.isFinite(a.completedAtEpoch);
    const bHasDate = Number.isFinite(b.completedAtEpoch);
    if (aHasDate && bHasDate) {
      return b.completedAtEpoch - a.completedAtEpoch;
    }
    if (aHasDate) {
      return -1;
    }
    if (bHasDate) {
      return 1;
    }
    return a.folder.localeCompare(b.folder);
  });

  const totalEvals = rows.length;
  const successfulEvals = rows.filter((row) => row.success).length;
  const failedEvals = totalEvals - successfulEvals;
  const agentSuccessfulEvals = rows.filter((row) => row.agentSuccess).length;
  const validatedEvals = rows.filter((row) => row.validationSuccess).length;
  const totalDurationSeconds = rows.reduce((sum, row) => sum + row.durationSeconds, 0);
  const averageDurationSeconds = totalEvals > 0 ? totalDurationSeconds / totalEvals : 0;

  const knownCostRows = rows.filter((row) => row.costUsd !== null);
  const totalKnownCostUsd = knownCostRows.reduce((sum, row) => sum + (row.costUsd ?? 0), 0);

  const headlineRows = rows.filter((row) => row.headlineEligible);
  const headlineSuccessfulEvals = headlineRows.filter((row) => row.success).length;

  return {
    rows,
    totalEvals,
    successfulEvals,
    failedEvals,
    agentSuccessfulEvals,
    validatedEvals,
    successRate: totalEvals > 0 ? (successfulEvals / totalEvals) * 100 : 0,
    validationRate: totalEvals > 0 ? (validatedEvals / totalEvals) * 100 : 0,
    headlineEvals: headlineRows.length,
    headlineSuccessfulEvals,
    headlineSuccessRate: headlineRows.length > 0 ? (headlineSuccessfulEvals / headlineRows.length) * 100 : 0,
    totalDurationSeconds,
    averageDurationSeconds,
    totalKnownCostUsd,
    knownCostCount: knownCostRows.length,
  };
}

async function loadPromptNumberLookup(promptsPath: string): Promise<Map<string, number>> {
  const lookup = new Map<string, number>();

  const file = Bun.file(promptsPath);
  if (!(await file.exists())) {
    return lookup;
  }

  let prompts: unknown;
  try {
    prompts = JSON.parse(await file.text());
  } catch {
    return lookup;
  }

  if (!Array.isArray(prompts)) {
    return lookup;
  }

  for (let i = 0; i < prompts.length; i += 1) {
    const prompt = prompts[i];
    if (typeof prompt === "string") {
      if (!lookup.has(prompt)) {
        lookup.set(prompt, i + 1);
      }
      continue;
    }

    if (prompt && typeof prompt === "object" && typeof (prompt as { prompt?: unknown }).prompt === "string") {
      const promptText = (prompt as { prompt: string }).prompt;
      if (!lookup.has(promptText)) {
        lookup.set(promptText, i + 1);
      }
    }
  }

  return lookup;
}

function inferPromptNumber(
  folder: string,
  direct: unknown,
  resultPrompt: unknown,
  promptText: string,
  promptNumberByText: Map<string, number>,
): number | null {
  const directNum = parsePositiveInt(direct);
  if (directNum !== null) {
    return directNum;
  }

  const folderMatch = folder.match(/(?:^|_)p(\d+)(?:_|$)/);
  const folderValue = folderMatch?.[1];
  if (folderValue) {
    const folderNum = Number.parseInt(folderValue, 10);
    if (Number.isInteger(folderNum) && folderNum > 0) {
      return folderNum;
    }
  }

  const prompt = typeof resultPrompt === "string" && resultPrompt.trim() !== "" ? resultPrompt : promptText;
  if (promptNumberByText.has(prompt)) {
    return promptNumberByText.get(prompt) ?? null;
  }

  return null;
}

function extractCostUsd(result: EvalResultFile): number | null {
  const directKeys = ["cost_usd", "total_cost", "cost"];
  for (const key of directKeys) {
    const value = parsePositiveNumber(result[key]);
    if (value !== null) {
      return value;
    }
  }

  const usage = result.usage;
  if (usage && typeof usage === "object") {
    const usageObj = usage as Record<string, unknown>;
    for (const key of ["cost_usd", "total_cost", "cost"]) {
      const value = parsePositiveNumber(usageObj[key]);
      if (value !== null) {
        return value;
      }
    }
  }

  return null;
}

function normalizePreviewMode(value: unknown): DashboardPreviewMode | null {
  if (value === "static" || value === "project_server" || value === "none") {
    return value;
  }
  return null;
}

function normalizeRunMode(value: unknown): DashboardRunMode | null {
  if (value === ".run" || value === "uv" || value === "legacy" || value === "none") {
    return value;
  }
  return null;
}
