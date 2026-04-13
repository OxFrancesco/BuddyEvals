import { defineConfig } from "vitest/config"
import path from "node:path"

export default defineConfig({
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "src"),
    },
  },
  test: {
    globals: true,
    include: ["test/**/*.test.ts"],
    hookTimeout: 30_000,
    testTimeout: 30_000,
  },
})
