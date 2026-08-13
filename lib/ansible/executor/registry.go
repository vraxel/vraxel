package executor

import (
	"sync"

	"vraxel.io/vraxel/lib/ansible/connector"
)

// connectorRegistry is the per-execution host->Connector map, guarded
// by a RWMutex so that mutators (PlaybookExecutor.initConnectors /
// closeConnectors) and readers (TaskExecutor task workers,
// gatherFacts, plus out-of-band PlaybookExecutor.Resize /
// SetLiveOutput from a WS-driven goroutine) cannot trigger a
// concurrent-map-access panic.
//
// Pre-registry, the bare map was only safe because all access was
// serialized through PlaybookExecutor.Execute. Adding live PTY
// resize / output streaming introduced a goroutine that reads the
// map outside that flow; a registry encapsulates the locking so
// every callsite is automatically race-free.
type connectorRegistry struct {
	mu sync.RWMutex
	m  map[string]connector.Connector
}

func newConnectorRegistry() *connectorRegistry {
	return &connectorRegistry{m: make(map[string]connector.Connector)}
}

// Get returns the connector registered for host, or nil if none.
func (r *connectorRegistry) Get(host string) connector.Connector {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.m[host]
}

// Has reports whether a connector is registered for host.
func (r *connectorRegistry) Has(host string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.m[host]
	return ok
}

// Put stores c under host, overwriting any previous registration.
func (r *connectorRegistry) Put(host string, c connector.Connector) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[host] = c
}

// Delete removes host from the registry and returns the previous
// connector (nil when host was absent).
func (r *connectorRegistry) Delete(host string) connector.Connector {
	r.mu.Lock()
	defer r.mu.Unlock()
	c := r.m[host]
	if c != nil {
		delete(r.m, host)
	}
	return c
}

// Drain removes every registration and returns the connectors it held.
// Play teardown uses it instead of deleting a known host list because a
// delegate_to target can add a connector the play never listed, and that
// one has to be closed too.
func (r *connectorRegistry) Drain() []connector.Connector {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]connector.Connector, 0, len(r.m))
	for host, c := range r.m {
		out = append(out, c)
		delete(r.m, host)
	}
	return out
}

// Snapshot returns a slice copy of every registered connector.
// Iterating the slice is safe under concurrent Put/Delete because
// the slice itself is independent of the map.
func (r *connectorRegistry) Snapshot() []connector.Connector {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]connector.Connector, 0, len(r.m))
	for _, c := range r.m {
		out = append(out, c)
	}
	return out
}
