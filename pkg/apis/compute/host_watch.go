package compute

import (
	"context"
	"encoding/json"
	"strconv"

	apierrors "vraxel.io/vraxel/lib/api/errors"
	"vraxel.io/vraxel/lib/logger"
	"vraxel.io/vraxel/lib/pgnotify"
	"vraxel.io/vraxel/lib/rest"
	"vraxel.io/vraxel/lib/statushub"
	ws "vraxel.io/vraxel/lib/websocket"
	modstore "vraxel.io/vraxel/pkg/apis/compute/store"
	"vraxel.io/vraxel/pkg/apis/shared/hostevent"
)

// watchEvent is the wire format of one event, sent as a MsgData frame.
//
// It carries no host fields on purpose: a client refetches on receipt, so
// the socket never becomes a second, subtly different source of truth for
// what a host looks like. Deleted is the exception because there is
// nothing left to fetch.
type watchEvent struct {
	EntityType string `json:"entityType"`
	EntityID   int64  `json:"entityId"`
	Deleted    bool   `json:"deleted,omitempty"`
}

// scopeResolver reads a host's tenancy. Satisfied by modstore.HostStore.
type scopeResolver interface {
	GetScope(ctx context.Context, id int64) (*modstore.HostScope, error)
}

// StartHostWatch wires cross-instance host events onto an in-process hub
// and returns the hub the WebSocket handler subscribes to.
//
// The two halves exist for different reasons. pgnotify carries an event
// to every instance, because the instance holding an agent's channel is
// rarely the one holding the operator's browser tab. The hub decides
// which of this instance's watchers may see it, because that is a
// tenancy question and pgnotify has no idea what a workspace is.
//
// hosts resolves the tenancy of events that arrive without one -- every
// event from agentgw, which owns host_agents and cannot see the hosts
// table's scope columns. That read is why the subscription is the async
// kind: a blocking handler on the shared dispatch goroutine would stall
// every other channel in the process, RBAC invalidation included.
func StartHostWatch(ctx context.Context, mux *pgnotify.Multiplexer, hosts scopeResolver) *statushub.Hub {
	hub := statushub.New()
	if mux == nil {
		return hub
	}
	hostevent.Channel.SubscribeAsync(mux, ctx, nil, func(ev hostevent.Event) {
		publishHostEvent(ctx, hub, hosts, ev)
	})
	return hub
}

func publishHostEvent(ctx context.Context, hub *statushub.Hub, hosts scopeResolver, ev hostevent.Event) {
	out := statushub.Event{
		EntityType:  "host",
		EntityID:    ev.HostID,
		Deleted:     ev.Deleted,
		Scope:       ev.Scope,
		WorkspaceID: ev.WorkspaceID,
		NamespaceID: ev.NamespaceID,
	}
	if out.Scope == "" {
		sc, err := hosts.GetScope(ctx, ev.HostID)
		if err != nil {
			// The host was deleted between the write and this lookup. Its
			// own deletion event carries a scope, so dropping this one
			// loses nothing.
			if se := apierrors.FromDomain(err, "host"); se == nil || !apierrors.IsNotFound(se) {
				logger.Warnf("compute: host watch scope lookup for host %d: %v", ev.HostID, err)
			}
			return
		}
		out.Scope, out.WorkspaceID, out.NamespaceID = sc.Scope, sc.WorkspaceID, sc.NamespaceID
	}
	hub.Publish(out)
}

// NewHostWatchHandler streams host changes to one client.
//
//	GET /api/compute/v1/hosts/watch
//	GET /api/compute/v1/workspaces/{workspaceId}/hosts/watch
//	GET /api/compute/v1/workspaces/{workspaceId}/namespaces/{namespaceId}/hosts/watch
//
// The scope in the URL is the scope watched, and it is the same one the
// list endpoint at that URL reads -- which is why the route borrows
// compute:hosts:list rather than inventing a permission of its own.
func NewHostWatchHandler(hub *statushub.Hub) rest.WebSocketHandler {
	return func(ctx context.Context, params map[string]string, conn *ws.Conn) {
		defer conn.Close(ws.StatusNormalClosure, "")

		ch, unsub := hub.Subscribe(statushub.ScopeKey(resolveWatchScope(params)))
		defer unsub()

		streamHostEvents(ctx, conn, ch, watchReadPump(ctx, conn))
	}
}

// watchReadPump drains client frames. Nothing is expected on this socket,
// but reads have to be consumed for a close frame to be noticed at all;
// the returned channel closes when the client goes away.
func watchReadPump(ctx context.Context, conn *ws.Conn) <-chan struct{} {
	disconnected := make(chan struct{})
	go func() {
		defer close(disconnected)
		for {
			if _, _, err := conn.ReadMessage(ctx); err != nil {
				return
			}
		}
	}()
	return disconnected
}

func streamHostEvents(ctx context.Context, conn *ws.Conn, ch <-chan statushub.Event, disconnected <-chan struct{}) {
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return
			}
			data, err := json.Marshal(watchEvent{
				EntityType: ev.EntityType,
				EntityID:   ev.EntityID,
				Deleted:    ev.Deleted,
			})
			if err != nil {
				continue
			}
			if wErr := conn.WriteBinary(ctx, ws.EncodeMessage(ws.MsgData, data)); wErr != nil {
				return
			}
		case <-disconnected:
			return
		case <-ctx.Done():
			return
		}
	}
}

// resolveWatchScope reads the scope out of the path params. A malformed
// id degrades to the scope above it, which the authz layer has already
// vetted the caller for; it never widens past what the URL addressed.
func resolveWatchScope(params map[string]string) (string, *int64, *int64) {
	wsID, ok := parseScopeID(params["workspaceId"])
	if !ok {
		return "platform", nil, nil
	}
	nsID, ok := parseScopeID(params["namespaceId"])
	if !ok {
		return "workspace", &wsID, nil
	}
	return "namespace", &wsID, &nsID
}

func parseScopeID(raw string) (int64, bool) {
	if raw == "" {
		return 0, false
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, false
	}
	return id, true
}
