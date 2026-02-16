import { readdir } from "node:fs/promises";
import { join } from "node:path";

import { findIndexHtml, findScript } from "./filesystem.ts";
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

    const promptNumber = inferPromptNumber(folder, parsed.prompt_number, parsed.prompt, promptText, promptNumberByText);

    const folderFullPath = join(evalsDir, folder);
    const previewPath = await findIndexHtml(folderFullPath);
    const scriptPath = await findScript(folderFullPath);

    rows.push({
      folder,
      prompt: typeof parsed.prompt === "string" && parsed.prompt.trim() !== "" ? parsed.prompt : promptText,
      promptNumber,
      model: typeof parsed.model === "string" ? parsed.model : "unknown",
      success: parsed.success === true,
      durationSeconds: Number.isFinite(parsed.duration_seconds)
        ? Math.max(0, Number(parsed.duration_seconds))
        : 0,
      completedAt,
      completedAtEpoch,
      costUsd: extractCostUsd(parsed),
      error: typeof parsed.error === "string" ? parsed.error : "",
      previewPath,
      scriptPath,
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
  const totalDurationSeconds = rows.reduce((sum, row) => sum + row.durationSeconds, 0);
  const averageDurationSeconds = totalEvals > 0 ? totalDurationSeconds / totalEvals : 0;

  const knownCostRows = rows.filter((row) => row.costUsd !== null);
  const totalKnownCostUsd = knownCostRows.reduce((sum, row) => sum + (row.costUsd ?? 0), 0);

  return {
    rows,
    totalEvals,
    successfulEvals,
    failedEvals,
    successRate: totalEvals > 0 ? (successfulEvals / totalEvals) * 100 : 0,
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
    if (typeof prompt !== "string") {
      continue;
    }
    if (!lookup.has(prompt)) {
      lookup.set(prompt, i + 1);
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
  if (folderMatch) {
    const folderNum = Number.parseInt(folderMatch[1], 10);
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
