package main

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"vraxel.io/vraxel/lib/agent/hostinfo"
)

func testLogger() stdLogger { return stdLogger{log.New(os.Stderr, "", 0)} }

// writeTestState puts a state file on disk the way a registration would.
func writeTestState(t *testing.T, st *state) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "agent.json")
	if err := saveState(path, st, testLogger()); err != nil {
		t.Fatalf("saveState: %v", err)
	}
	return path
}

func readTestState(t *testing.T, path string) *state {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	var st state
	if err := json.Unmarshal(data, &st); err != nil {
		t.Fatalf("parse state: %v", err)
	}
	return &st
}

// TestCheckStateMachineRejectsAForeignCredential is the clone guard: a
// state file that arrived with a copied disk carries another machine's
// credential, and starting with it puts two agents on one host row.
func TestCheckStateMachineRejectsAForeignCredential(t *testing.T) {
	st := &state{ServerURL: "https://s", AgentID: "a", HostID: 7, AgentToken: "tok",
		MachineID: "the-machine-this-was-cloned-from"}
	path := writeTestState(t, st)

	err := checkStateMachine(path, st, testLogger())
	if err == nil {
		t.Fatal("expected a foreign machine id to be refused")
	}
	// The obvious fix (delete the state file) is wrong on its own for a
	// raw clone, so the message has to name the machine-id reset too.
	if !strings.Contains(err.Error(), "machine-id") {
		t.Errorf("error should tell the operator to reset the machine identity, got: %v", err)
	}
}

// TestCheckStateMachineAcceptsItsOwnMachine covers the ordinary restart:
// the guard must be invisible to every host that was not cloned.
func TestCheckStateMachineAcceptsItsOwnMachine(t *testing.T) {
	st := &state{ServerURL: "https://s", AgentID: "a", HostID: 7, AgentToken: "tok",
		MachineID: hostinfo.MachineID()}
	path := writeTestState(t, st)

	if err := checkStateMachine(path, st, testLogger()); err != nil {
		t.Fatalf("own machine id must be accepted: %v", err)
	}
}

// TestCheckStateMachineAdoptsALegacyState covers agents that registered
// before the field existed. Refusing them would take every already-managed
// host offline on upgrade, so the first start records the identity instead.
func TestCheckStateMachineAdoptsALegacyState(t *testing.T) {
	st := &state{ServerURL: "https://s", AgentID: "a", HostID: 7, AgentToken: "tok"}
	path := writeTestState(t, st)

	if err := checkStateMachine(path, st, testLogger()); err != nil {
		t.Fatalf("a state file without a machine id must be adopted, not refused: %v", err)
	}
	// Persisted, or the guard would keep adopting and never actually guard.
	if got := readTestState(t, path).MachineID; got != hostinfo.MachineID() {
		t.Errorf("machineId not persisted: got %q, want %q", got, hostinfo.MachineID())
	}
}
