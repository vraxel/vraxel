// Package hostevent declares the cross-instance notification that a host
// changed.
//
// It sits in shared/ because the two modules that write host state can
// not see each other: compute owns the hosts table, agentgw owns
// host_agents, and agentgw must not import compute (the dependency runs
// the other way). A neutral package is what lets both publish onto the
// same channel and lets compute be the single subscriber.
//
// This file is also the seam for the transport. Everything above it
// speaks Event; only the channel declaration knows the delivery is
// Postgres LISTEN/NOTIFY. Moving to another bus is a change here and at
// the eleven publish lines, not in any handler.
package hostevent

import "vraxel.io/vraxel/lib/pgnotify"

// Event says a host row -- or the agent bound to it -- changed. It is a
// refetch hint, not a diff: no field describes WHAT changed, and
// subscribers respond by re-reading through the normal API.
//
// That is a deliberate contract, and it is what makes the publishers
// cheap to get right. A duplicate event costs one redundant read; a lost
// event (pgnotify drops whatever fires during a reconnect) costs a stale
// row until the next one. Neither can produce a wrong row, so publishers
// may err towards publishing rather than tracking exactly what changed.
type Event struct {
	HostID int64 `json:"id"`

	// Scope / WorkspaceID / NamespaceID are the host's tenancy, used for
	// delivery routing only. They are OPTIONAL: a publisher that already
	// holds them fills them in, and the subscriber looks up whatever is
	// missing.
	//
	// A deletion MUST carry them, because by the time the event is
	// handled the row that would answer the lookup is gone.
	Scope       string `json:"scope,omitempty"`
	WorkspaceID *int64 `json:"wsId,omitempty"`
	NamespaceID *int64 `json:"nsId,omitempty"`

	Deleted bool `json:"deleted,omitempty"`
}

// Channel is the LISTEN/NOTIFY channel every host change goes through.
// Published from compute/store and agentgw/store; subscribed once, by
// compute's watch bridge.
var Channel = pgnotify.NewChannel[Event]("compute_hosts_event")
