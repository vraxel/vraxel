// Package agentgw is the agent gateway: the machine-facing protocol
// surface at /api/agent/v1/* plus the in-process registry of live
// control channels.
//
// It is deliberately NOT a REST module -- no rest.APIGroupInfo, no RBAC.
// Callers are machines authenticating with a bearer credential. The one
// user-facing surface (agent-join-tokens) is registered by the host module
// under its own permission tree.
//
// The wire contract lives in lib/agent/types so the agent binary can
// share it without linking any of this.
//
// This is the register / control-channel slice: register + channel are
// live; data-channel / bundles / jobs / scrape-targets land later and 404
// until then. The RunManager is a stub (see runmanager.go).
package agentgw

import (
	"context"
	"net/http"
	"time"

	"vraxel.io/vraxel/lib/logger"
	"vraxel.io/vraxel/lib/serverinstance"
	gwstore "vraxel.io/vraxel/pkg/apis/agentgw/store"
	"vraxel.io/vraxel/pkg/db"
)

// agentStaleAfter is when an agent with no heartbeat is presumed offline
// (design §4.2: 60s of silence). The normal path is the read loop
// noticing the closed socket and writing 'offline' immediately; this
// sweep is the backstop for when nobody got to write it -- host powered
// off mid-heartbeat, or the owning instance SIGKILLed.
const agentStaleAfter = 60 * time.Second

// agentStatusOnline is the host_agents.status value for a live channel.
const agentStatusOnline = "online"

// ModuleResult is what the assembly layer wires up.
type ModuleResult struct {
	// ProtocolHandler serves /api/agent/v1/*, mounted as a prefix branch
	// in the server's HTTP handler.
	ProtocolHandler http.HandlerFunc
	// Registry is the live control-channel table.
	Registry *Registry
	// Dispatcher runs playbooks over agents. Exposed for the host module's
	// install flow to call through an interface it declares; a stub until
	// the jobs slice.
	Dispatcher *RunManager
}

// Deps are the cross-module dependencies of the gateway.
type Deps struct {
	// HostRegistrar is the host module's hosts-row writer.
	HostRegistrar HostRegistrar
	// JoinTokens is shared with the host module's agent-join-tokens
	// resource, so the assembly layer builds it once (NewJoinTokenStore)
	// and passes the same instance to both.
	JoinTokens JoinTokenStore
	// EncryptionKey is the platform master key; the agent-token signing key
	// is derived from it.
	EncryptionKey []byte
	// ServerName is config server.name, the deployment identity component
	// of this instance's id.
	ServerName string
	// ListenAddr is the address this process serves HTTP on. It is the
	// port source for the address siblings use to reach this instance;
	// externalUrl cannot be, because behind a load balancer it names the
	// balancer rather than any one instance.
	ListenAddr string
}

// NewJoinTokenStore builds the join-token store. Exported as a top-level
// factory so pkg/apis/install.go can hand the same instance to the host
// module before agentgw itself is constructed, breaking what would
// otherwise be a circular assembly order.
func NewJoinTokenStore(d *db.DB) JoinTokenStore { return gwstore.NewPGJoinTokenStore(d) }

// NewModule boots the agent gateway: the instance lease, the
// control-channel registry, and the /api/agent/v1/ handler.
func NewModule(ctx context.Context, database *db.DB, deps Deps) ModuleResult {
	stores := gwstore.NewStores(database)
	if deps.JoinTokens != nil {
		stores.JoinToken = deps.JoinTokens
	}

	instanceID := serverinstance.BuildInstanceID(deps.ServerName)
	registry := NewRegistry(stores.Agent, instanceID)

	// Residue from this instance's previous life: rows still claiming
	// status='online' under our instance_id after a hard kill. Clear
	// before serving so addressing never points at a dead socket.
	if err := stores.Agent.MarkInstanceOffline(ctx, instanceID); err != nil {
		logger.Warnf("agentgw: clear residual online agents for %s: %v", instanceID, err)
	}

	runManager := &RunManager{}

	instReg := serverinstance.NewRegistry(database.Pool)
	lease := serverinstance.NewLease(
		instReg,
		instanceID,
		serverinstance.BuildInternalAddr(deps.ListenAddr),
		logAdapter{},
	)
	// The stale-agent sweep hangs off the lease tick: agent semantics, safe
	// on every instance concurrently since the write is DB-clock guarded.
	// The orphaned-run sweep arrives with the jobs slice.
	lease.OnTick = func(tickCtx context.Context) {
		if err := stores.Agent.MarkStaleOffline(tickCtx, agentStaleAfter); err != nil {
			logger.Warnf("agentgw: sweep stale agents: %v", err)
		}
	}
	lease.Start(ctx)

	handler := NewProtocolHandler(ctx, stores, deps.HostRegistrar,
		NewTokenSigner(deps.EncryptionKey), NewSessionTokenSigner(deps.EncryptionKey),
		registry, runManager)

	return ModuleResult{
		ProtocolHandler: handler,
		Registry:        registry,
		Dispatcher:      runManager,
	}
}

// logAdapter bridges lib/logger's package-level functions onto the
// serverinstance.Logger interface.
type logAdapter struct{}

func (logAdapter) Infof(format string, args ...any) { logger.Infof(format, args...) }
func (logAdapter) Warnf(format string, args ...any) { logger.Warnf(format, args...) }
