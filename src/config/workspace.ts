import path from "node:path"

export type WorkspacePaths = {
  root: string
  dataDir: string
  dbPath: string
  artifactsDir: string
  workspacesDir: string
}

export function getWorkspacePaths(root = process.cwd()): WorkspacePaths {
  const dataDir = path.join(root, ".buddyevals")
  return {
    root,
    dataDir,
    dbPath: path.join(dataDir, "runs.sqlite"),
    artifactsDir: path.join(dataDir, "artifacts"),
    workspacesDir: path.join(dataDir, "workspaces"),
  }
}
