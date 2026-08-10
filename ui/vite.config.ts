import path from "path"
import { defineConfig } from "vite"
import react from "@vitejs/plugin-react"
import tailwindcss from "@tailwindcss/vite"

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  server: {
    port: 5173,
    // The repo lives at /Users/.../vraxel and pnpm hoists shared deps into
    // /Users/.../vraxel/node_modules/.pnpm. Vite's default fs.allow only
    // includes the workspace root (ui/) + its own dist, so requests for
    // assets such as @fontsource-variable/inter/files/*.woff2 (resolved
    // through ../node_modules/.pnpm/...) return 403. Whitelisting the
    // repo root lets dev mode reach the pnpm store while keeping it
    // bound to a known prefix.
    fs: {
      allow: [".", ".."],
    },
    proxy: {
      "/api/": {
        target: "http://localhost:8088",
        changeOrigin: true,
        ws: true,
        configure: (proxy) => {
          proxy.on("error", (err) => {
            if ((err as NodeJS.ErrnoException).code === "EPIPE") return
            console.error("proxy error:", err.message)
          })
        },
      },
      "/docs": {
        target: "http://localhost:8088",
        changeOrigin: true,
      },
      "/oidc": {
        target: "http://localhost:8088",
        changeOrigin: true,
      },
      "/.well-known": {
        target: "http://localhost:8088",
        changeOrigin: true,
      },
    },
  },
})
