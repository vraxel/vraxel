package upgrade

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
	"vraxel.io/vraxel/lib/buildinfo"
)

type testLogger struct{ t *testing.T }

func (l testLogger) Infof(f string, a ...any) { l.t.Logf("INFO  "+f, a...) }
func (l testLogger) Warnf(f string, a ...any) { l.t.Logf("WARN  "+f, a...) }

// fakeAgent is a stand-in agent binary: a script that answers -version
// the way the real one does, printing the full build string rather than
// the short version the server announces.
func fakeAgent(version string) []byte {
	return []byte("#!/bin/sh\necho \"agent-" + version + "-20260812-061329-heads-main-0-g7fd60a64a\"\n")
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// harness wires a Manager against a temp dir and a server that hands out
// the given binary.
type harness struct {
	m      *Manager
	dir    string
	binary string
	exits  chan struct{}
}

func newHarness(t *testing.T, current string, served []byte) *harness {
	t.Helper()
	dir := t.TempDir()
	binary := filepath.Join(dir, "agent")
	if err := os.WriteFile(binary, fakeAgent(current), 0o755); err != nil {
		t.Fatalf("seed binary: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != agenttypes.BinaryPath(runtime.GOOS, runtime.GOARCH) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write(served)
	}))
	t.Cleanup(srv.Close)

	h := &harness{dir: dir, binary: binary, exits: make(chan struct{}, 1)}
	h.m = &Manager{
		BinaryPath: binary,
		StateDir:   dir,
		ServerURL:  srv.URL,
		Version:    current,
		Log:        testLogger{t},
		VersionOf:  buildinfo.ShortVersionOf,
		Exit:       func() { h.exits <- struct{}{} },
	}
	return h
}

func (h *harness) currentVersionScript(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(h.binary)
	if err != nil {
		t.Fatalf("read binary: %v", err)
	}
	return string(data)
}

