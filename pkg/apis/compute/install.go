// Package compute owns the hosts table: the operator-facing host
// resource, the pending-onboarding tokens that bring hosts into it, and
// the host registrar backing the agent gateway's machine-facing
// /register.
package compute

import (
	"context"

	"vraxel.io/vraxel/lib/apiserver"
	"vraxel.io/vraxel/lib/pgnotify"
	"vraxel.io/vraxel/lib/statushub"
	"vraxel.io/vraxel/pkg/apis/agentgw"
	modstore "vraxel.io/vraxel/pkg/apis/compute/store"
	"vraxel.io/vraxel/pkg/db"
)

// ModuleResult is what the assembly layer wires up.
type ModuleResult struct {
	// Hub is the in-process end of the host watch stream. Handed back to
	// Registrar, which binds it into the /hosts/watch route.
	Hub *statushub.Hub
}

// NewModule starts the module's background half: the bridge from
// cross-instance host events to this instance's watchers.
//
// Separate from Registrar because the two run at different times and need
// different things. Registration must work with no database at all (that
// is how openapi-gen reads the route table), while the bridge needs both
// a database and the pgnotify multiplexer, and must be subscribed before
// mux.Start.
func NewModule(ctx context.Context, database *db.DB, mux *pgnotify.Multiplexer) ModuleResult {
	return ModuleResult{Hub: StartHostWatch(ctx, mux, modstore.NewPGHostStore(database))}
}

// NewAgentHostRegistrar builds compute's implementation of the agent
// gateway's HostRegistrar. Lives here (rather than in agent_registrar.go)
// because install.go is the only business-layer file permitted to touch
// pkg/db.
func NewAgentHostRegistrar(d *db.DB) agentgw.HostRegistrar {
	return &agentHostRegistrar{store: modstore.NewPGAgentHostStore(d)}
}

// Registrar returns the module's route-registration closure. Building
// the stores only wraps the handle, so a nil database yields a registrar
// that declares every route without touching Postgres -- that is what
// lets openapi-gen read the real route table offline.
// serverURL is server.externalUrl: where agents reach this deployment.
// Passed in rather than read from the config global so the module has no
// opinion about where configuration comes from, matching how agentgw
// receives ServerName.
// hub is the watch stream from NewModule. It is required: the /watch
// route binds it at registration, so a caller with nothing to stream
// passes an unattached hub rather than nil.
func Registrar(database *db.DB, serverURL string, hub *statushub.Hub) func(*apiserver.Server) {
	hosts := modstore.NewPGHostStore(database)
	// host_agents belongs to the gateway; the merge reaches it through
	// the same top-level factory the join-token store uses, so no
	// cross-module store import is needed.
	agents := agentgw.NewAgentStore(database)
	agentHosts := modstore.NewPGAgentHostStore(database)
	// The join-token table belongs to the gateway, which owns the
	// /register path that consumes them. compute reaches it through the
	// store interface rather than pkg/db, the same way every other
	// cross-module data path in the tree works.
	tokens := agentgw.NewJoinTokenStore(database)
	return func(s *apiserver.Server) {
		apiserver.Register(s, HostsDef(hosts, agentHosts, agents, hub))
		apiserver.Register(s, AgentJoinTokensDef(tokens, hosts, serverURL))
	}
}
