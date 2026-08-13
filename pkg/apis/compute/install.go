// Package compute owns the hosts table. For now it carries only the agent
// self-registration path (the host registrar backing the agent gateway);
// the full host-management surface lands with its own slice.
package compute

import (
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
