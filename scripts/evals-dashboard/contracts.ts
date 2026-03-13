import { readdir, readFile } from "node:fs/promises";
import { dirname, join, relative } from "node:path";

export type DashboardTrack = "web" | "python" | "cli" | "integration" | "mobile";
export type DashboardPreviewMode = "static" | "project_server" | "none";
export type DashboardRunMode = ".run" | "uv" | "legacy" | "none";

export type LegacyAnalysis = {
  previewMode: DashboardPreviewMode;
  runMode: DashboardRunMode;
  violations: string[];
};

const SKIP_DIRS = new Set([".git", "node_modules", ".venv", "__pycache__", ".ruff_cache", "dist", "build"]);
const PLACEHOLDER_RE = /(<[A-Z][A-Z0-9_]+>)|(\{\{[A-Z][A-Z0-9_]+\}\})/;
const SCRIPT_REF_RE = /<script[^>]+src=["']([^"']+)["']/gi;
const LINK_REF_RE = /<link[^>]+href=["']([^"']+)["']/gi;
const IMG_REF_RE = /<(?:img|source)[^>]+src=["']([^"']+)["']/gi;
const CSS_URL_RE = /url\(([^)]+)\)/gi;
const IMPORT_RE = /import\s+(?:[^"'\n]+?\s+from\s+)?["']([^"']+)["']|import\(["']([^"']+)["']\)/g;

export async function analyzeLegacyFolder(folderDir: string, track: string | undefined, prompt: string): Promise<LegacyAnalysis> {
  const normalizedTrack = normalizeTrack(track, prompt);
  const hasRun = await exists(join(folderDir, ".run"));
  const missingRefs = await collectMissingLocalReferences(folderDir);
  const placeholderLeaks = await collectPlaceholderLeaks(folderDir);
  const forbiddenTooling = await collectForbiddenTooling(folderDir, normalizedTrack);
  const starterTemplates = await collectStarterTemplates(folderDir);
  const nestedProjects = await collectNestedProjects(folderDir, hasRun || (normalizedTrack === "mobile" && await exists(join(folderDir, "README.md"))));

  const violations = [
    ...(normalizedTrack === "mobile" || hasRun ? [] : ["Missing root .run contract"]),
    ...(missingRefs.length > 0 ? [`Missing local asset references: ${missingRefs.join(", ")}`] : []),
    ...(placeholderLeaks.length > 0 ? [`Unresolved placeholders found in: ${placeholderLeaks.join(", ")}`] : []),
    ...forbiddenTooling,
    ...starterTemplates,
    ...nestedProjects,
  ];

  return {
    previewMode: await detectPreviewMode(folderDir),
    runMode: await detectRunMode(folderDir, normalizedTrack),
    violations: uniqueSorted(violations),
  };
}

export function normalizeTrack(track: string | undefined, prompt: string): DashboardTrack {
  const value = (track ?? "").trim().toLowerCase();
  if (value === "web" || value === "python" || value === "cli" || value === "integration" || value === "mobile") {
    return value;
  }

  const lower = prompt.toLowerCase();
  if (lower.includes("expo") || lower.includes("react native") || lower.includes("mobile")) {
    return "mobile";
  }
  if (lower.includes("uv init") || lower.includes("python") || lower.includes("manim")) {
    return "python";
  }
  if (lower.includes("cli tool")) {
    return "cli";
  }
  if (lower.includes("notion") || lower.includes("airtable") || lower.includes("linear") || lower.includes("reference post") || lower.includes("github commits")) {
    return "integration";
  }
  return "web";
}

export async function detectPreviewMode(folderDir: string): Promise<DashboardPreviewMode> {
  if (!await exists(join(folderDir, "index.html"))) {
    return "none";
  }
  if (await hasProjectServerSignals(folderDir)) {
    return "project_server";
  }
  return "static";
}

export async function detectRunMode(folderDir: string, track: DashboardTrack): Promise<DashboardRunMode> {
  if (await exists(join(folderDir, ".run"))) {
    return ".run";
  }
  if (track === "python" && (await exists(join(folderDir, "pyproject.toml")) || await folderContainsExtension(folderDir, ".py"))) {
    return "uv";
  }
  if (await exists(join(folderDir, "index.html")) || await exists(join(folderDir, "package.json"))) {
    return "legacy";
  }
  return "none";
}

async function hasProjectServerSignals(folderDir: string): Promise<boolean> {
  if (await fileContains(join(folderDir, "index.ts"), "Bun.serve(") || await fileContains(join(folderDir, "server.ts"), "Bun.serve(")) {
    return true;
  }

  const indexPath = join(folderDir, "index.html");
  if (!await exists(indexPath)) {
    return false;
  }

  const html = await readFile(indexPath, "utf8");
  if (/<script[^>]+src=["'][^"']+\.(ts|tsx|jsx)["']/.test(html)) {
    return true;
  }
  if (html.includes('fetch("/api/') || html.includes("fetch('/api/")) {
    return true;
  }

  return folderContainsContent(folderDir, [".js", ".jsx", ".ts", ".tsx"], ['fetch("/api/', "fetch('/api/", "Bun.serve("]);
}

async function collectMissingLocalReferences(folderDir: string): Promise<string[]> {
  const files = await walkFiles(folderDir);
  const missing = new Set<string>();

  for (const file of files) {
    const refs = await collectFileReferences(file);
    if (refs.length === 0) continue;

    for (const ref of refs) {
      const target = resolveReferenceTarget(folderDir, dirname(file), ref);
      if (!target || await exists(target)) continue;
      missing.add(`${toRelative(folderDir, file)} -> ${ref}`);
    }
  }

  return [...missing].sort();
}

async function collectFileReferences(filePath: string): Promise<string[]> {
  const ext = filePath.slice(filePath.lastIndexOf(".")).toLowerCase();
  const content = await readFile(filePath, "utf8");

  if (ext === ".html") {
    return [
      ...collectMatches(content, SCRIPT_REF_RE, true),
      ...collectMatches(content, LINK_REF_RE, true),
      ...collectMatches(content, IMG_REF_RE, true),
    ];
  }
  if (ext === ".css") {
    return collectMatches(content, CSS_URL_RE, true);
  }
  if ([".js", ".jsx", ".ts", ".tsx", ".mjs", ".cjs"].includes(ext)) {
    return collectMatches(content, IMPORT_RE, false);
  }
  return [];
}

function collectMatches(content: string, pattern: RegExp, allowBare: boolean): string[] {
  const refs: string[] = [];
  pattern.lastIndex = 0;
  let match: RegExpExecArray | null;
  while ((match = pattern.exec(content)) !== null) {
    const value = sanitizeRef(match[1] ?? match[2] ?? "");
    if (isLocalRef(value, allowBare)) {
      refs.push(value);
    }
  }
  return refs;
}

async function collectPlaceholderLeaks(folderDir: string): Promise<string[]> {
  const leaks: string[] = [];
  for (const file of await walkFiles(folderDir)) {
    if (!isTextValidationFile(file)) continue;
    const content = await readFile(file, "utf8");
    if (PLACEHOLDER_RE.test(content)) {
      leaks.push(toRelative(folderDir, file));
    }
  }
  return leaks.sort();
}

async function collectForbiddenTooling(folderDir: string, track: DashboardTrack): Promise<string[]> {
  const violations = new Set<string>();
  for (const fileName of ["package-lock.json", "pnpm-lock.yaml", "yarn.lock"]) {
    if (await folderContainsFile(folderDir, fileName)) {
      violations.add(`Forbidden toolchain file detected: ${fileName}`);
    }
  }
  for (const fileName of ["vite.config.ts", "vite.config.js", "vite.config.mjs", "vite.config.cjs"]) {
    if (await folderContainsFile(folderDir, fileName)) {
      violations.add("Forbidden Vite configuration detected");
      break;
    }
  }

  if (track === "web" || track === "cli" || track === "integration") {
    if (await folderContainsContent(folderDir, [".js", ".jsx", ".ts", ".tsx", ".json", ".html", ".css"], ["@vitejs/plugin-react", "@tailwindcss/vite"])) {
      violations.add("Forbidden Vite plugin references detected");
    }
    if (await folderContainsContent(folderDir, [".html"], ["/src/"])) {
      violations.add("Absolute /src browser imports are forbidden on Bun tracks");
    }
  }

  return [...violations].sort();
}

async function collectStarterTemplates(folderDir: string): Promise<string[]> {
  const needles = [
    "Open up App.tsx to start working on your app!",
    "Open up app/index.tsx to start working on your app!",
  ];

  const violations = new Set<string>();
  for (const file of await walkFiles(folderDir)) {
    if (!isTextValidationFile(file)) continue;
    const content = await readFile(file, "utf8");
    if (needles.some((needle) => content.includes(needle))) {
      violations.add(`Starter template content detected in ${toRelative(folderDir, file)}`);
    }
  }
  return [...violations].sort();
}

async function collectNestedProjects(folderDir: string, hasRootEntryContract: boolean): Promise<string[]> {
  if (hasRootEntryContract) {
    return [];
  }

  const projectFiles = new Set(["package.json", "pyproject.toml", "app.json", "Cargo.toml", "go.mod"]);
  const nested: string[] = [];
  for (const file of await walkFiles(folderDir)) {
    const name = file.slice(file.lastIndexOf("/") + 1);
    if (!projectFiles.has(name)) continue;
    if (dirname(file) === folderDir) continue;
    nested.push(toRelative(folderDir, file));
  }

  if (nested.length === 0) {
    return [];
  }

  return [`Nested project detected without a root entry contract: ${uniqueSorted(nested).join(", ")}`];
}

async function walkFiles(rootDir: string, currentDir = rootDir): Promise<string[]> {
  let entries: Array<{ name: string; isDirectory(): boolean }> = [];
  try {
    entries = await readdir(currentDir, { withFileTypes: true, encoding: "utf8" });
  } catch {
    return [];
  }

  const files: string[] = [];
  for (const entry of entries) {
    const entryName = entry.name;
    if (entry.isDirectory()) {
      if (SKIP_DIRS.has(entryName)) continue;
      files.push(...await walkFiles(rootDir, join(currentDir, entryName)));
      continue;
    }
    files.push(join(currentDir, entryName));
  }
  return files;
}

async function folderContainsFile(rootDir: string, fileName: string): Promise<boolean> {
  for (const file of await walkFiles(rootDir)) {
    if (file.endsWith(`/${fileName}`) || file === join(rootDir, fileName)) {
      return true;
    }
  }
  return false;
}

async function folderContainsExtension(rootDir: string, ext: string): Promise<boolean> {
  for (const file of await walkFiles(rootDir)) {
    if (file.toLowerCase().endsWith(ext.toLowerCase())) {
      return true;
    }
  }
  return false;
}

async function folderContainsContent(rootDir: string, extensions: string[], needles: string[]): Promise<boolean> {
  const allowed = new Set(extensions.map((ext) => ext.toLowerCase()));
  for (const file of await walkFiles(rootDir)) {
    const ext = file.slice(file.lastIndexOf(".")).toLowerCase();
    if (!allowed.has(ext)) continue;
    const content = await readFile(file, "utf8");
    if (needles.some((needle) => content.includes(needle))) {
      return true;
    }
  }
  return false;
}

async function fileContains(path: string, needle: string): Promise<boolean> {
  try {
    const content = await readFile(path, "utf8");
    return content.includes(needle);
  } catch {
    return false;
  }
}

async function exists(path: string): Promise<boolean> {
  try {
    await readFile(path);
    return true;
  } catch {
    return false;
  }
}

function sanitizeRef(value: string): string {
  const [withoutHash = ""] = value.trim().replace(/^['"]|['"]$/g, "").split("#");
  const [withoutQuery = ""] = withoutHash.split("?");
  return withoutQuery;
}

function isLocalRef(ref: string, allowBare: boolean): boolean {
  if (!ref) return false;
  const lower = ref.toLowerCase();
  if (lower.startsWith("http://") || lower.startsWith("https://") || lower.startsWith("data:") || lower.startsWith("mailto:") || lower.startsWith("tel:") || lower.startsWith("javascript:") || lower.startsWith("#")) {
    return false;
  }
  if (ref.startsWith("/") || ref.startsWith("./") || ref.startsWith("../")) {
    return true;
  }
  return allowBare;
}

function resolveReferenceTarget(rootDir: string, baseDir: string, ref: string): string | null {
  if (!ref) return null;
  if (ref.startsWith("/")) {
    return join(rootDir, ref.slice(1));
  }
  return join(baseDir, ref);
}

function isTextValidationFile(path: string): boolean {
  const base = path.slice(path.lastIndexOf("/") + 1);
  if (base === ".run" || base === "README" || base.startsWith("README.")) {
    return true;
  }

  const ext = path.slice(path.lastIndexOf(".")).toLowerCase();
  return [".txt", ".md", ".json", ".html", ".css", ".js", ".jsx", ".ts", ".tsx", ".py", ".sh", ".toml", ".yaml", ".yml"].includes(ext);
}

function toRelative(rootDir: string, path: string): string {
  return relative(rootDir, path).replaceAll("\\", "/");
}

function uniqueSorted(values: string[]): string[] {
  return [...new Set(values.filter((value) => value.trim() !== ""))].sort();
}
