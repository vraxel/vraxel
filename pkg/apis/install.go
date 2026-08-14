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
	"vraxel.io/vraxel/lib/logger"
	"vraxel.io/vraxel/lib/oidc"
	"vraxel.io/vraxel/lib/pgnotify"
	"vraxel.io/vraxel/lib/rest/filters"
	"vraxel.io/vraxel/lib/statushub"
	"vraxel.io/vraxel/pkg/apis/agentgw"
	"vraxel.io/vraxel/pkg/apis/audit"
	"vraxel.io/vraxel/pkg/apis/compute"
	"vraxel.io/vraxel/pkg/apis/iam"
	"vraxel.io/vraxel/pkg/apis/pki"
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

	// AgentProtocolHandler serves /api/agent/v1/* (register / control
	// channel). Not a REST module -- main mounts it as a prefix branch
	// ahead of the IAM APIHandler.
	AgentProtocolHandler http.HandlerFunc

	// InstallScriptHandler serves /install-agent.sh at the root.
	InstallScriptHandler http.HandlerFunc

	iamResult iam.ModuleResult
}

// SyncPermissions persists the apiserver's derived permission records
// through iam's per-module sync pipeline. Called by main.go after the
// Registrars have been replayed.
func (r Result) SyncPermissions(ctx context.Context, srv *apiserver.Server) error {
	return iam.SyncPermissions(ctx, r.iamResult.Stores.Permission, srv.PermRecordsByModule())
}

// NewModules assembles all API modules and returns the aggregated result.
//
// listenAddr is where this process serves HTTP. The agent gateway
// advertises it to sibling instances, so it has to be the real listener
// rather than anything derived from externalUrl.
func NewModules(ctx context.Context, database *db.DB, listenAddr string) Result {
	// Cross-instance pgnotify multiplexer. Single dedicated PG connection
	// multiplexed across every module's LISTEN channel. Modules call
	// XxxChannel.Subscribe(mux, handler) inside NewModule; main calls
	// mux.Start(ctx) once every Subscribe has happened.
	mux := pgnotify.New(database.Pool)

	iamResult := iam.NewModule(ctx, database)

	// The platform master key: loaded from the DB, generated on first boot.
	// The agent-token signing key is derived from it.
	pkiResult, err := pki.NewModule(ctx, database)
	if err != nil {
		logger.Fatalf("cannot load/generate encryption key: %v", err)
	}

	// The agent gateway: machine-facing register + control-channel surface.
	// Not a REST module; its handler is mounted by main ahead of IAM.
	agentgwResult := agentgw.NewModule(ctx, database, agentgw.Deps{
		HostRegistrar: compute.NewAgentHostRegistrar(database),
		JoinTokens:    agentgw.NewJoinTokenStore(database),
		EncryptionKey: pkiResult.EncryptionKey,
		ServerName:    config.Get().Server.Name,
		ListenAddr:    listenAddr,
	})

	// Host watch: cross-instance host / agent-status events onto this
	// instance's WebSocket subscribers. Subscribes on mux, so it has to be
	// built before main calls Start.
	computeResult := compute.NewModule(ctx, database, mux)

	return Result{
		Mux:                  mux,
		Registrars:           moduleRegistrars(database, config.Get().Server.ExternalURL, computeResult.Hub),
		AgentProtocolHandler: agentgwResult.ProtocolHandler,
		InstallScriptHandler: agentgwResult.InstallScriptHandler,
		iamResult:            iamResult,
	}
}

// moduleRegistrars is the single list of modules. Both the running
// server and openapi-gen go through it, so a module cannot be wired
// into one and forgotten in the other.
func moduleRegistrars(database *db.DB, serverURL string, hostWatch *statushub.Hub) []func(*apiserver.Server) {
	return []func(*apiserver.Server){
		iam.Registrar(database),
		audit.Registrar(database),
		compute.Registrar(database, serverURL, hostWatch),
	}
}

// Registrars returns every module's registration closure without a
// database. Registration builds no queries, so the resulting server has
// the real route table and nothing else -- the input openapi-gen needs
// to describe exactly the endpoints that exist.
// The empty serverURL is correct here: openapi-gen describes routes and
// schemas, and no response body is produced. The hub is real but
// unattached -- nothing publishes to it and no client subscribes, which
// is cheaper than teaching every route to tolerate a nil one.
func Registrars() []func(*apiserver.Server) { return moduleRegistrars(nil, "", statushub.New()) }

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

// NewOIDCMux creates the OIDC public endpoint HTTP handler, including the
// self-service registration and social-login endpoints wired from the iam
// stores and OIDC config.
func NewOIDCMux(provider *oidc.Provider, auditLogger libaudit.Logger, result Result, cfg *config.OIDCConfig, externalURL string) http.Handler {
	return iam.NewOIDCMux(provider, auditLogger, result.iamResult.Stores, cfg, externalURL)
}
