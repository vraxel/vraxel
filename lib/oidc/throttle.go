package oidc

import (
	"context"
	"strings"
	"time"

	"vraxel.io/vraxel/lib/logger"
)

// Login brute-force limits. bcrypt already prices a single guess at
// ~100ms of CPU; these fixed windows bound the sustained rate. The
// per-username lock protects one account from a distributed attack;
// the per-IP lock protects the username space from a single source.
// Counters live in PostgreSQL (login_throttle) so every instance of a
// horizontally-scaled deployment sees the same state.
const (
	loginFailWindow    = 15 * time.Minute
	loginUserFailLimit = 5  // per lower-cased username
	loginIPFailLimit   = 20 // per client IP
)

// LoginThrottleStore persists fixed-window failure counters. The
// production implementation is PostgreSQL-backed (NewPGLoginThrottleStore).
type LoginThrottleStore interface {
	// Get returns the current counter for key. found=false when the key
	// has no row.
	Get(ctx context.Context, key string) (failCount int, windowStart time.Time, found bool, err error)
	// Bump increments the counter, restarting the window if the previous
	// one (windowSeconds) has fully elapsed.
	Bump(ctx context.Context, key string, windowSeconds int) error
	// Reset deletes the counter for key.
	Reset(ctx context.Context, key string) error
	// Sweep deletes counters older than windowSeconds.
	Sweep(ctx context.Context, windowSeconds int) error
}

// SetLoginThrottle wires the failure-counter store. When unset the
// provider performs no login throttling (unit-test construction).
func (p *Provider) SetLoginThrottle(s LoginThrottleStore) { p.loginThrottle = s }

// Registration abuse limits. Unlike login throttling (which counts only
// failures), EVERY register attempt counts: successful spam registrations
// are precisely the abuse being bounded, and each attempt burns a bcrypt
// hash regardless of outcome. Counters share the login_throttle table
// under a distinct key prefix; both windows are equal so the shared
// sweep keeps register rows GC'd too.
const (
	registerWindow  = loginFailWindow
	registerIPLimit = 10 // per client IP
)

func loginUserKey(username string) string { return "u:" + strings.ToLower(strings.TrimSpace(username)) }
func loginIPKey(ip string) string         { return "ip:" + ip }
func registerIPKey(ip string) string      { return "reg:" + ip }

// LoginLocked reports whether a login attempt for (username, ip) is
// currently locked out, and for how long. Fails open: a store error is
// logged and treated as not-locked, so a degraded PG connection can
// break brute-force protection but never logins themselves.
func (p *Provider) LoginLocked(ctx context.Context, username, ip string) (time.Duration, bool) {
	if p.loginThrottle == nil {
		return 0, false
	}
	for key, limit := range map[string]int{
		loginUserKey(username): loginUserFailLimit,
		loginIPKey(ip):         loginIPFailLimit,
	} {
		count, windowStart, found, err := p.loginThrottle.Get(ctx, key)
		if err != nil {
			logger.Errorf("login throttle: get %q: %v", key, err)
			continue
		}
		if !found {
			continue
		}
		if retry, locked := throttleDecision(count, windowStart, limit, loginFailWindow, time.Now()); locked {
			return retry, true
		}
	}
	return 0, false
}

// throttleDecision is the pure lockout rule: locked while the fixed
// window that accumulated >= limit failures has not yet elapsed.
func throttleDecision(count int, windowStart time.Time, limit int, window time.Duration, now time.Time) (time.Duration, bool) {
	if count < limit {
		return 0, false
	}
	remaining := windowStart.Add(window).Sub(now)
	if remaining <= 0 {
		return 0, false
	}
	return remaining, true
}

// RegisterLocked reports whether registration from ip is currently locked
// out, and for how long. Same fail-open contract as LoginLocked.
func (p *Provider) RegisterLocked(ctx context.Context, ip string) (time.Duration, bool) {
	if p.loginThrottle == nil {
		return 0, false
	}
	key := registerIPKey(ip)
	count, windowStart, found, err := p.loginThrottle.Get(ctx, key)
	if err != nil {
		logger.Errorf("register throttle: get %q: %v", key, err)
		return 0, false
	}
	if !found {
		return 0, false
	}
	return throttleDecision(count, windowStart, registerIPLimit, registerWindow, time.Now())
}

// NoteRegisterAttempt records one registration attempt (success or not)
// against the per-IP counter. Best-effort: errors are logged only.
func (p *Provider) NoteRegisterAttempt(ctx context.Context, ip string) {
	if p.loginThrottle == nil {
		return
	}
	if err := p.loginThrottle.Bump(ctx, registerIPKey(ip), int(registerWindow.Seconds())); err != nil {
		logger.Errorf("register throttle: bump: %v", err)
	}
}

// NoteLoginFailure records a failed attempt against both counters.
// Best-effort: errors are logged, never surfaced to the caller.
func (p *Provider) NoteLoginFailure(ctx context.Context, username, ip string) {
	if p.loginThrottle == nil {
		return
	}
	secs := int(loginFailWindow.Seconds())
	for _, key := range []string{loginUserKey(username), loginIPKey(ip)} {
		if err := p.loginThrottle.Bump(ctx, key, secs); err != nil {
			logger.Errorf("login throttle: bump %q: %v", key, err)
		}
	}
}

// NoteLoginSuccess clears the username counter (the IP counter keeps
// accumulating: a success must not reset a spraying source) and
// opportunistically GCs expired rows.
func (p *Provider) NoteLoginSuccess(ctx context.Context, username string) {
	if p.loginThrottle == nil {
		return
	}
	if err := p.loginThrottle.Reset(ctx, loginUserKey(username)); err != nil {
		logger.Errorf("login throttle: reset: %v", err)
	}
	if err := p.loginThrottle.Sweep(ctx, int(loginFailWindow.Seconds())); err != nil {
		logger.Errorf("login throttle: sweep: %v", err)
	}
}
