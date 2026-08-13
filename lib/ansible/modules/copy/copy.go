package copy

import (
	"context"
	"fmt"
	"io/fs"
	"time"

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

	if err := moduleCopyWriteDest(ctx, opts, data, dest, mode); err != nil {
		return "", "", err
	}

	// Set ownership if specified.
	if chownArg := buildChownArg(owner, group); chownArg != "" {
		cmd := fmt.Sprintf("chown %s %s", chownArg, dest)
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

// moduleCopyWriteDest writes data to dest, using a temp-file + sudo mv
// dance when Become is set to avoid SFTP permission issues.
func moduleCopyWriteDest(ctx context.Context, opts internal.ExecOptions, data []byte, dest string, mode fs.FileMode) error {
	if opts.Become {
		// Write to temp file first, then sudo mv to avoid SFTP permission issues
		tmpPath := fmt.Sprintf("/tmp/.ansible_cp_%d", time.Now().UnixNano())
		if err := opts.Connector.PutFile(ctx, data, tmpPath, mode); err != nil {
			return fmt.Errorf("copy: write temp %s: %w", tmpPath, err)
		}
		mvCmd := fmt.Sprintf("mv %s %s && chmod %04o %s", tmpPath, dest, mode, dest)
		if _, _, err := opts.Connector.ExecuteCommand(ctx, mvCmd); err != nil {
			return fmt.Errorf("copy: move to %s: %w", dest, err)
		}
		return nil
	}
	if err := opts.Connector.PutFile(ctx, data, dest, mode); err != nil {
		return fmt.Errorf("copy: put file %s: %w", dest, err)
	}
	return nil
}

// buildChownArg builds the "owner:group" argument for chown.
func buildChownArg(owner, group string) string {
	if owner != "" && group != "" {
		return owner + ":" + group
	}
	if owner != "" {
		return owner
	}
	if group != "" {
		return ":" + group
	}
	return ""
}
