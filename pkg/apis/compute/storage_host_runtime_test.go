package compute

import (
	"testing"

	"vraxel.io/vraxel/lib/apiserver"
)

// The two runtime reads are mounted differently on purpose, and the
// difference IS the permission boundary: accounts declares a resource, so
// the framework derives compute:hosts:accounts:list for it, while
// processes is a verb and inherits compute:hosts:get. Collapse either one
// into the other and "may read this host" silently starts carrying "may
// see who has root on it".
func TestAccountsIsItsOwnResource(t *testing.T) {
	def := HostAccountsDef(nil, nil)
	if def.Parent != "hosts" {
		t.Errorf("Parent = %q, want hosts", def.Parent)
	}
	if def.SingletonGet == nil {
		t.Error("SingletonGet is nil; the route would not exist")
	}
	// Only a read is declared, so only a read code is derived. A write op
	// here would mint compute:hosts:accounts:create and a route to go with
	// it -- and nothing in this feature may write an account.
	if def.Ops.List != nil || def.Ops.Get != nil || def.Ops.Create != nil ||
		def.Ops.Update != nil || def.Ops.Patch != nil || def.Ops.Delete != nil {
		t.Errorf("accounts declares an op beyond SingletonGet: %+v", def.Ops)
	}
	if len(def.Actions) != 0 {
		t.Errorf("accounts declares actions: %+v", def.Actions)
	}
	if def.Scopes != apiserver.ScopeAll {
		t.Errorf("Scopes = %v, want ScopeAll so the nested paths exist at every tenancy level", def.Scopes)
	}
}

// Processes rides the hosts resource as a verb, which is what makes it
// inherit compute:hosts:get instead of minting a code.
func TestProcessesIsAVerbOnHosts(t *testing.T) {
	def := HostsDef(nil, nil, nil, nil, NewTerminalSessions(), NewAgentDialerHolder(),
		NewAgentLiveMetrics(NewAgentDialerHolder()), nil)
	for _, v := range def.Verbs {
		if v.Name == "processes" {
			return
		}
	}
	t.Fatalf("hosts declares no processes verb; verbs = %+v", def.Verbs)
}
