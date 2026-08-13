package wait_for

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"vraxel.io/vraxel/lib/ansible/connector"
	"vraxel.io/vraxel/lib/ansible/modules/internal"
)

func localOpts(t *testing.T, args map[string]any) internal.ExecOptions {
	t.Helper()
	conn := connector.NewLocalConnector("")
	if err := conn.Init(context.Background()); err != nil {
		t.Fatalf("init local connector: %v", err)
	}
	return internal.ExecOptions{Args: args, Connector: conn}
}

func TestWaitFor_ExistingPathReturnsAtOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ready")
	if err := os.WriteFile(path, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	if _, _, err := ModuleWaitFor(context.Background(), localOpts(t, map[string]any{
		"path": path, "timeout": 5,
	})); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("an already-present path should return immediately, took %s", elapsed)
	}
}

func TestWaitFor_PathAppearsWhileWaiting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "later")
	go func() {
		time.Sleep(300 * time.Millisecond)
		_ = os.WriteFile(path, []byte("x"), 0644)
	}()

	if _, _, err := ModuleWaitFor(context.Background(), localOpts(t, map[string]any{
		"path": path, "timeout": 10, "sleep": 1,
	})); err != nil {
		t.Fatalf("expected the path to be picked up, got %v", err)
	}
}

func TestWaitFor_AbsentStateWaitsForRemoval(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vanishing")
	if err := os.WriteFile(path, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(300 * time.Millisecond)
		_ = os.Remove(path)
	}()

	if _, _, err := ModuleWaitFor(context.Background(), localOpts(t, map[string]any{
		"path": path, "state": "absent", "timeout": 10, "sleep": 1,
	})); err != nil {
		t.Fatalf("expected the removal to be picked up, got %v", err)
	}
}

func TestWaitFor_TimesOut(t *testing.T) {
	_, _, err := ModuleWaitFor(context.Background(), localOpts(t, map[string]any{
		"path": filepath.Join(t.TempDir(), "never"), "timeout": 1, "sleep": 1,
	}))
	if err == nil {
		t.Fatal("expected a timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("expected a timeout message, got %q", err.Error())
	}
}

func TestWaitFor_ListeningPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close() //nolint:errcheck
	port := ln.Addr().(*net.TCPAddr).Port

	if _, _, err := ModuleWaitFor(context.Background(), localOpts(t, map[string]any{
		"host": "127.0.0.1", "port": port, "timeout": 5,
	})); err != nil {
		t.Fatalf("expected the listening port to be reachable, got %v", err)
	}
}

func TestWaitFor_CancelledContextStopsTheWait(t *testing.T) {
	// The loop runs here rather than on the host precisely so this works.
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	_, _, err := ModuleWaitFor(ctx, localOpts(t, map[string]any{
		"path": filepath.Join(t.TempDir(), "never"), "timeout": 60, "sleep": 1,
	}))
	if err == nil {
		t.Fatal("expected cancellation to end the wait")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("cancellation should be prompt, took %s", elapsed)
	}
}

func TestWaitFor_ArgumentErrors(t *testing.T) {
	if _, _, err := ModuleWaitFor(context.Background(), localOpts(t, map[string]any{})); err == nil {
		t.Error("expected an error when neither port nor path is given")
	}
	if _, _, err := ModuleWaitFor(context.Background(), localOpts(t, map[string]any{
		"path": "/tmp", "state": "bogus",
	})); err == nil {
		t.Error("expected an error for an unsupported state")
	}
}
