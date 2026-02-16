#!/usr/bin/env bun
import { join } from "node:path";

import { openBrowser, readPortArg, readStringArg } from "./evals-dashboard/cli.ts";
import { createDashboardServer } from "./evals-dashboard/server.ts";

const args = process.argv.slice(2);
const noOpen = args.includes("--no-open");
const host = readStringArg(args, "--host") ?? "127.0.0.1";
const port = readPortArg(args, "--port") ?? 3888;

const workspaceRoot = process.cwd();
const evalsDir = join(workspaceRoot, "evals");
const promptsPath = join(workspaceRoot, "prompts.json");

const server = createDashboardServer({
  host,
  port,
  evalsDir,
  promptsPath,
});

const dashboardURL = `http://${server.hostname}:${server.port}`;
console.log(`Evals dashboard running at ${dashboardURL}`);
if (!noOpen) {
  openBrowser(dashboardURL);
}
