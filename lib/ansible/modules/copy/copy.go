package copy

import (
	"context"
	"fmt"

	"vraxel.io/vraxel/lib/ansible/modules/internal"
)

// ModuleCopy copies files or content to a remote host.
//
// Args:
//
//	src:     source file path (relative to playbook source or absolute)
//	content: direct content string (alternative to src)
//	dest:    destination path on remote (required)
//	mode:    file mode string or integer (optional, default 0644)
//	owner:   file owner (optional)
//	group:   file group (optional)
func ModuleCopy(ctx context.Context, opts internal.ExecOptions) (string, string, error) {
	dest := internal.StringArg(opts.Args, "dest")
	if dest == "" {
		return "", "", fmt.Errorf("copy: dest is required")
	}

	mode := internal.FileModeArg(opts.Args, "mode", 0644)
	owner := internal.StringArg(opts.Args, "owner")
	group := internal.StringArg(opts.Args, "group")

	data, err := moduleCopyResolveData(opts)
	if err != nil {
		return "", "", err
	}

	if err := internal.WriteFile(ctx, opts, data, dest, mode); err != nil {
		return "", "", fmt.Errorf("copy: %w", err)
	}

	// Set ownership if specified.
	if chownArg := internal.ChownArg(owner, group); chownArg != "" {
		cmd := fmt.Sprintf("chown %s %s", chownArg, internal.ShellQuote(dest))
		if _, _, err := opts.Connector.ExecuteCommand(ctx, cmd); err != nil {
			return "", "", fmt.Errorf("copy: chown %s: %w", dest, err)
		}
	}

	return fmt.Sprintf("copied to %s", dest), "", nil
}

// moduleCopyResolveData resolves the bytes to write from the content
// arg (direct string) or the src arg (read from the playbook source).
func moduleCopyResolveData(opts internal.ExecOptions) ([]byte, error) {
	if content, ok := opts.Args["content"].(string); ok {
		return []byte(content), nil
	} else if src, ok := opts.Args["src"].(string); ok && src != "" {
		data, err := internal.ReadSource(opts.Source, src)
		if err != nil {
			return nil, fmt.Errorf("copy: read source %s: %w", src, err)
		}
		return data, nil
	}
	return nil, fmt.Errorf("copy: either src or content is required")
}
