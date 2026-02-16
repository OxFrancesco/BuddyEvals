export type EvalResultFile = {
  prompt?: string;
  prompt_number?: number;
  model?: string;
  success?: boolean;
  error?: string;
  duration_seconds?: number;
  completed_at?: string;
  cost_usd?: number;
  cost?: number;
  total_cost?: number;
  [key: string]: unknown;
};

export type EvalRow = {
  folder: string;
  prompt: string;
  promptNumber: number | null;
  model: string;
  success: boolean;
  durationSeconds: number;
  completedAt: string;
  completedAtEpoch: number;
  costUsd: number | null;
  error: string;
  previewPath: string | null;
  scriptPath: string | null;
};

export type ReportData = {
  rows: EvalRow[];
  totalEvals: number;
  successfulEvals: number;
  failedEvals: number;
  successRate: number;
  totalDurationSeconds: number;
  averageDurationSeconds: number;
  totalKnownCostUsd: number;
  knownCostCount: number;
};
