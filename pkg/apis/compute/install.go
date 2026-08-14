// Package compute owns the hosts table: the operator-facing host
// resource, the pending-onboarding tokens that bring hosts into it, and
// the host registrar backing the agent gateway's machine-facing
// /register.
package compute

import (
	"vraxel.io/vraxel/lib/apiserver"
	"vraxel.io/vraxel/pkg/apis/agentgw"
	modstore "vraxel.io/vraxel/pkg/apis/compute/store"
	"vraxel.io/vraxel/pkg/db"
)

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
func Registrar(database *db.DB) func(*apiserver.Server) {
	hosts := modstore.NewPGHostStore(database)
	// The join-token table belongs to the gateway, which owns the
	// /register path that consumes them. compute reaches it through the
	// store interface rather than pkg/db, the same way every other
	// cross-module data path in the tree works.
	tokens := agentgw.NewJoinTokenStore(database)
	return func(s *apiserver.Server) {
		apiserver.Register(s, HostsDef(hosts))
		apiserver.Register(s, AgentJoinTokensDef(tokens, hosts))
	}
}
