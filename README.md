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
docker compose -f deployment/docker-compose.yaml up -d   # local PG (or bring your own)
pnpm install
make build             # frontend + release binary into bin/
./bin/vraxel-server -config ./app/vraxel-server/config.yaml
```

Boot is self-provisioning: migrations run, the admin user is seeded from
`admin:` in the config, built-in platform roles are seeded, and every
route's permission code is derived from the registered storages and
synced into the `permissions` table.

Zero-config also works -- with no config file at all, every default
lands (PG on localhost, `externalUrl` http://localhost:9099) and the
OIDC issuer + redirect URIs are derived from `externalUrl`, so
authentication is always on. There is no "auth disabled" mode. Override
precedence: CLI flags > env (`DB_*`, `SERVER_EXTERNAL_URL`, ...) >
config file > defaults.

Open <http://localhost:9099> and log in with `admin` / `Admin123!`.

For frontend work run both halves with HMR:

```bash
make dev     # vraxel-server :9099 + vite :5199 (proxies /api, /oidc, /docs)
```

`make dev` prints and uses the gitignored
`app/vraxel-server/config.dev.yaml` when present, else the committed
`config.yaml`. The overlay only needs the handful of values that differ
from the defaults -- typically `server.externalUrl`, `server.name` and
`database.host`; the OIDC issuer and both callback URLs (embedded
frontend + vite on :5199) are derived from `externalUrl`, so no `oidc:`
section is needed for local work:

```yaml
server:
  externalUrl: "http://localhost:9099"
  name: "vraxel-dev"
database:
  host: "db.internal.example" # omit entirely for a local PostgreSQL
```

## Common tasks

`make` with no target lists everything:

```
dev             Run vraxel-server (:9099) + vite (:5199) with HMR
build           Build the frontend and link the release binary into bin/
generate        Regenerate committed sqlc / OpenAPI / TS artifacts (commit the result)
check           Run all gates: gofmt, vet, layer guard, Go tests, UI typecheck/lint/tests
fmt             Format Go (gofmt -s) and the UI (prettier)
new-migration   Create a timestamped migration (NAME=add_widgets)
setup-hooks     Point git at .githooks (pre-commit: UI typecheck + layer guard)
clean           Remove build output
```

`make check` is the pre-commit gate; CI (.github/workflows/ci.yml) runs
the same gate plus `go test -race` and `make build`. Store-layer
integration tests (a throwaway database per test via `pkg/db/dbtest`)
run whenever PostgreSQL is reachable at the compose defaults -- start
`deployment/docker-compose.yaml` locally or rely on the CI service
container; without one they skip. `make generate`
must be run and its output committed after changing
`pkg/db/query/*.sql`, a resource registration, or a Go API type --
`build` compiles committed sources and never regenerates.

A container build lives at `deployment/Dockerfile` (UI -> Go ->
distroless, ~30MB image).

## API E2E

```bash
TOKEN=$(DB_HOST=... go run ./cmd/dev-token)
curl -s --cookie "vraxel_at=$TOKEN" http://localhost:9099/api/iam/v1/users
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
registration depth. Run `make check` before committing.

On the frontend, mirror it: `ui/src/modules/<mod>/{defs.ts,api/,pages/}`,
add the routes to `ui/src/app/routes.tsx`, the nav entries to
`ui/src/core/registry/nav-config.ts`, the prefix to
`ui/src/core/registry/modules.ts`, and a locale file per language under
`ui/src/i18n/locales/`.

## Operational notes

- Password logins are brute-force throttled in PostgreSQL (shared by
  all instances): 5 failures per username or 20 per client IP within
  15 minutes lock the key for the rest of the window (HTTP 429 +
  Retry-After). A success clears the username counter only.
- The client IP behind that throttle comes from the socket peer unless
  `server.trustedProxies` lists the ingress range. X-Forwarded-For is
  forgeable, so it is ignored by default -- set the CIDRs when running
  behind a load balancer, or every per-IP control keys on a value the
  attacker chooses.
- `/debug/pprof/*` is disabled unless `-pprofAuthKey` or `-httpAuth.*`
  is configured -- it exposes heap contents and the command line, and
  this server fronts the edge. `/metrics` stays open (Prometheus
  convention); gate it with `-metricsAuthKey` if needed.
- The API reference is a separate HTML entry (`/api-docs.html`, also
  reachable as `/api-docs`): Scalar embeds a full Vue runtime, and the
  split keeps it out of the SPA's first paint (~4.2MB -> ~1.1MB
  pre-gzip).
- The React Compiler is on (vite babel preset): components are
  auto-memoized at build time, and the react-hooks v7 lint rules gate
  compiler compatibility in `pnpm lint`.
