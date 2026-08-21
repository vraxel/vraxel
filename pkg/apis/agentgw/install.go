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
// Live so far: register, the control channel, and the data channel that
// carries interactive streams. bundles / jobs / scrape-targets land later
// and 404 until then. The RunManager is a stub (see runmanager.go).
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
	// InstallScriptHandler serves GET /install-agent.sh. It sits at the
	// root rather than under /api/agent/v1/ because an operator pastes
	// the URL into a shell, and a path with an api version in it invites
	// them to think it is one.
	InstallScriptHandler http.HandlerFunc

	// ProtocolHandler serves /api/agent/v1/*, mounted as a prefix branch
	// in the server's HTTP handler.
	ProtocolHandler http.HandlerFunc
	// Registry is the live control-channel table.
	Registry *Registry
	// Dispatcher runs playbooks over agents. Exposed for the host module's
	// install flow to call through an interface it declares; a stub until
	// the jobs slice.
	Dispatcher *RunManager
	// DataHub opens streams on agents' data channels. Exposed so the host
	// module's terminal can reach it through an interface it declares --
	// compute must not import this package's handlers.
	DataHub *DataHub
}

// Deps are the cross-module dependencies of the gateway.
type Deps struct {
	// HostRegistrar is the host module's hosts-row writer.
	HostRegistrar HostRegistrar
	// HostScopes resolves a host's tenancy for the alert evaluator, so a
	// workspace rule stays inside its workspace. Same seam as
	// HostRegistrar: compute implements it, agentgw consumes it.
	HostScopes HostScopes
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
	// MetricsPushURL is config metrics.pushUrl: the VictoriaMetrics
	// address hosts push their collector output to. Empty is the lite
	// tier -- scrape-targets then tells agents to push nothing.
	MetricsPushURL string
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

// NewAgentStore builds the host_agents store. Exported for the same
// reason as NewJoinTokenStore: compute's host merge has to move an agent
// binding, and host_agents belongs to this module.
func NewAgentStore(d *db.DB) AgentStore { return gwstore.NewPGAgentStore(d) }

// NewModule boots the agent gateway: the instance lease, the
// control-channel registry, and the /api/agent/v1/ handler.
func NewModule(ctx context.Context, database *db.DB, deps Deps) ModuleResult {
	stores := gwstore.NewStores(database)
	if deps.JoinTokens != nil {
		stores.JoinToken = deps.JoinTokens
	}

	instanceID := serverinstance.BuildInstanceID(deps.ServerName)
	registry := NewRegistry(stores.Agent, instanceID)

	// Residue from any instance that died without closing its sockets,
	// this process's own previous life included. Cleared before serving so
	// addressing never points at a dead socket. Instances holding a live
	// lease are left alone, so this is safe to run on every boot.
	if err := stores.Agent.MarkOrphansOffline(ctx, serverinstance.StaleAfter); err != nil {
		logger.Warnf("agentgw: clear residual online agents: %v", err)
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
		if err := stores.Alert.SweepDisabled(tickCtx); err != nil {
			logger.Warnf("agentgw: sweep disabled alert states: %v", err)
		}
	}
	lease.Start(ctx)

	// Registry -> router -> hub, in that order: the router resolves where a
	// host's channel is, and the hub asks it to bring the data channel up.
	router := NewChannelRouter(instanceID, registry, stores.Agent)
	dataHub := NewDataHub(router)

	handler, installScript := NewProtocolHandler(ctx, stores, deps.HostRegistrar,
		NewTokenSigner(deps.EncryptionKey), NewSessionTokenSigner(deps.EncryptionKey),
		registry, runManager, dataHub, newAlertEvaluator(stores.Alert, deps.HostScopes),
		deps.MetricsPushURL, deps.ServerName)

	return ModuleResult{
		ProtocolHandler:      handler,
		InstallScriptHandler: installScript,
		Registry:             registry,
		Dispatcher:           runManager,
		DataHub:              dataHub,
	}
}

// logAdapter bridges lib/logger's package-level functions onto the
// serverinstance.Logger interface.
type logAdapter struct{}

func (logAdapter) Infof(format string, args ...any) { logger.Infof(format, args...) }
func (logAdapter) Warnf(format string, args ...any) { logger.Warnf(format, args...) }
