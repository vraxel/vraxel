import path from "path"
import { defineConfig } from "vitest/config"
import react from "@vitejs/plugin-react"

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  test: {
    globals: true,
    environment: "jsdom",
    setupFiles: ["./src/test/setup.ts"],
    // e2e/ is the Playwright suite (`pnpm e2e`); its *.spec.ts files
    // must not be collected by vitest.
    exclude: ["**/node_modules/**", "e2e/**"],
  },
})
