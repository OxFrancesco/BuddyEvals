import { readdir } from "node:fs/promises";
import { join } from "node:path";

export async function findIndexHtml(folderPath: string, maxDepth = 3): Promise<string | null> {
  const rootFile = Bun.file(join(folderPath, "index.html"));
  if (await rootFile.exists()) {
    return "index.html";
  }

  async function search(dir: string, depth: number): Promise<string | null> {
    if (depth >= maxDepth) return null;
    let entries: string[];
    try {
      entries = await readdir(dir, { withFileTypes: false }) as unknown as string[];
    } catch {
      return null;
    }
    for (const entry of entries) {
      if (entry === "node_modules" || entry === ".git") continue;
      const full = join(dir, entry);
      const stat = Bun.file(full);
      if (entry === "index.html" && (await stat.exists())) {
        return full.slice(folderPath.length + 1);
      }
    }
    for (const entry of entries) {
      if (entry === "node_modules" || entry === ".git") continue;
      const full = join(dir, entry);
      try {
        const dirEntries = await readdir(full);
        if (dirEntries) {
          const result = await search(full, depth + 1);
          if (result) return result;
        }
      } catch {
        // not a directory
      }
    }
    return null;
  }

  return search(folderPath, 0);
}

export async function findScript(folderPath: string, maxDepth = 3): Promise<string | null> {
  async function search(dir: string, depth: number): Promise<string | null> {
    if (depth >= maxDepth) return null;
    let entries: string[];
    try {
      entries = await readdir(dir, { withFileTypes: false }) as unknown as string[];
    } catch {
      return null;
    }
    for (const entry of entries) {
      if (entry === "node_modules" || entry === ".git" || entry === "__pycache__" || entry === ".venv") continue;
      if (entry.endsWith(".py")) {
        return join(dir, entry);
      }
    }
    for (const entry of entries) {
      if (entry === "node_modules" || entry === ".git" || entry === "__pycache__" || entry === ".venv") continue;
      const full = join(dir, entry);
      try {
        const dirEntries = await readdir(full);
        if (dirEntries) {
          const result = await search(full, depth + 1);
          if (result) return result;
        }
      } catch {
        // not a directory
      }
    }
    return null;
  }

  return search(folderPath, 0);
}

export async function findUvProjectDir(folderPath: string, maxDepth = 3): Promise<string | null> {
  async function hasUvProject(dir: string): Promise<boolean> {
    const pyproject = Bun.file(join(dir, "pyproject.toml"));
    return await pyproject.exists();
  }

  if (await hasUvProject(folderPath)) {
    return folderPath;
  }

  async function search(dir: string, depth: number): Promise<string | null> {
    if (depth >= maxDepth) return null;
    let entries: string[];
    try {
      entries = await readdir(dir, { withFileTypes: false }) as unknown as string[];
    } catch {
      return null;
    }

    for (const entry of entries) {
      if (entry === "node_modules" || entry === ".git" || entry === "__pycache__" || entry === ".venv") continue;
      const full = join(dir, entry);
      try {
        const dirEntries = await readdir(full);
        if (dirEntries) {
          if (await hasUvProject(full)) {
            return full;
          }
          const result = await search(full, depth + 1);
          if (result) return result;
        }
      } catch {
        // not a directory
      }
    }

    return null;
  }

  return search(folderPath, 0);
}

export async function findMainPyDir(folderPath: string, maxDepth = 5): Promise<string | null> {
  const rootMain = Bun.file(join(folderPath, "main.py"));
  if (await rootMain.exists()) {
    return folderPath;
  }

  async function search(dir: string, depth: number): Promise<string | null> {
    if (depth >= maxDepth) return null;
    let entries: string[];
    try {
      entries = await readdir(dir, { withFileTypes: false }) as unknown as string[];
    } catch {
      return null;
    }

    for (const entry of entries) {
      if (entry === "node_modules" || entry === ".git" || entry === "__pycache__" || entry === ".venv") continue;
      const full = join(dir, entry);
      try {
        const dirEntries = await readdir(full);
        if (dirEntries) {
          const nestedMain = Bun.file(join(full, "main.py"));
          if (await nestedMain.exists()) {
            return full;
          }
          const result = await search(full, depth + 1);
          if (result) return result;
        }
      } catch {
        // not a directory
      }
    }

    return null;
  }

  return search(folderPath, 0);
}

export async function findRunnablePythonFiles(folderPath: string, maxDepth = 5): Promise<string[]> {
  const candidates: { path: string; hasMainGuard: boolean }[] = [];
  const seen = new Set<string>();

  async function inspectFile(fullPath: string): Promise<void> {
    const relativePath = fullPath.slice(folderPath.length + 1);
    const parts = relativePath.split("/");
    const baseName = parts.length > 0 ? parts[parts.length - 1] : "";
    if (
      !relativePath.endsWith(".py")
      || baseName === "__init__.py"
      || baseName.startsWith("test_")
      || relativePath.includes("/test_")
      || relativePath.includes("/tests/")
      || seen.has(relativePath)
    ) {
      return;
    }

    seen.add(relativePath);
    const file = Bun.file(fullPath);
    let hasMainGuard = false;
    try {
      const content = await file.text();
      hasMainGuard = /if\s+__name__\s*==\s*["']__main__["']\s*:/.test(content);
    } catch {
      // unreadable file; keep as non-main-guard candidate
    }

    candidates.push({ path: relativePath, hasMainGuard });
  }

  async function search(dir: string, depth: number): Promise<void> {
    if (depth >= maxDepth) return;
    let entries: string[];
    try {
      entries = await readdir(dir, { withFileTypes: false }) as unknown as string[];
    } catch {
      return;
    }

    for (const entry of entries) {
      if (entry === "node_modules" || entry === ".git" || entry === "__pycache__" || entry === ".venv") continue;
      const full = join(dir, entry);
      try {
        const dirEntries = await readdir(full);
        if (dirEntries) {
          await search(full, depth + 1);
        }
      } catch {
        await inspectFile(full);
      }
    }
  }

  await search(folderPath, 0);

  return candidates
    .sort((a, b) => {
      if (a.hasMainGuard !== b.hasMainGuard) {
        return a.hasMainGuard ? -1 : 1;
      }
      return a.path.localeCompare(b.path);
    })
    .map((item) => item.path);
}

export function normalizeScriptTarget(value: string | null): string | null {
  if (!value) {
    return null;
  }
  const normalized = value.trim().replace(/\\/g, "/").replace(/^\.?\//, "");
  if (!normalized || normalized.includes("..")) {
    return null;
  }
  return normalized;
}
