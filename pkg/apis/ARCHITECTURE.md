# pkg/apis Layer Architecture Rules (Model 2: Minimal Onion)

Authoritative layering rules for `pkg/apis/<module>/`. Enforced by
`scripts/check-layer-leak.sh` (runnable via `make lint-layers`) and
the pre-commit hook.

## Design principle: three-layer onion, one-way dependency

Every arrow below points **from** a layer that may import **to** the
layer it imports. No cycles, no cross-arrows, no up-arrows:

```
pkg/apis/install.go (top-level assembly)
        │
        ▼
pkg/apis/<mod>/*.go           (business: module entry, REST storages, API types)
        │
        ▼
pkg/apis/<mod>/store/*.go     (store interfaces + domain types + pg impls)
        │
        ▼
pkg/db/*                      (pgx wrappers, sqlc generated, pgerrors, ...)
```

Each module exposes a single entry file `pkg/apis/<mod>/install.go`
(package `<mod>`) with `NewModule(d *db.DB, deps...) ModuleResult`.
The top-level assembly imports only `pkg/apis/<mod>` (never
`pkg/apis/<mod>/store`). The store sub-package is self-contained:
domain row types, input types, store interfaces, and pg impls all
live inside it.

Rationale: v1/ sub-packages are ceremony for a Go-level distinction
API versioning does not actually need. API version is an HTTP concept
driven by `APIGroupInfo.Version`; today every module has exactly one
version, so a flat `<mod>/install.go` is the minimal and correct
layout. Multi-version support is lazy — promoted per module only when
that module introduces a second version (see "Multi-version API"
below).

## The three layers

