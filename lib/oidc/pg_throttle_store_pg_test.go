package oidc_test

import (
	"context"
	"testing"

	"vraxel.io/vraxel/lib/oidc"
	"vraxel.io/vraxel/pkg/db/dbtest"
)

// Exercises the fixed-window UPSERT against a real database: increment
// within the window, restart after it elapses, reset, sweep.
func TestPGLoginThrottleWindow(t *testing.T) {
	database := dbtest.New(t)
	s := oidc.NewPGLoginThrottleStore(database.GetQueries())
	ctx := context.Background()
	const window = 900

	if _, _, found, err := s.Get(ctx, "u:none"); err != nil || found {
		t.Fatalf("empty get: found=%v err=%v", found, err)
	}

	for range 3 {
		if err := s.Bump(ctx, "u:alice", window); err != nil {
			t.Fatalf("bump: %v", err)
		}
	}
	count, _, found, err := s.Get(ctx, "u:alice")
	if err != nil || !found || count != 3 {
		t.Fatalf("after 3 bumps: count=%d found=%v err=%v", count, found, err)
	}

	// Age the window past expiry; the next bump must restart at 1.
	if _, err := database.GetPool().Exec(ctx,
		"UPDATE login_throttle SET window_start = now() - interval '1 hour' WHERE key = 'u:alice'"); err != nil {
		t.Fatalf("age window: %v", err)
	}
	if err := s.Bump(ctx, "u:alice", window); err != nil {
		t.Fatalf("bump after expiry: %v", err)
	}
	if count, _, _, _ = s.Get(ctx, "u:alice"); count != 1 {
		t.Fatalf("expired window must restart at 1, got %d", count)
	}

	if err := s.Reset(ctx, "u:alice"); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if _, _, found, _ = s.Get(ctx, "u:alice"); found {
		t.Fatal("reset must delete the row")
	}

	// Sweep removes only expired rows.
	_ = s.Bump(ctx, "u:old", window)
	_ = s.Bump(ctx, "u:fresh", window)
	if _, err := database.GetPool().Exec(ctx,
		"UPDATE login_throttle SET window_start = now() - interval '1 hour' WHERE key = 'u:old'"); err != nil {
		t.Fatalf("age row: %v", err)
	}
	if err := s.Sweep(ctx, window); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if _, _, found, _ = s.Get(ctx, "u:old"); found {
		t.Fatal("sweep must drop the expired row")
	}
	if _, _, found, _ = s.Get(ctx, "u:fresh"); !found {
		t.Fatal("sweep must keep the fresh row")
	}
}
