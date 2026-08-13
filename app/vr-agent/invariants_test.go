package main

import (
	"os/exec"
	"strings"
	"testing"
)

// deps returns the full import closure of this main package.
func deps(t *testing.T) []string {
	t.Helper()
	out, err := exec.Command("go", "list", "-deps", ".").Output()
	if err != nil {
		t.Fatalf("go list -deps: %v", err)
	}
	return strings.Split(strings.TrimSpace(string(out)), "\n")
}

// TestAgentLinksNoSSHStack pins the property the connector split exists
// for: the agent runs one play against the machine it is installed on
// and never dials anything, so a full SSH client and SFTP library have
// no business in its binary. A single stray import would silently put
// the SSH path back into a root process on every managed host.
func TestAgentLinksNoSSHStack(t *testing.T) {
	banned := []string{
		"vraxel.io/vraxel/lib/clients/sshclient",
		"golang.org/x/crypto/ssh",
		"github.com/pkg/sftp",
	}
	for _, dep := range deps(t) {
		for _, b := range banned {
			if dep == b {
				t.Errorf("the agent links %s again; keep SSH behind lib/ansible/connector/ssh "+
					"and pass it via the server side only", dep)
			}
		}
	}
}

// TestAgentLinksNoServerAPIs pins the other half: the agent is the
// minimal binary shipped to every managed host and must not link any
// server-side API/DB code. Any dependency on pkg/apis pulls the store
// and REST layers into a process that only needs the wire protocol in
// lib/agent/types.
func TestAgentLinksNoServerAPIs(t *testing.T) {
	const banned = "vraxel.io/vraxel/pkg/apis"
	for _, dep := range deps(t) {
		if dep == banned || strings.HasPrefix(dep, banned+"/") {
			t.Errorf("the agent links %s; the agent must depend only on lib/agent/* "+
				"and the wire protocol in lib/agent/types, never on server-side APIs", dep)
		}
	}
}
