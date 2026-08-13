package wait_for

import (
	"context"
	"fmt"
	"time"

	"vraxel.io/vraxel/lib/ansible/modules/internal"
)

// ModuleWaitFor blocks until a TCP port or a path reaches the wanted state.
//
// Args:
//
//	host:    host to probe for a port check (default 127.0.0.1)
//	port:    TCP port to probe
//	path:    path to probe; use instead of port
//	state:   present/started (default) or absent/stopped
//	timeout: seconds to keep trying (default 300)
//	delay:   seconds to wait before the first probe
//	sleep:   seconds between probes (default 1)
//
// The polling loop runs here rather than on the host so that cancelling the
// playbook's context stops the wait immediately.
func ModuleWaitFor(ctx context.Context, opts internal.ExecOptions) (string, string, error) {
	probe, target, err := buildProbe(opts)
	if err != nil {
		return "", "", err
	}

	wantUp, err := wantedUp(internal.StringArg(opts.Args, "state"))
	if err != nil {
		return "", "", err
	}

	timeout := time.Duration(intArg(opts.Args, "timeout", 300)) * time.Second
	sleep := time.Duration(intArg(opts.Args, "sleep", 1)) * time.Second
	if delay := intArg(opts.Args, "delay", 0); delay > 0 {
		if err := wait(ctx, time.Duration(delay)*time.Second); err != nil {
			return "", "", err
		}
	}

	deadline := time.Now().Add(timeout)
	for {
		_, _, probeErr := opts.Connector.ExecuteCommand(ctx, probe)
		if (probeErr == nil) == wantUp {
			return fmt.Sprintf("%s is %s", target, stateWord(wantUp)), "", nil
		}
		if time.Now().After(deadline) {
			return "", "", fmt.Errorf("wait_for: timed out after %s waiting for %s to be %s",
				timeout, target, stateWord(wantUp))
		}
		if err := wait(ctx, sleep); err != nil {
			return "", "", err
		}
	}
}

// buildProbe returns the shell command that succeeds while the target is up,
// plus a description of the target for messages.
func buildProbe(opts internal.ExecOptions) (probe, target string, err error) {
	path := internal.StringArg(opts.Args, "path")
	port := intArg(opts.Args, "port", 0)

	switch {
	case path != "":
		qp := internal.ShellQuote(path)
		return "test -e " + qp, path, nil
	case port > 0:
		host := internal.StringArg(opts.Args, "host")
		if host == "" {
			host = "127.0.0.1"
		}
		qh := internal.ShellQuote(host)
		// nc is the portable probe; bash's /dev/tcp covers hosts without it.
		cmd := fmt.Sprintf(`nc -z -w 1 %s %d 2>/dev/null || bash -c 'exec 3<>/dev/tcp/'%s'/%d' 2>/dev/null`,
			qh, port, qh, port)
		return cmd, fmt.Sprintf("%s:%d", host, port), nil
	default:
		return "", "", fmt.Errorf("wait_for: one of port or path is required")
	}
}

// wantedUp maps the state argument onto "target must be reachable".
func wantedUp(state string) (bool, error) {
	switch state {
	case "", "present", "started":
		return true, nil
	case "absent", "stopped":
		return false, nil
	default:
		return false, fmt.Errorf("wait_for: unsupported state %q", state)
	}
}

func stateWord(up bool) string {
	if up {
		return "present"
	}
	return "absent"
}

// wait sleeps for d unless the context ends first.
func wait(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

// intArg reads an integer argument, tolerating the float and string forms
// YAML and template rendering produce.
func intArg(args map[string]any, key string, def int) int {
	v, ok := args[key]
	if !ok {
		return def
	}
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	case string:
		var n int
		if _, err := fmt.Sscanf(x, "%d", &n); err == nil {
			return n
		}
	}
	return def
}
