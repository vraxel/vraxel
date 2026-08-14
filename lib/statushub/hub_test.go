package statushub

import "testing"

func ptr(v int64) *int64 { return &v }

// A namespace event has to reach three subscribers and no others: its own
// namespace, the workspace above it, and platform. The sibling namespace
// must not see it -- that is the tenancy boundary, and it is invisible in
// any test that only checks "the event arrived somewhere".
func TestNamespaceEventReachesEveryScopeAboveItAndNoSibling(t *testing.T) {
	h := New()

	own, stopOwn := h.Subscribe("ns:1:2")
	defer stopOwn()
	sibling, stopSibling := h.Subscribe("ns:1:3")
	defer stopSibling()
	workspace, stopWorkspace := h.Subscribe("ws:1")
	defer stopWorkspace()
	platform, stopPlatform := h.Subscribe("platform")
	defer stopPlatform()

	h.Publish(Event{
		EntityType: "host", EntityID: 7,
		Scope: "namespace", WorkspaceID: ptr(1), NamespaceID: ptr(2),
	})

	for name, ch := range map[string]<-chan Event{"own": own, "workspace": workspace, "platform": platform} {
		select {
		case ev := <-ch:
			if ev.EntityID != 7 {
				t.Fatalf("%s: EntityID = %d, want 7", name, ev.EntityID)
			}
		default:
			t.Fatalf("%s subscriber got no event", name)
		}
	}
	select {
	case ev := <-sibling:
		t.Fatalf("sibling namespace saw host %d from another namespace", ev.EntityID)
	default:
	}
}

// A platform event stays at platform. A workspace watcher that received
// it would refetch on every unrelated host in the deployment.
func TestPlatformEventDoesNotReachWorkspaceWatchers(t *testing.T) {
	h := New()
	workspace, stop := h.Subscribe("ws:1")
	defer stop()
	platform, stopPlatform := h.Subscribe("platform")
	defer stopPlatform()

	h.Publish(Event{EntityType: "host", EntityID: 1, Scope: "platform"})

	select {
	case <-platform:
	default:
		t.Fatal("platform subscriber got no event")
	}
	select {
	case <-workspace:
		t.Fatal("workspace subscriber saw a platform event")
	default:
	}
}

// A workspace-scoped event with no workspace id cannot be routed to a
// workspace. Falling back to platform keeps it visible to an admin
// instead of dropping it silently.
func TestScopeWithoutIDsFallsBackToPlatform(t *testing.T) {
	h := New()
	platform, stop := h.Subscribe("platform")
	defer stop()

	h.Publish(Event{EntityType: "host", EntityID: 1, Scope: "workspace"})

	select {
	case <-platform:
	default:
		t.Fatal("malformed workspace event vanished instead of landing at platform")
	}
}

// Publish must not block on a subscriber that stopped reading, or one
// dead WebSocket would stall every other watcher in the process.
func TestSlowSubscriberIsDroppedNotBlocking(t *testing.T) {
	h := New()
	ch, stop := h.Subscribe("platform")
	defer stop()

	for i := 0; i < channelBufferSize+10; i++ {
		h.Publish(Event{EntityType: "host", EntityID: int64(i), Scope: "platform"})
	}
	if got := len(ch); got != channelBufferSize {
		t.Fatalf("buffered %d events, want the buffer size %d", got, channelBufferSize)
	}
}

// unsubscribe is wired to defer AND called explicitly on some paths;
// closing twice would panic and take the process with it.
func TestUnsubscribeIsIdempotent(t *testing.T) {
	h := New()
	_, stop := h.Subscribe("platform")
	stop()
	stop()
}
