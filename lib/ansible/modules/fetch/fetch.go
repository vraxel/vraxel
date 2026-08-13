package fetch

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"vraxel.io/vraxel/lib/ansible/modules/internal"
)

// ModuleFetch downloads a file from a remote host to the local filesystem.
//
// Args:
//
//	src:  remote file path (required)
//	dest: local destination path (required)
func ModuleFetch(ctx context.Context, opts internal.ExecOptions) (string, string, error) {
	src := internal.StringArg(opts.Args, "src")
	if src == "" {
		return "", "", fmt.Errorf("fetch: src is required")
	}

	dest := internal.StringArg(opts.Args, "dest")
	if dest == "" {
		return "", "", fmt.Errorf("fetch: dest is required")
	}

	// Ensure the destination directory exists.
	destDir := filepath.Dir(dest)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return "", "", fmt.Errorf("fetch: create dest dir %s: %w", destDir, err)
	}

	var buf bytes.Buffer

	if opts.Become {
		// Copy to a temp file via sudo, then SFTP fetch the temp file
		tmpPath := fmt.Sprintf("/tmp/.ansible_fetch_%d", time.Now().UnixNano())
		cpCmd := fmt.Sprintf("cp %s %s && chmod 644 %s", src, tmpPath, tmpPath)
		if _, _, err := opts.Connector.ExecuteCommand(ctx, cpCmd); err != nil {
			return "", "", fmt.Errorf("fetch: sudo cp %s: %w", src, err)
		}
		defer func() {
			rmCmd := fmt.Sprintf("rm -f %s", tmpPath)
			opts.Connector.ExecuteCommand(ctx, rmCmd) //nolint:errcheck
		}()
		if err := opts.Connector.FetchFile(ctx, tmpPath, &buf); err != nil {
			return "", "", fmt.Errorf("fetch: fetch temp %s: %w", tmpPath, err)
		}
	} else {
		if err := opts.Connector.FetchFile(ctx, src, &buf); err != nil {
			return "", "", fmt.Errorf("fetch: fetch file %s: %w", src, err)
		}
	}

	// Write fetched content to the destination file.
	if err := os.WriteFile(dest, buf.Bytes(), 0644); err != nil {
		return "", "", fmt.Errorf("fetch: write file %s: %w", dest, err)
	}

	return fmt.Sprintf("fetched %s to %s", src, dest), "", nil
}
