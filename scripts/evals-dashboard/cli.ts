export function readStringArg(args: string[], flag: string): string | null {
  const idx = args.indexOf(flag);
  if (idx === -1) {
    return null;
  }
  const value = args[idx + 1];
  if (!value || value.startsWith("--")) {
    return null;
  }
  return value;
}

export function readPortArg(args: string[], flag: string): number | null {
  const raw = readStringArg(args, flag);
  if (!raw) {
    return null;
  }
  const value = Number.parseInt(raw, 10);
  if (!Number.isInteger(value) || value < 1 || value > 65535) {
    return null;
  }
  return value;
}

export function openBrowser(url: string): void {
  let cmd: string[];
  if (process.platform === "darwin") {
    cmd = ["open", url];
  } else if (process.platform === "win32") {
    cmd = ["cmd", "/c", "start", "", url];
  } else {
    cmd = ["xdg-open", url];
  }

  try {
    Bun.spawn(cmd, { stdout: "ignore", stderr: "ignore" });
  } catch {
    console.log(`Open this URL manually: ${url}`);
  }
}
