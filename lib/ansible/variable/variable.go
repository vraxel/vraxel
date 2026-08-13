package variable

import (
	"sync"

	"vraxel.io/vraxel/lib/ansible"
)

// GetFunc retrieves data from a Variable's Value.
type GetFunc func(v *Value) any

// MergeFunc mutates a Variable's Value.
type MergeFunc func(v *Value)

// Variable provides thread-safe access to playbook execution variables.
// Get acquires a read lock; Merge acquires a write lock.
type Variable interface {
	Get(fn GetFunc) any
	Merge(fn MergeFunc)
}

// Value holds all variable data for a playbook execution.
type Value struct {
	// Inventory is the parsed inventory for the playbook.
	Inventory ansible.Inventory
	// Hosts stores per-host variables (remote + runtime).
	Hosts map[string]*HostVars
	// Result stores global result variables set during execution.
	Result map[string]any
}

// HostVars holds variables for a single host.
type HostVars struct {
	// DefaultVars are loaded from role defaults/main.yml.
	// They have the lowest priority and can be overridden by anything.
	DefaultVars map[string]any
	// RemoteVars are collected from the remote host (e.g. gather_facts).
	// They are read-only after being set.
	RemoteVars map[string]any
	// RuntimeVars are set during playbook execution (e.g. set_fact, block vars, include_vars, role vars).
	RuntimeVars map[string]any
}

// variable is the concrete, thread-safe implementation of Variable.
type variable struct {
	mu    sync.RWMutex
	value *Value
}

// New creates a new Variable initialised from the given Inventory.
// All hosts found in the inventory (including those resolved from groups)
// are pre-populated with empty HostVars entries.
func New(inv ansible.Inventory) Variable {
	hosts := make(map[string]*HostVars, len(inv.Hosts))

	// Add all explicitly declared hosts.
	for hostname := range inv.Hosts {
		hosts[hostname] = &HostVars{
			DefaultVars: make(map[string]any),
			RemoteVars:  make(map[string]any),
			RuntimeVars: make(map[string]any),
		}
	}

	// Add hosts referenced in groups that may not be in inv.Hosts yet.
	for _, group := range inv.Groups {
		for _, hostname := range group.Hosts {
			if _, ok := hosts[hostname]; !ok {
				hosts[hostname] = &HostVars{
					DefaultVars: make(map[string]any),
					RemoteVars:  make(map[string]any),
					RuntimeVars: make(map[string]any),
				}
			}
		}
	}

	return &variable{
		value: &Value{
			Inventory: inv,
			Hosts:     hosts,
			Result:    make(map[string]any),
		},
	}
}

// Get executes fn under a read lock and returns the result.
func (v *variable) Get(fn GetFunc) any {
	v.mu.RLock()
	defer v.mu.RUnlock()

	return fn(v.value)
}

// Merge executes fn under a write lock, allowing mutation of Value.
func (v *variable) Merge(fn MergeFunc) {
	v.mu.Lock()
	defer v.mu.Unlock()

	fn(v.value)
}
