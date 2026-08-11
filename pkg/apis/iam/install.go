package iam

import (
	"context"

	"net/http"
	"strconv"
	"strings"

	"vraxel.io/vraxel/lib/apiserver"
	"vraxel.io/vraxel/lib/config"
	"vraxel.io/vraxel/lib/logger"
	"vraxel.io/vraxel/lib/oidc"
	"vraxel.io/vraxel/lib/pgnotify"
	"vraxel.io/vraxel/lib/rest/filters"
	"vraxel.io/vraxel/pkg/apis/iam/store"
	"vraxel.io/vraxel/pkg/db"
)

// ModuleResult holds the output of IAM module initialization.
type ModuleResult struct {
	Register func(*apiserver.Server)
	Stores   store.Stores
}

// NewModule initializes the IAM module: builds the registration closure
// and seeds the admin user + built-in roles. Permission sync is handled
// centrally by apis (SyncPermissions) after all modules are registered.
func NewModule(ctx context.Context, database *db.DB) ModuleResult {
	stores := store.NewStores(database)

	adminCfg := config.Get().Admin
	if err := SeedAdmin(ctx, stores.User, adminCfg); err != nil {
		logger.Fatalf("cannot seed admin user: %v", err)
	}

	if err := SeedRBAC(ctx, stores.Role); err != nil {
		logger.Fatalf("cannot seed RBAC: %v", err)
	}

	return ModuleResult{
		Register: newRegistrar(stores),
		Stores:   stores,
	}
}

// newRegistrar returns the registration closure over typed ResourceDefs.
// workspaces / namespaces are REAL iam resources whose nested-tree URLs
// coincide exactly with the framework's scope prefixes, so the per-scope
// defs register as scoped resources and the scope-stripped permission
// codes (iam:namespaces:* etc.) fall out of registration depth.
func newRegistrar(s store.Stores) func(*apiserver.Server) {
	return func(srv *apiserver.Server) {
		// users: platform CRUD + password actions + read verbs
		// (self-user rule on ExtraAllow, Sensitive bodies).
		apiserver.Register(srv, UsersDef(s))
		// workspaces / namespaces: real iam resources whose platform
		// list injects the binding-scoped AccessFilter for non-admins.
		// The workspace-level namespaces registration provides the
		// /workspaces/{id}/namespaces nesting.
		apiserver.Register(srv, WorkspacesDef(s.Workspace, s.User, s.RoleBinding))
		apiserver.Register(srv, NamespacesDef(s.Namespace, s.Workspace, s.User, s.RoleBinding))
		// Scoped member/rolebinding/role surfaces: distinct ops per
		// scope; each registers at its own nesting level.
		apiserver.Register(srv, WorkspaceUsersDef(s.RoleBinding, s.User))
		apiserver.Register(srv, NamespaceUsersDef(s.RoleBinding, s.Namespace, s.User))
		apiserver.Register(srv, RoleBindingsDef(s.RoleBinding, s.Role))
		apiserver.Register(srv, WorkspaceRoleBindingsDef(s.RoleBinding, s.Role))
		apiserver.Register(srv, NamespaceRoleBindingsDef(s.RoleBinding, s.Role, s.Namespace))
		apiserver.Register(srv, RolesDef(s.Role, s.RoleBinding, s.Permission))
		apiserver.Register(srv, ScopedRolesDef(s.Role, s.RoleBinding, s.Permission, store.ScopeWorkspace))
		apiserver.Register(srv, ScopedRolesDef(s.Role, s.RoleBinding, s.Permission, store.ScopeNamespace))
		apiserver.Register(srv, PermissionsDef(s.Permission))
	}
}

// selfUserAllow: a user may always read their own user resource (get +
// read verbs, which inherit users:get) and change their own password,
// regardless of RBAC bindings.
func selfUserAllow(permCode string, r *http.Request, userID int64) bool {
	if permCode != "iam:users:get" && permCode != "iam:users:change-password" {
		return false
	}
	raw := r.PathValue("userId")
	if colon := strings.IndexByte(raw, ':'); colon > 0 {
		raw = raw[:colon]
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	return err == nil && id == userID
}

// NewAuthorizerFromResult creates the Authorizer from the IAM module
// result. Subscribes to rbac_invalidate via the shared pgnotify
// multiplexer so RBAC cache entries across vraxel-server instances are
// invalidated within Postgres NOTIFY latency (ms).
func NewAuthorizerFromResult(mux *pgnotify.Multiplexer, result ModuleResult) *filters.Authorizer {
	authorizer, checker := NewAuthorizer(result.Stores.RoleBinding)
	store.RBACInvalidateChannel.Subscribe(mux, checker.InvalidateUser)
	return authorizer
}

// NewOIDCProvider creates the OIDC provider with all internal store wiring.
// Keys are auto-generated and stored in the database. The issuer is
// always set (config.SetDefaults derives it from server.externalUrl), so
// authentication is unconditionally on.
func NewOIDCProvider(database *db.DB, result ModuleResult, cfg *config.OIDCConfig) *oidc.Provider {
	providerCfg, err := oidc.ParseConfig(cfg)
	if err != nil {
		logger.Fatalf("invalid OIDC config: %v", err)
	}

	keyStore := oidc.NewDBKeyStore(database.Pool, database.Queries)
	keySet, err := keyStore.LoadOrGenerate(cfg.Algorithm)
	if err != nil {
		logger.Fatalf("cannot load/generate OIDC keys: %v", err)
	}

	logger.Infof("OIDC keys ready (algorithm=%s, kid=%s)", keySet.Algorithm, keySet.KeyID)

	sessionStore := oidc.NewPGSessionStore(database.Queries)
	codeStore := oidc.NewPGAuthCodeStore(database.Queries)
	pendingStore := oidc.NewPGPendingStore(database.Queries)

	provider := oidc.NewProvider(providerCfg, keySet,
		NewUserLookupAdapter(result.Stores.User),
		NewRefreshTokenAdapter(result.Stores.RefreshToken),
		sessionStore, codeStore, pendingStore,
	)
	provider.SetClients(oidc.ParseClients(cfg.Clients))
	provider.SetLoginThrottle(oidc.NewPGLoginThrottleStore(database.Queries))

	logger.Infof("OIDC provider initialized (issuer=%s)", cfg.Issuer)
	return provider
}
