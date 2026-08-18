package agentgw

import (
	"context"
	"errors"
	"fmt"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
	gwstore "vraxel.io/vraxel/pkg/apis/agentgw/store"
)

// ErrHostUnreachable reports that no instance holds this host's control
// channel: the agent is offline, or its row has not caught up with a
// socket that just died.
var ErrHostUnreachable = errors.New("host has no reachable agent control channel")

// ErrHostOnAnotherInstance reports that the host IS connected, just not
// here.
//
// It exists to keep a multi-instance deployment legible. Cross-instance
// forwarding is not built yet, so this case cannot be served -- but
// collapsing it into ErrHostUnreachable would present the one failure a
// second replica introduces as the one failure it has nothing to do with,
// and "the agent is offline" is a long thing to chase when the agent is
// online and two metres away. The error names the instance instead.
var ErrHostOnAnotherInstance = errors.New("host's agent channel is held by another server instance")

// channelRoutingStore is the one method the router needs from
// gwstore.AgentStore: read where a host's channel is. Declared narrow so
// the routing decision is testable without a fake nobody reads.
// gwstore.AgentStore satisfies it structurally.
type channelRoutingStore interface {
	GetByHostID(ctx context.Context, hostID int64) (*gwstore.AgentRow, error)
}

// ChannelRouter sends a control frame to a host's agent, and owns the
// question of where that host's channel actually is.
//
// It exists so no caller ever asks "is this host's channel MINE" -- a
// question whose honest answer stops being useful the moment there is
// more than one instance. Callers ask "send this to that host"; where the
// socket lives is this type's problem. Today the only answer it can act
// on is "here", but the seam is what lets forwarding land behind Send
// without touching a single caller. Putting the local lookup in the
// callers instead is the mistake that later has to be undone everywhere
// at once.
//
// Local first, always: only a miss costs the host_agents read, so a
// single-instance deployment pays nothing.
type ChannelRouter struct {
	self     string
	registry *Registry
	agents   channelRoutingStore
}

func NewChannelRouter(self string, registry *Registry, agents channelRoutingStore) *ChannelRouter {
	return &ChannelRouter{self: self, registry: registry, agents: agents}
}

// Send delivers a frame to the host's control channel.
//
// The write is scoped to the SESSION's context, not the caller's: the
// frame belongs to the channel's lifetime, and a caller that gives up
// (browser tab closed mid-handshake) must not cancel a write already on
// the wire and desynchronise the frame stream for everyone else.
func (r *ChannelRouter) Send(ctx context.Context, hostID int64, f agenttypes.Frame) error {
	if sess := r.registry.GetByHost(hostID); sess != nil {
		return WriteFrame(sess.Context(), sess.Conn, f)
	}
	return r.classifyMiss(ctx, hostID)
}

// classifyMiss says why a local lookup came up empty. Only called when it
// did, and never returns nil.
func (r *ChannelRouter) classifyMiss(ctx context.Context, hostID int64) error {
	row, err := r.agents.GetByHostID(ctx, hostID)
	if err != nil {
		return fmt.Errorf("resolve channel holder for host %d: %w", hostID, err)
	}
	if row == nil || row.Status != agentStatusOnline || row.InstanceID == "" {
		return ErrHostUnreachable
	}
	if row.InstanceID == r.self {
		// The row names us but the registry has no session: our own socket
		// died and the row has not caught up yet. Nothing to forward to,
		// and the agent is already reconnecting.
		return ErrHostUnreachable
	}
	return fmt.Errorf("%w: host %d is on instance %s", ErrHostOnAnotherInstance, hostID, row.InstanceID)
}
