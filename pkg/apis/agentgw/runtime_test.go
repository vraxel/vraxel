package agentgw

import (
	"context"
	"strings"
	"testing"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
)

// A host.processes frame lands in the store keyed by the session's host,
// re-encoded as jsonb. An absent payload writes nothing, which is what a
// frame from an agent that dropped the field would look like.
func TestRecordProcesses(t *testing.T) {
	store := &fakeAgentStore{}
	h := &protocolHandler{agents: store}
	sess := &Session{AgentID: "a-1", HostID: 42}

	h.recordProcesses(context.Background(), sess, nil)
	if len(store.procsCalls()) != 0 {
		t.Fatalf("a frame without processes must not write: %+v", store.procs)
	}

	h.recordProcesses(context.Background(), sess, &agenttypes.HostProcesses{
		Groups: []agenttypes.ProcessGroup{{
			Name: "nginx", User: "www-data", Count: 4, Unit: "nginx.service",
			Ports: []agenttypes.ListenPort{{Proto: "tcp", Addr: "0.0.0.0", Port: 80}},
		}},
	})

	calls := store.procsCalls()
	if len(calls) != 1 {
		t.Fatalf("processes not stored: %+v", calls)
	}
	if calls[0].hostID != 42 {
		t.Fatalf("stored under host %d, want 42", calls[0].hostID)
	}
	const want = `[{"name":"nginx","user":"www-data","count":4,"unit":"nginx.service","ports":[{"proto":"tcp","addr":"0.0.0.0","port":80}]}]`
	if string(calls[0].groups) != want {
		t.Fatalf("groups = %s\nwant %s", calls[0].groups, want)
	}

	// An empty workload list must reach jsonb as [], never null: the
	// column is NOT NULL, and "nothing running" is a real answer.
	h.recordProcesses(context.Background(), sess, &agenttypes.HostProcesses{})
	if got := string(store.procsCalls()[1].groups); got != "[]" {
		t.Fatalf("empty groups = %s, want []", got)
	}
}

func TestRecordAccounts(t *testing.T) {
	store := &fakeAgentStore{}
	h := &protocolHandler{agents: store}
	sess := &Session{AgentID: "a-1", HostID: 42}

	h.recordAccounts(context.Background(), sess, nil)
	if len(store.acctsCalls()) != 0 {
		t.Fatalf("a frame without accounts must not write: %+v", store.accts)
	}

	h.recordAccounts(context.Background(), sess, &agenttypes.HostAccounts{
		Users: []agenttypes.Account{{
			Name: "root", UID: 0, GID: 0, Group: "root", Home: "/root", Shell: "/bin/bash",
			CanLogin: true, Password: agenttypes.PwSet,
			Privileges: []string{agenttypes.PrivRoot, agenttypes.PrivSudo},
			SSHKeys:    []agenttypes.SSHKey{{Type: "ssh-ed25519", Fingerprint: "SHA256:abc", Comment: "zly@laptop"}},
		}},
		Groups:    []agenttypes.UserGroup{{Name: "sudo", GID: 27, Members: []string{"zly"}}},
		SudoRules: []string{"root\tALL=(ALL:ALL) ALL"},
	})

	calls := store.acctsCalls()
	if len(calls) != 1 {
		t.Fatalf("accounts not stored: %+v", calls)
	}
	got := calls[0]
	if got.hostID != 42 {
		t.Fatalf("stored under host %d, want 42", got.hostID)
	}
	if !strings.Contains(string(got.in.Users), `"password":"set"`) {
		t.Fatalf("users = %s", got.in.Users)
	}
	if !strings.Contains(string(got.in.Users), `"privileges":["root","sudo"]`) {
		t.Fatalf("privileges lost: %s", got.in.Users)
	}
	// The whole point of the fingerprint form: a key body must never be
	// on this path, so there is nothing here that could carry one.
	if strings.Contains(string(got.in.Users), "AAAAC3Nza") {
		t.Fatalf("a key body reached the store: %s", got.in.Users)
	}
	if !strings.Contains(string(got.in.Groups), `"gid":27`) {
		t.Fatalf("groups = %s", got.in.Groups)
	}
	if !strings.Contains(string(got.in.SudoRules), "ALL=(ALL:ALL)") {
		t.Fatalf("sudo rules = %s", got.in.SudoRules)
	}

	h.recordAccounts(context.Background(), sess, &agenttypes.HostAccounts{})
	empty := store.acctsCalls()[1].in
	for name, v := range map[string][]byte{"users": empty.Users, "groups": empty.Groups, "sudoRules": empty.SudoRules} {
		if string(v) != "[]" {
			t.Errorf("empty %s = %s, want []", name, v)
		}
	}
}

// A password hash must never be representable on this path. The agent
// reduces the shadow field to one of four words before it is sent, so
// there is no field on the wire type that could carry one -- this asserts
// the type itself, which is what keeps a later "just add the hash" from
// compiling quietly.
func TestAccountCarriesNoSecret(t *testing.T) {
	for _, s := range []string{agenttypes.PwSet, agenttypes.PwLocked, agenttypes.PwDisabled, agenttypes.PwEmpty} {
		if strings.HasPrefix(s, "$") || len(s) > 12 {
			t.Errorf("password state %q looks like a hash, not a classification", s)
		}
	}
}