| Layer | Location | Responsibility | May import |
|---|---|---|---|
| **top-level assembly** | `pkg/apis/install.go` | Boots every module, stitches cross-module dependencies using each module's `ModuleResult`. | `pkg/apis/<mod>` for every module; `pkg/db` for the `*db.DB` handle. **MUST NOT import any `<mod>/store`, `pkg/db/generated`, or `pkg/db/sqlnull`.** |
| **business** | `pkg/apis/<mod>/*.go` (excluding `store/`) | Module entry (`install.go`), REST storage impls, validation, domain orchestration, HTTP API types, cross-module interfaces. | `pkg/apis/<same-mod>/store`, other modules' public packages `pkg/apis/<other>` (for interfaces they expose), `lib/...`, `lib/api/errors`. **`pkg/db` (root, for the `*db.DB` handle) is allowed only in `<mod>/install.go`** — never in `storage.go`, handlers, or other business files. **MUST NOT import `pkg/db/generated`, `pkg/db/sqlnull`, or any pgx subpackage.** |
| **store (data)** | `pkg/apis/<mod>/store/*.go` | CRUD against the SQL tier. Owns **store interfaces**, **domain row/input types**, and translation between sqlc rows (`generated.*`, `pgtype.*`) and those domain types. | `pgx/*`, `pgtype`, `pgconn`, `pgxpool`, `pkg/db`, `pkg/db/generated`, `pkg/db/pgerrors`, `pkg/db/sqlnull`; pure-Go libs (`lib/list`, `lib/logger`, etc.). **MUST NOT import `pkg/apis/<same-mod>`** (the module's business package). |

## Hard rules

1. **pgx / pgtype / pgconn / pgxpool / `pkg/db/generated` confined to the store layer.** They must not appear in business-layer files or in `pkg/apis/install.go`. Root `pkg/db` (for `*db.DB` handle) is allowed in `pkg/apis/install.go` and `pkg/apis/<mod>/install.go` only.

2. **No cross-module store imports.** A file outside `pkg/apis/<B>/` must not import `pkg/apis/<B>/store` for any module B. Cross-module data access goes through interfaces declared in B's business package (`pkg/apis/<B>`), with impls exposed via B's `ModuleResult`.

3. **Store sub-package is self-contained.** `pkg/apis/<mod>/store` MUST NOT import `pkg/apis/<mod>` (the module's business layer). Domain row types (`FooRow`, `FooWithBarRow`), input types (`FooCreateInput`, `FooUpdateInput`), and store interfaces (`FooStore`) live in `<mod>/store/types.go` + `<mod>/store/interfaces.go` (or equivalent split), not in the business layer.

4. **HTTP API types live in the business layer.** The types REST handlers return to clients (`User`, `Workspace`, `Namespace`, etc.) belong in `<mod>/types.go` (business). They are distinct from domain rows (in `<mod>/store/types.go`). The business layer owns translation between them.

5. **Test files (`*_test.go`) are exempt from rules 1-3 during migration.** Long-term target: tests in `<mod>/store/*_test.go` use only store-layer types; tests in `<mod>/*_test.go` use only business-layer types + mocks of store interfaces.

## Multi-version API

API version is an HTTP-level concept (`/api/<mod>/v1/*`) driven by
`APIGroupInfo.Version`, not by Go package path. For a module with a
single version (the common case today), the flat layout above is the
minimal and correct form.

When a module introduces a second version (e.g. `Foo` shape changes
incompatibly), promote **that module only**:

```
pkg/apis/<mod>/install.go         (dispatcher; returns ModuleResult with Groups []*APIGroupInfo)
pkg/apis/<mod>/v1/install.go      (package v1: BuildGroup(stores, deps...) *APIGroupInfo)
pkg/apis/<mod>/v1/types.go        (v1-specific API types)
pkg/apis/<mod>/v1/storage.go      (v1-specific REST storages)
pkg/apis/<mod>/v2/install.go      (package v2)
pkg/apis/<mod>/v2/types.go
pkg/apis/<mod>/v2/storage.go
pkg/apis/<mod>/store/*            (shared store; split into store/v1/ + store/v2/ only if DB schema diverges)
```

The dispatcher `<mod>/install.go`:

```go
package <mod>

import (
    "vraxel.io/vraxel/lib/rest"
    "vraxel.io/vraxel/pkg/apis/<mod>/store"
    "vraxel.io/vraxel/pkg/apis/<mod>/v1"
    "vraxel.io/vraxel/pkg/apis/<mod>/v2"
    "vraxel.io/vraxel/pkg/db"
)

type ModuleResult struct {
    Groups []*rest.APIGroupInfo
}

func NewModule(database *db.DB) ModuleResult {
    stores := store.NewStores(database)
    return ModuleResult{
        Groups: []*rest.APIGroupInfo{
            v1.BuildGroup(stores),
            v2.BuildGroup(stores),
        },
    }
}
```

Permission codes (`{module}:{resource}:{verb}`) are version-stable, so
v1 and v2 of the same resource share a permission tree. No per-version
permission sync is needed.

## Shared libraries under pkg/apis/shared/

Packages under `pkg/apis/shared/` are **apis-scoped shared libraries**,
not business modules:

- no REST endpoints, no `install.go` / `ModuleResult` / `store/` subdir
- no participation in the three-layer onion — they follow plain Go
  package rules
- any `pkg/apis/<mod>/*.go` or `pkg/apis/<mod>/store/*.go` may import
  them freely; `scripts/check-layer-leak.sh` does not treat them as a
  module (they do not match the `<mod>` segment handling in any rule)

Current residents: none. `pkg/apis/shared/` is created on first use — a
primitive graduates here only once a second module needs it, never
speculatively.

When adding a new shared library here, keep the same contract: no REST
surface, no `store/` subdir, no participation in per-module lint rules.
If the new code is a business module, it belongs at `pkg/apis/<mod>/`
instead; if it's project-wide (consumed by `app/` / `cmd/` as well as
`pkg/apis/`), it belongs at `pkg/<name>/` (next to `pkg/db`) or `lib/<name>/` for pure-Go libraries with no Vraxel
business assumptions.

## Lint coverage

`scripts/check-layer-leak.sh` enforces the rules:

- **rule A (infra leak)**: imports of `pgx/*`, `pgtype`, `pgconn`, `pgxpool`, `pkg/db/*` subtree may only appear in `pkg/apis/<mod>/store/*.go`. Narrow exemption: `pkg/apis/install.go` and `pkg/apis/<mod>/install.go` may import `pkg/db` (root package only, for the `*db.DB` handle).

- **rule B (cross-module store)**: any file not under `pkg/apis/<B>/` (the importee module's own tree) importing `pkg/apis/<B>/store` is forbidden. Same-module imports are always allowed.

- **rule D (store reverse)**: `pkg/apis/<mod>/store/*.go` importing `pkg/apis/<mod>` (the business package) is forbidden.

- **rule E (raw SQL in store)**: `pkg/apis/<mod>/store/*.go` must not call `pool.Query` / `tx.Exec` / `QueryRow(ctx, "...")` with a literal DML string (`SELECT` / `INSERT` / `UPDATE` / `DELETE` / `WITH`). Add the query to `pkg/db/query/*.sql`, run `make sqlc-generate`, and call it via `s.Q().XxxMethod()`. Non-DML escape hatch (`pg_notify`, `LISTEN`, advisory-lock primitives) — append `// lint:allow-raw-sql` to the call site.

Modes:

- `make lint-layers` / `./scripts/check-layer-leak.sh` — full-tree scan. This is the pre-commit gate and the required CI gate.
- `./scripts/check-layer-leak.sh staged` — staged-only scan, retained for optional use in future focused migrations. Not used by the pre-commit hook anymore.

## Refactor checklist (Model 1 → minimal Model 2, per module)

Use this to migrate one module. Each step maps to one commit.

1. **Move domain types.** Move every `FooRow` / `FooWithBarRow` / `FooCreateInput` / `FooUpdateInput` / `FooPatchInput` from `<mod>/types.go` to `<mod>/store/types.go`. Keep HTTP API types (`Foo`, `FooSpec`, `FooList`) in `<mod>/types.go`.

2. **Move store interfaces.** Move every store interface (`FooStore`, `BarStore`) from `<mod>/store.go` to `<mod>/store/interfaces.go`. Delete the now-empty `<mod>/store.go`. Place the `Stores` aggregate and `NewStores(d *db.DB)` factory inside `<mod>/store/` — a dedicated `stores.go` for multi-store modules, or merged into `interfaces.go` for single-store modules.

3. **Reverse import direction.**
   - `<mod>/store/*.go`: remove `import "vraxel.io/vraxel/pkg/apis/<mod>"` (types now live next to impl). Strip the `<mod>.` prefix from type references.
   - `<mod>/storage*.go` and other business-layer files: add `import modstore "vraxel.io/vraxel/pkg/apis/<mod>/store"`. Change every `<mod>.FooStore` / `<mod>.FooRow` / `<mod>.FooCreateInput` reference to `modstore.FooStore` / `modstore.FooRow` / etc.

4. **Collapse `v1/install.go` into `<mod>/install.go`.** Merge the old `<mod>/v1/install.go` body into a new `<mod>/install.go` at the package root (`package <mod>`). Rename `NewXxxModule` to `NewModule`. The new file is the only business-layer file that imports `pkg/db` (handle only) and `<mod>/store`. Inside it, call `store.NewStores(database)` directly. Delete `<mod>/v1/` and, if they exist, `<mod>/build.go` and `<mod>/provider.go` (their former roles collapse into `install.go`).

5. **Embed `pkg/db.Store` in store impls.** Each `<mod>/store/pg_*.go` replaces `db *pgxpool.Pool; queries *generated.Queries` with a single `db.Store` embedding; constructors take `*db.DB`; methods use `s.Q()` / `db.WithTxReturning` instead of hand-written `s.db.Begin`.

6. **Migrate errors to `pkg/db/pgerrors`.** Store impls return `pgerrors.ErrNotFound` / `pgerrors.ErrConflict` sentinels (via `%w`) instead of `apierrors.NewNotFound(...)`. Business layer (or a REST middleware) calls `apierrors.FromDomain(err, resource)` to produce `*StatusError` at the HTTP boundary.

7. **Update top-level `pkg/apis/install.go`.** Change the module's import from `<mod>v1 "vraxel.io/vraxel/pkg/apis/<mod>/v1"` to `"vraxel.io/vraxel/pkg/apis/<mod>"` and the call from `<mod>v1.NewXxxModule(...)` to `<mod>.NewModule(...)`.

8. **Verify.** `make lint-layers` must report zero entries for the migrated module.

## OpenAPI generation & the Registrar convention

The committed spec (`app/vraxel-server/apis/openapi.json`) is derived from
the **apiserver route table**, not from the shape of the source:
`openapi-gen` replays every module's registration onto a bare server and
reads back the routes it declared. Registration is the only thing that
knows what is served, so it is the input; the AST parser is left with what
it alone knows — schemas and doc text. `cmd/openapi-gen`'s
`TestSpecMatchesRouteTable` pins the two together, so a route added without
re-running `make generate` fails the build instead of shipping a spec that
lies about the API.

This works only because registration obeys four rules ("static
registration, lazy wiring"). A module that breaks one silently drops or
mis-describes its endpoints:

1. **Unconditional.** A module registers its full set of resources /
   actions / verbs every time. Never gate a `Register` call on a runtime
   dep (`if deps.X != nil { apiserver.Register(...) }`). Whether an
   endpoint *exists* is a static fact; whether a deployment can *serve* it
   is the handler's concern (return 503 when its backing dep is absent).
2. **DB-free & side-effect-free.** The registration path constructs no
   database/pool/queries resource and runs no seed / goroutine / pgnotify
   subscribe. Constructors that dereference the DB at build time
   (`tasktracker.NewWithDB`, `*.GetPool()`, event publishers) are built in
   `NewModule` and threaded into the registrar; the registrar passes `nil`.
   The global config *may* be read (openapi-gen initialises a default one).
3. **Single source of truth.** `NewModule`'s `.V2` (real deps) and the
   exported `Registrar(db)` (nil deps) call the *same* package-level
   registration function, so the running server and the spec never drift.
4. **Payload types stay honest.** A `list` route responds `<Type>List`; the
   generator synthesises a `{items,totalCount}` envelope for any wrapper a
   module never declared, and emits a generic object (not a dangling `$ref`)
   for a payload the AST parser never annotated — so an under-annotated
   module still gets its endpoints, just with looser schemas.

**Per-module hook.** Every module exposes
`func Registrar(database *db.DB) func(*apiserver.Server)`. `Registrars()`
in `pkg/apis/install.go` lists all of them with a `nil` handle and is what
openapi-gen replays; it MUST stay in lockstep with the module registrar
list used by `NewModules`. A module that serves only anonymous
machine-facing handlers (no `apiserver.Register`) has no route-table
contribution and is omitted from `Registrars()`.

