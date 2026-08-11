// Package apis is the assembly layer: it constructs every API module,
// aggregates their registration closures, and exposes the few
// cross-cutting singletons (audit writer, OIDC provider, RBAC
// authorizer) that main.go needs. It is the ONLY package allowed to
// know about more than one module at a time -- see
// pkg/apis/ARCHITECTURE.md.
package apis

import (
	"context"
	"net/http"

	"vraxel.io/vraxel/lib/apiserver"
	libaudit "vraxel.io/vraxel/lib/audit"
	"vraxel.io/vraxel/lib/config"
	"vraxel.io/vraxel/lib/oidc"
	"vraxel.io/vraxel/lib/pgnotify"
	"vraxel.io/vraxel/lib/rest/filters"
	"vraxel.io/vraxel/pkg/apis/audit"
	"vraxel.io/vraxel/pkg/apis/iam"
	"vraxel.io/vraxel/pkg/db"
)

// Result holds the outputs of API module initialization.
//
// Mux is exposed so main can call Mux.Start(ctx) AFTER NewAuthorizer
// has run (which is the last Subscribe call site). Starting earlier
// would panic any later Subscribe.
type Result struct {
	Mux *pgnotify.Multiplexer

	// Registrars collects every module's registration closure. Held as
	// closures because the apiserver needs the RBAC checker at Register
	// time, and the checker is constructed later in main.go via
	// NewAuthorizer. main replays them onto the apiserver.
	Registrars []func(*apiserver.Server)

	iamResult iam.ModuleResult
}

// SyncPermissions persists the apiserver's derived permission records
// through iam's per-module sync pipeline. Called by main.go after the
// Registrars have been replayed.
func (r Result) SyncPermissions(ctx context.Context, srv *apiserver.Server) error {
	return iam.SyncPermissions(ctx, r.iamResult.Stores.Permission, srv.PermRecordsByModule())
}

// NewModules assembles all API modules and returns the aggregated result.
func NewModules(ctx context.Context, database *db.DB) Result {
	// Cross-instance pgnotify multiplexer. Single dedicated PG connection
	// multiplexed across every module's LISTEN channel. Modules call
	// XxxChannel.Subscribe(mux, handler) inside NewModule; main calls
	// mux.Start(ctx) once every Subscribe has happened.
	mux := pgnotify.New(database.Pool)

	iamResult := iam.NewModule(ctx, database)

	return Result{
		Mux:        mux,
		Registrars: moduleRegistrars(database),
		iamResult:  iamResult,
	}
}

// moduleRegistrars is the single list of modules. Both the running
// server and openapi-gen go through it, so a module cannot be wired
// into one and forgotten in the other.
func moduleRegistrars(database *db.DB) []func(*apiserver.Server) {
	return []func(*apiserver.Server){
		iam.Registrar(database),
		audit.Registrar(database),
	}
}

// Registrars returns every module's registration closure without a
// database. Registration builds no queries, so the resulting server has
// the real route table and nothing else -- the input openapi-gen needs
// to describe exactly the endpoints that exist.
func Registrars() []func(*apiserver.Server) { return moduleRegistrars(nil) }

// NewAuthorizer creates a fully-wired Authorizer from API group definitions.
// Subscribes RBAC invalidation on the shared multiplexer -- the LAST
// Subscribe call in the boot sequence; main MUST call result.Mux.Start(ctx)
// afterwards.
func NewAuthorizer(result Result) *filters.Authorizer {
	return iam.NewAuthorizerFromResult(result.Mux, result.iamResult)
}

// NewOIDCProvider creates the OIDC provider with all internal store wiring.
// Keys are auto-generated and stored in the database.
func NewOIDCProvider(database *db.DB, result Result, cfg *config.OIDCConfig) *oidc.Provider {
	return iam.NewOIDCProvider(database, result.iamResult, cfg)
}

// NewAuditWriter creates the async audit log writer. The actual store
// construction lives in the audit module so this file never reaches
// into any module's store package directly (see pkg/apis/ARCHITECTURE.md).
func NewAuditWriter(database *db.DB) *libaudit.Writer {
	return audit.NewAuditWriter(database)
}

// NewOIDCMux creates the OIDC public endpoint HTTP handler.
func NewOIDCMux(provider *oidc.Provider, auditLogger libaudit.Logger) http.Handler {
	return iam.NewOIDCMux(provider, auditLogger)
}
