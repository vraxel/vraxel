import path from "node:path"
import { defineConfig } from "vite"
import react, { reactCompilerPreset } from "@vitejs/plugin-react"
import babel from "@rolldown/plugin-babel"
import tailwindcss from "@tailwindcss/vite"

export default defineConfig({
  plugins: [
    react(),
    // React Compiler: automatic memoization at build time. The
    // react-hooks v7 lint rules are the compiler's own diagnostics and
    // the tree passes them clean, so every component is compiled -- no
    // opt-out list. Manual useMemo/useCallback remain valid and are
    // preserved when provably equivalent.
    babel({
      include: /\.tsx?$/,
      exclude: [/node_modules/],
      presets: [reactCompilerPreset()],
    }),
    tailwindcss(),
  ],
  resolve: {
    alias: {
      "@": path.resolve(import.meta.dirname, "./src"),
    },
  },
  build: {
    rollupOptions: {
      input: {
        main: path.resolve(import.meta.dirname, "index.html"),
        "api-docs": path.resolve(import.meta.dirname, "api-docs.html"),
      },
    },
  },
  server: {
    port: 5173,
    // Fail instead of silently sliding to 5174: the OIDC client's
    // redirect URI is pinned to :5173, so a shifted port turns into a
    // login failure with no obvious link to the port.
    strictPort: true,
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
