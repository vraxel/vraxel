// Package statushub is the in-process fan-out between "something
// changed" and the WebSocket connections watching for it.
//
// It is deliberately process-local and deliberately dumb: it holds no
// state about the entities it carries and never touches a database. The
// cross-instance half is a pgnotify channel, which every instance
// re-emits onto its own hub (see pkg/apis/compute/host_watch.go); the
// hub's only job from there is deciding which of this process's
// subscribers may see the event.
package statushub

import (
	"fmt"
	"sync"
)

// channelBufferSize is how far one subscriber may fall behind before its
// events are dropped. Dropping is safe because an event says only "this
// entity changed" -- the client refetches on receipt, so a dropped event
// costs at most a stale row until the next one, and never a wrong one.
const channelBufferSize = 64

// Event is one entity change.
type Event struct {
	// EntityType lets one hub carry several resources; subscribers filter
	// on it. Today only "host" is published.
	EntityType string
	EntityID   int64
	// Deleted marks a row that is gone, so a client can drop it from a
	// cached list instead of refetching a 404.
	Deleted bool

	// Scope / WorkspaceID / NamespaceID are the entity's tenancy, and the
	// only thing routing is based on.
	Scope       string
	WorkspaceID *int64
	NamespaceID *int64
}

// Hub fans events out to the subscribers whose scope can see them.
type Hub struct {
	mu          sync.RWMutex
	subscribers map[string]map[*chan Event]struct{}
}

func New() *Hub {
	return &Hub{subscribers: make(map[string]map[*chan Event]struct{})}
}

// Publish delivers an event to every scope that can see the entity.
func (h *Hub) Publish(event Event) {
	keys := visibilityKeys(event)
	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, key := range keys {
		for chp := range h.subscribers[key] {
			select {
			case *chp <- event:
			default:
				// Subscriber too slow; see channelBufferSize.
			}
		}
	}
}

// visibilityKeys returns every scope key whose subscribers can see this
// event. It mirrors the SQL visibility rules of the list endpoint:
//   - an entity in namespace N (under workspace W) is visible from
//     namespace N, from workspace W, and from platform;
//   - an entity in workspace W is visible from W and from platform;
//   - a platform entity is visible from platform only.
//
// Getting this wrong in the narrow direction is what makes a watcher
// silently stop updating; in the wide direction it leaks the existence
// of another tenant's rows. Neither is visible in a smoke test, which is
// why the rules live in one function with a test rather than being
// re-derived per publisher.
func visibilityKeys(event Event) []string {
	switch event.Scope {
	case "namespace":
		if event.WorkspaceID != nil && event.NamespaceID != nil {
			return []string{
				fmt.Sprintf("ns:%d:%d", *event.WorkspaceID, *event.NamespaceID),
				fmt.Sprintf("ws:%d", *event.WorkspaceID),
				"platform",
			}
		}
	case "workspace":
		if event.WorkspaceID != nil {
			return []string{fmt.Sprintf("ws:%d", *event.WorkspaceID), "platform"}
		}
	}
	return []string{"platform"}
}

// Subscribe returns a channel of the events visible at scope, and the
// function that stops the subscription. The channel is closed by
// unsubscribe, so a reader ranging over it terminates.
func (h *Hub) Subscribe(scope string) (ch <-chan Event, unsubscribe func()) {
	c := make(chan Event, channelBufferSize)

	h.mu.Lock()
	if h.subscribers[scope] == nil {
		h.subscribers[scope] = make(map[*chan Event]struct{})
	}
	h.subscribers[scope][&c] = struct{}{}
	h.mu.Unlock()

	var once sync.Once
	unsubscribe = func() {
		once.Do(func() {
			h.mu.Lock()
			if subs, ok := h.subscribers[scope]; ok {
				delete(subs, &c)
				if len(subs) == 0 {
					delete(h.subscribers, scope)
				}
			}
			h.mu.Unlock()
			close(c)
		})
	}
	return c, unsubscribe
}

// ScopeKey is the subscription key for one scope. Subscribers build it
// from the URL they were reached at; publishers never call it (Publish
// derives every key the event is visible from).
func ScopeKey(scope string, wsID, nsID *int64) string {
	switch scope {
	case "workspace":
		if wsID != nil {
			return fmt.Sprintf("ws:%d", *wsID)
		}
	case "namespace":
		if wsID != nil && nsID != nil {
			return fmt.Sprintf("ns:%d:%d", *wsID, *nsID)
		}
	}
	return "platform"
}
