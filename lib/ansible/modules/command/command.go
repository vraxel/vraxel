package command

import (
	"context"
	"fmt"

	"vraxel.io/vraxel/lib/ansible/modules/internal"
)

// ModuleCommand executes a command on the remote host via the connector.
// Args: map with "cmd", "command", or "shell" key containing the command string.
func ModuleCommand(ctx context.Context, opts internal.ExecOptions) (string, string, error) {
	cmd := ExtractCommand(opts.Args)
	if cmd == "" {
		return "", "", fmt.Errorf("command module: no command specified")
	}

	stdout, stderr, err := opts.Connector.ExecuteCommand(ctx, internal.EnvPrefix(opts.Environment)+cmd)
	return string(stdout), string(stderr), err
}

// ModuleShell executes a command via shell. The connector already wraps
// commands in a shell, so this is functionally identical to ModuleCommand.
func ModuleShell(ctx context.Context, opts internal.ExecOptions) (string, string, error) {
	return ModuleCommand(ctx, opts)
}

// ExtractCommand extracts the command string from module args.
// It checks "cmd", "command", and "shell" keys in order.
func ExtractCommand(args map[string]any) string {
	for _, key := range []string{"cmd", "command", "shell"} {
		if v, ok := args[key]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}
