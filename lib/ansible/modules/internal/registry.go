package internal

import (
	"sort"
	"sync"
)

var (
	registryMu sync.RWMutex
	registry   = make(map[string]ModuleExecFunc)
)

// RegisterModule registers a module by name.
func RegisterModule(name string, fn ModuleExecFunc) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[name] = fn
}

// FindModule returns the module function by name, or nil if not found.
func FindModule(name string) ModuleExecFunc {
	registryMu.RLock()
	defer registryMu.RUnlock()
	return registry[name]
}

// IsModule checks if a name is a registered module.
func IsModule(name string) bool {
	return FindModule(name) != nil
}

// ListModules returns all registered module names sorted.
func ListModules() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// SaveAndResetRegistry clears the global registry for test isolation
// and returns a cleanup function that restores it.
func SaveAndResetRegistry() func() {
	registryMu.Lock()
	saved := registry
	registry = make(map[string]ModuleExecFunc)
	registryMu.Unlock()
	return func() {
		registryMu.Lock()
		registry = saved
		registryMu.Unlock()
	}
}
