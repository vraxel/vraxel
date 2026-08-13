package service

import (
	"context"
	"fmt"
	"strings"

	"vraxel.io/vraxel/lib/ansible/modules/internal"
)

// ModuleService drives a systemd unit. It is registered under both "service"
// and "systemd": this engine targets systemd hosts, so the generic name has
// no second implementation to choose between.
//
// Args:
//
//	name:          unit name, with or without the .service suffix (required)
//	state:         started | stopped | restarted | reloaded
//	enabled:       true / false, to set the boot-time state
//	daemon_reload: true, to run systemctl daemon-reload first
//
// At least one of state, enabled or daemon_reload must be given.
func ModuleService(ctx context.Context, opts internal.ExecOptions) (string, string, error) {
	name := internal.StringArg(opts.Args, "name")
	if name == "" {
		return "", "", fmt.Errorf("service: name is required")
	}
	state := internal.StringArg(opts.Args, "state")
	enabled, hasEnabled := boolArg(opts.Args, "enabled")
	reload, _ := boolArg(opts.Args, "daemon_reload")

	if state == "" && !hasEnabled && !reload {
		return "", "", fmt.Errorf("service: one of state, enabled or daemon_reload is required")
	}

	var done []string
	qn := internal.ShellQuote(name)

	if reload {
		if err := run(ctx, opts, "systemctl daemon-reload"); err != nil {
			return "", "", err
		}
		done = append(done, "daemon-reload")
	}

	// Enable before starting, so a unit that is both enabled and started
	// comes up with the boot-time state already in place.
	if hasEnabled {
		verb := "disable"
		if enabled {
			verb = "enable"
		}
		if err := run(ctx, opts, "systemctl "+verb+" "+qn); err != nil {
			return "", "", err
		}
		done = append(done, verb)
	}

	if state != "" {
		verb, err := stateVerb(state)
		if err != nil {
			return "", "", err
		}
		if err := run(ctx, opts, "systemctl "+verb+" "+qn); err != nil {
			return "", "", err
		}
		done = append(done, verb)
	}

	return fmt.Sprintf("%s: %s", name, strings.Join(done, ", ")), "", nil
}

// stateVerb maps Ansible's state names onto systemctl subcommands.
func stateVerb(state string) (string, error) {
	switch state {
	case "started":
		return "start", nil
	case "stopped":
		return "stop", nil
	case "restarted":
		return "restart", nil
	case "reloaded":
		return "reload", nil
	default:
		return "", fmt.Errorf("service: unsupported state %q", state)
	}
}

func run(ctx context.Context, opts internal.ExecOptions, cmd string) error {
	if _, stderr, err := opts.Connector.ExecuteCommand(ctx, cmd); err != nil {
		return fmt.Errorf("service: %s: %w: %s", cmd, err, strings.TrimSpace(string(stderr)))
	}
	return nil
}

// boolArg reads a boolean argument, accepting YAML booleans and the strings
// playbooks write when the value came from a template.
func boolArg(args map[string]any, key string) (value, present bool) {
	v, ok := args[key]
	if !ok {
		return false, false
	}
	switch x := v.(type) {
	case bool:
		return x, true
	case string:
		switch strings.ToLower(strings.TrimSpace(x)) {
		case "true", "yes", "1":
			return true, true
		case "false", "no", "0":
			return false, true
		}
	}
	return false, false
}