func TestApplyInstallsVerifiesAndRestarts(t *testing.T) {
	next := fakeAgent("v2.0.0")
	h := newHarness(t, "v1.0.0", next)

	if err := h.m.Apply(context.Background(), agenttypes.AgentUpgrade{
		Version: "v2.0.0", SHA256: sha256Hex(next),
	}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if got := h.currentVersionScript(t); got != string(next) {
		t.Fatalf("binary was not replaced: %q", got)
	}
	if _, err := os.Stat(h.binary + ".old"); err != nil {
		t.Fatalf("no backup of the previous binary: %v", err)
	}
	if _, err := os.Stat(filepath.Join(h.dir, "upgrade.json")); err != nil {
		t.Fatalf("no sentinel written: %v", err)
	}
	select {
	case <-h.exits:
	case <-time.After(5 * time.Second):
		t.Fatal("idle agent never restarted")
	}
}

// TestRestartWaitsForBusy proves a running job is not killed to install
// an agent.
func TestRestartWaitsForBusy(t *testing.T) {
	next := fakeAgent("v2.0.0")
	h := newHarness(t, "v1.0.0", next)

	h.m.RestartPoll = 20 * time.Millisecond
	var mu sync.Mutex
	busy := true
	h.m.Busy = func() bool {
		mu.Lock()
		defer mu.Unlock()
		return busy
	}

	if err := h.m.Apply(context.Background(), agenttypes.AgentUpgrade{
		Version: "v2.0.0", SHA256: sha256Hex(next),
	}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !h.m.Pending() {
		t.Fatal("Pending() is false while a staged upgrade waits on a running job")
	}
	select {
	case <-h.exits:
		t.Fatal("restarted while a job was still running")
	case <-time.After(200 * time.Millisecond):
	}

	mu.Lock()
	busy = false
	mu.Unlock()
	select {
	case <-h.exits:
	case <-time.After(30 * time.Second):
		t.Fatal("never restarted after the job finished")
	}
}

func TestApplyRejectsDigestMismatch(t *testing.T) {
	next := fakeAgent("v2.0.0")
	h := newHarness(t, "v1.0.0", next)
	original := h.currentVersionScript(t)

	err := h.m.Apply(context.Background(), agenttypes.AgentUpgrade{
		Version: "v2.0.0", SHA256: sha256Hex([]byte("something else")),
	})
	if err == nil {
		t.Fatal("Apply accepted a binary whose digest did not match")
	}
	if got := h.currentVersionScript(t); got != original {
		t.Fatal("the running binary was replaced despite the digest mismatch")
	}
	if _, err := os.Stat(h.binary + ".new"); !os.IsNotExist(err) {
		t.Fatal("the rejected download was left on disk")
	}
}

// TestApplyRejectsBinaryThatMisreportsItsVersion covers the corrupt or
// wrong-architecture build: caught before the swap, not after.
func TestApplyRejectsBinaryThatMisreportsItsVersion(t *testing.T) {
	next := fakeAgent("v9.9.9")
	h := newHarness(t, "v1.0.0", next)
	original := h.currentVersionScript(t)

	err := h.m.Apply(context.Background(), agenttypes.AgentUpgrade{
		Version: "v2.0.0", SHA256: sha256Hex(next),
	})
	if err == nil {
		t.Fatal("Apply accepted a binary reporting the wrong version")
	}
	if got := h.currentVersionScript(t); got != original {
		t.Fatal("the running binary was replaced despite the version mismatch")
	}
}

func TestRollbackAfterTwoUnconfirmedBoots(t *testing.T) {
	next := fakeAgent("v2.0.0")
	h := newHarness(t, "v1.0.0", next)
	old := h.currentVersionScript(t)

	if err := h.m.Apply(context.Background(), agenttypes.AgentUpgrade{
		Version: "v2.0.0", SHA256: sha256Hex(next),
	}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	<-h.exits

	// First boot on the new binary: still given the benefit of the doubt.
	rolled, err := h.m.RollbackIfUnhealthy()
	if err != nil {
		t.Fatalf("RollbackIfUnhealthy: %v", err)
	}
	if rolled {
		t.Fatal("rolled back on the first boot")
	}
	if got := h.currentVersionScript(t); got != string(next) {
		t.Fatal("binary changed on the first boot")
	}

	// Second boot without ever confirming: restore the previous binary.
	rolled, err = h.m.RollbackIfUnhealthy()
	if err != nil {
		t.Fatalf("RollbackIfUnhealthy: %v", err)
	}
	if !rolled {
		t.Fatal("did not roll back after the boot budget was spent")
	}
	if got := h.currentVersionScript(t); got != old {
		t.Fatalf("binary is %q, want the previous %q", got, old)
	}
	if _, err := os.Stat(filepath.Join(h.dir, "upgrade.json")); !os.IsNotExist(err) {
		t.Fatal("sentinel survived the rollback")
	}
}

func TestConfirmHealthyEndsTheUpgrade(t *testing.T) {
	next := fakeAgent("v2.0.0")
	h := newHarness(t, "v1.0.0", next)

	if err := h.m.Apply(context.Background(), agenttypes.AgentUpgrade{
		Version: "v2.0.0", SHA256: sha256Hex(next),
	}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	<-h.exits

	h.m.ConfirmHealthy()
	if _, err := os.Stat(filepath.Join(h.dir, "upgrade.json")); !os.IsNotExist(err) {
		t.Fatal("sentinel survived confirmation")
	}
	if _, err := os.Stat(h.binary + ".old"); !os.IsNotExist(err) {
		t.Fatal("backup survived confirmation")
	}

	// A later restart must not look like an unconfirmed upgrade.
	rolled, err := h.m.RollbackIfUnhealthy()
	if err != nil || rolled {
		t.Fatalf("rolled back after a confirmed upgrade (rolled=%v err=%v)", rolled, err)
	}
}

func TestApplyIsNoOpForTheRunningVersion(t *testing.T) {
	h := newHarness(t, "v1.0.0", fakeAgent("v1.0.0"))
	if err := h.m.Apply(context.Background(), agenttypes.AgentUpgrade{
		Version: "v1.0.0", SHA256: "whatever",
	}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if _, err := os.Stat(filepath.Join(h.dir, "upgrade.json")); !os.IsNotExist(err) {
		t.Fatal("re-announcing the running version staged an upgrade")
	}
}
