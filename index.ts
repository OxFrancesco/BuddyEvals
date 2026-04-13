import { prepareLauncher } from "./src/launcher.ts"

process.argv = await prepareLauncher(process.argv)

await import("./src/main.ts")
