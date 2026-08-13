package file

import (
	"context"
	"fmt"

	"vraxel.io/vraxel/lib/ansible/modules/internal"
)

// ModuleFile creates, removes and adjusts paths on the remote host.
//
// Args:
//
//	path:  target path (required)
//	state: directory | file | touch | link | absent (default "file")
//	src:   link target, required when state is "link"
//	mode:  file mode, e.g. "0644"
//	owner: file owner
//	group: file group
//
// state "file" asserts the path exists and only applies mode/owner/group;
// it does not create the file, which is what "touch" is for.
func ModuleFile(ctx context.Context, opts internal.ExecOptions) (string, string, error) {
	path := internal.StringArg(opts.Args, "path")
	if path == "" {
		return "", "", fmt.Errorf("file: path is required")
	}

	state := internal.StringArg(opts.Args, "state")
	if state == "" {
		state = "file"
	}

	cmd, err := stateCommand(state, path, internal.StringArg(opts.Args, "src"))
	if err != nil {
		return "", "", err
	}
	if _, stderr, err := opts.Connector.ExecuteCommand(ctx, cmd); err != nil {
		return "", string(stderr), fmt.Errorf("file: %s %s: %w", state, path, err)
	}

	if state == "absent" {
		return fmt.Sprintf("removed %s", path), "", nil
	}
	if err := applyOwnership(ctx, opts, path); err != nil {
		return "", "", err
	}
	return fmt.Sprintf("%s %s", state, path), "", nil
}

// stateCommand builds the shell command that brings path into state.
func stateCommand(state, path, src string) (string, error) {
	qp := internal.ShellQuote(path)

	switch state {
	case "directory":
		return "mkdir -p " + qp, nil
	case "touch":
		return fmt.Sprintf("mkdir -p \"$(dirname %s)\" && touch %s", qp, qp), nil
	case "absent":
		return "rm -rf " + qp, nil
	case "link":
		if src == "" {
			return "", fmt.Errorf("file: src is required for state=link")
		}
		return fmt.Sprintf("ln -sfn %s %s", internal.ShellQuote(src), qp), nil
	case "file":
		// Assert rather than create: a missing path is a playbook error.
		return fmt.Sprintf("test -e %s", qp), nil
	default:
		return "", fmt.Errorf("file: unsupported state %q", state)
	}
}

// applyOwnership applies mode/owner/group when the task asked for them.
func applyOwnership(ctx context.Context, opts internal.ExecOptions, path string) error {
	qp := internal.ShellQuote(path)

	if _, ok := opts.Args["mode"]; ok {
		mode := internal.FileModeArg(opts.Args, "mode", 0644)
		cmd := fmt.Sprintf("chmod %04o %s", mode, qp)
		if _, _, err := opts.Connector.ExecuteCommand(ctx, cmd); err != nil {
			return fmt.Errorf("file: chmod %s: %w", path, err)
		}
	}

	chownArg := internal.ChownArg(
		internal.StringArg(opts.Args, "owner"),
		internal.StringArg(opts.Args, "group"),
	)
	if chownArg == "" {
		return nil
	}
	cmd := fmt.Sprintf("chown %s %s", chownArg, qp)
	if _, _, err := opts.Connector.ExecuteCommand(ctx, cmd); err != nil {
		return fmt.Errorf("file: chown %s: %w", path, err)
	}
	return nil
}
