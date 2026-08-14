package compute

import (
	"context"
	"fmt"
	"testing"

	"vraxel.io/vraxel/lib/statushub"
	modstore "vraxel.io/vraxel/pkg/apis/compute/store"
	"vraxel.io/vraxel/pkg/apis/shared/hostevent"
	"vraxel.io/vraxel/pkg/db/pgerrors"
)

type fakeScopeResolver struct {
	scope  *modstore.HostScope
	err    error
	lookup int
}

func (f *fakeScopeResolver) GetScope(context.Context, int64) (*modstore.HostScope, error) {
	f.lookup++
	if f.err != nil {
		return nil, f.err
	}
	return f.scope, nil
}

func int64p(v int64) *int64 { return &v }

// agentgw publishes without a scope, because it owns host_agents and has
// no business reading the hosts table's tenancy columns. If the bridge
// forgot to fill it in, every agent online/offline would be routed to
// platform and a workspace operator would never see their own hosts
// change state.
func TestAgentEventWithoutScopeIsRoutedByTheHostsOwnTenancy(t *testing.T) {
	hub := statushub.New()
	ws, stop := hub.Subscribe("ws:4")
	defer stop()
	hosts := &fakeScopeResolver{scope: &modstore.HostScope{Scope: "workspace", WorkspaceID: int64p(4)}}

	publishHostEvent(context.Background(), hub, hosts, hostevent.Event{HostID: 9})

	select {
	case ev := <-ws:
		if ev.EntityType != "host" || ev.EntityID != 9 {
			t.Fatalf("got %+v, want a host event for 9", ev)
		}
	default:
		t.Fatal("workspace watcher saw nothing; the event was routed elsewhere")
	}
}

// A deletion carries its own tenancy because the row that would answer a
// lookup is already gone. Looking it up anyway would drop every delete.
func TestDeletionUsesTheScopeItCarriesAndDoesNotLookUp(t *testing.T) {
	hub := statushub.New()
	ns, stop := hub.Subscribe("ns:4:5")
	defer stop()
	hosts := &fakeScopeResolver{err: fmt.Errorf("host 9: %w", pgerrors.ErrNotFound)}

	publishHostEvent(context.Background(), hub, hosts, hostevent.Event{
		HostID: 9, Scope: "namespace", WorkspaceID: int64p(4), NamespaceID: int64p(5), Deleted: true,
	})

	select {
	case ev := <-ns:
		if !ev.Deleted {
			t.Fatal("Deleted was dropped; the client would refetch a 404 instead of removing the row")
		}
	default:
		t.Fatal("namespace watcher saw nothing")
	}
	if hosts.lookup != 0 {
		t.Fatalf("looked the scope up %d times for a row that no longer exists", hosts.lookup)
	}
}

// A host deleted between the write and the lookup has already had its own
// deletion event published, so dropping this one is right -- and it must
// not be published to platform, which is where an unfilled scope lands.
func TestEventForAVanishedHostIsDropped(t *testing.T) {
	hub := statushub.New()
	platform, stop := hub.Subscribe("platform")
	defer stop()
	hosts := &fakeScopeResolver{err: fmt.Errorf("host 9: %w", pgerrors.ErrNotFound)}

	publishHostEvent(context.Background(), hub, hosts, hostevent.Event{HostID: 9})

	select {
	case ev := <-platform:
		t.Fatalf("published %+v for a host that no longer exists", ev)
	default:
	}
}

func TestResolveWatchScope(t *testing.T) {
	cases := []struct {
		name   string
		params map[string]string
		scope  string
		ws, ns *int64
	}{
		{"platform", map[string]string{}, "platform", nil, nil},
		{"workspace", map[string]string{"workspaceId": "4"}, "workspace", int64p(4), nil},
		{"namespace", map[string]string{"workspaceId": "4", "namespaceId": "5"}, "namespace", int64p(4), int64p(5)},
		// A namespace id that is not a number cannot address a namespace.
		// Falling back to the workspace keeps the watcher inside what the
		// URL (and the authz check on it) already covered.
		{"bad namespace id", map[string]string{"workspaceId": "4", "namespaceId": "x"}, "workspace", int64p(4), nil},
		{"bad workspace id", map[string]string{"workspaceId": "x", "namespaceId": "5"}, "platform", nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			scope, ws, ns := resolveWatchScope(tc.params)
			if scope != tc.scope || !idEq(ws, tc.ws) || !idEq(ns, tc.ns) {
				t.Fatalf("got (%s, %v, %v), want (%s, %v, %v)",
					scope, deref(ws), deref(ns), tc.scope, deref(tc.ws), deref(tc.ns))
			}
		})
	}
}

func idEq(a, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func deref(p *int64) any {
	if p == nil {
		return nil
	}
	return *p
}
