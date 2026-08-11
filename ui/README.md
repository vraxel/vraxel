# vraxel ui

React 19 + TypeScript + Vite + Tailwind 4 + shadcn/ui (Radix).
Built output lands in `dist/` and is `go:embed`ed by `vraxel-server`
(`dist/.gitkeep` is committed and restored from `public/` on every build
so a fresh clone can `go build ./...` before ever running vite).

TypeScript is pinned to `~6.0.x` (latest JS-based compiler): the native
TS 7 toolchain has no JS compiler API yet, which hard-breaks
typescript-eslint (typescript-eslint#10940). Bump to 7.x once that
lands.

```bash
pnpm dev        # vite :5199, waits for vraxel-server :9099, proxies /api /oidc /docs
pnpm build      # tsc -b && vite build  (what CI runs)
pnpm typecheck  # tsc -b --noEmit
pnpm test       # vitest
pnpm lint       # prettier --check + eslint + lint:jsx + lint:boundaries
```

## Layers

`shared` -> `core` -> `frameworks` -> `modules`, one direction only.
`pnpm lint:boundaries` enforces it with an empty allowlist:

- a module must not import another module's internals
- `shared` / `core` / `frameworks` must not import `modules`
- `shared/ui` (shadcn primitives) must stay free of app code

Identity and permission payloads live in `core/auth/identity.ts`, not in
the iam module, because the session shell reads them before any module
page mounts.

## Conventions

- **No dynamic loading.** No `React.lazy`, `<Suspense>` code-splitting,
  react-router `lazy:`, or `await import()`. Everything is a static ESM
  import; split files, not chunks.
- All user-facing text goes through `t("key")`; `zh-CN` is the source of
  truth for the key set and `en-US` must satisfy `Record<MessageKey, string>`.
- Lists get search (300ms debounce), filters, sortable columns,
  pagination, batch select, skeleton, and empty state — via
  `frameworks/list/resource-list-page`.
- Form dialogs use `frameworks/form/form-dialog` so the
  header / scrollable body / pinned footer structure stays consistent.
- Routes embed scope: `/{module}/workspaces/{wsId}/namespaces/{nsId}/{resource}`.
