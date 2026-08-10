# Vraxel

A Go + React platform skeleton: a declarative REST framework with
scope-aware RBAC, an embedded OIDC provider, and a matching frontend
shell. It ships two modules — `iam` (users / workspaces / namespaces /
roles / role bindings / permissions) and `audit` (audit log) — which are
both the working auth substrate and the reference implementation for
every module you add.

## Stack

| | |
|---|---|
| Backend | Go 1.26, pgx/v5 + sqlc, embedded OIDC (EdDSA), PostgreSQL |
| Frontend | React 19 + TypeScript + Vite + Tailwind 4 + shadcn/ui (Radix), TanStack Query, zustand, react-hook-form + zod |
| Delivery | single binary — the built `ui/dist` is `go:embed`ed into `vraxel-server` |

## Layout

```
app/vraxel-server/   main + root HTTP router (health, OIDC, OpenAPI, /api, SPA)
cmd/                 dev-token, init-admin, openapi-gen
lib/                 framework: apiserver, rest, runtime, oidc, audit, config,
                     httpserver, logger, list, openapi, pgnotify, websocket, ...
pkg/apis/            API modules (three-layer onion, see pkg/apis/ARCHITECTURE.md)
pkg/db/              pgx pool, migrations, sqlc queries + generated code
ui/src/core/         api client, auth, permissions, query keys, resource registry, scope
ui/src/frameworks/   list page + form dialog scaffolding
ui/src/shared/       shadcn primitives, generic components, hooks, utils
ui/src/modules/      per-module pages/api/defs (iam, audit)
```

## Quick start

```bash
createdb vraxel                      # or point DB_* env vars at an existing PG
pnpm install && make ui-build        # build the embedded frontend
go run ./app/vraxel-server -config ./app/vraxel-server/config.yaml
```

Boot is self-provisioning: migrations run, the admin user is seeded from
`admin:` in the config, built-in platform roles are seeded, and every
route's permission code is derived from the registered storages and
synced into the `permissions` table.

Zero-config also works -- with no config file at all, every default
lands (PG on localhost, `externalUrl` http://localhost:8088) and the
OIDC issuer + redirect URIs are derived from `externalUrl`, so
authentication is always on. There is no "auth disabled" mode. Override
precedence: CLI flags > env (`DB_*`, `SERVER_EXTERNAL_URL`, ...) >
config file > defaults.

Open <http://localhost:8088> and log in with `admin` / `Admin123!`.

For frontend work run both halves with HMR:

```bash
make dev     # vraxel-server :8088 + vite :5173 (proxies /api, /oidc, /docs)
```

`make dev` uses the gitignored `app/vraxel-server/config.dev.yaml` when
present, else the committed `config.yaml`.

## Common tasks

```bash
make test lint-layers        # go test + pkg/apis layer guard
make ui-test ui-lint         # vitest + eslint/jsx/boundary lints
make new-migration NAME=add_widgets
make sqlc-generate           # regenerate pkg/db/generated from query/*.sql
make openapi-gen             # regenerate the served OpenAPI spec
make ts-types                # regenerate ui/src/generated from Go types
make setup-hooks             # pre-commit: typecheck + layer guard
```

## API E2E

```bash
TOKEN=$(DB_HOST=... go run ./cmd/dev-token)
curl -s --cookie "vraxel_at=$TOKEN" http://localhost:8088/api/iam/v1/users
```

Auth is cookie-only (BFF pattern). CSRF is enforced only when a
`vraxel_csrf` cookie is present, so curl `POST`/`PUT`/`DELETE` with just
`vraxel_at` work without a CSRF header.

## Adding a module

`pkg/apis/ARCHITECTURE.md` is the contract. In short: create
`pkg/apis/<mod>/install.go` returning a `ModuleResult`, put stores under
`pkg/apis/<mod>/store/`, register resources on the `apiserver.Server`,
and hook the module into `pkg/apis/install.go`. URL paths
(`/api/{group}/{version}/{resource}`) and permission codes
(`{module}:{resource}:{verb}`) are derived from the registration — never
hand-written. RBAC scope (platform / workspace / namespace) follows
registration depth. Run `make lint-layers` before committing.

On the frontend, mirror it: `ui/src/modules/<mod>/{defs.ts,api/,pages/}`,
add the routes to `ui/src/app/routes.tsx`, the nav entries to
`ui/src/core/registry/nav-config.ts`, the prefix to
`ui/src/core/registry/modules.ts`, and a locale file per language under
`ui/src/i18n/locales/`.
