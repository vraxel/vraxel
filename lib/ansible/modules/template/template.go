package template

import (
	"context"
	"fmt"
	"time"

	"vraxel.io/vraxel/lib/ansible/modules/internal"
	tmpl "vraxel.io/vraxel/lib/ansible/template"
)

// ModuleTemplate renders a template file and uploads to remote host.
// Args:
//
//	src: template file path (required)
//	dest: destination path on remote (required)
//	mode: file mode (optional, default 0644)
//	owner: file owner (optional)
//	group: file group (optional)
func ModuleTemplate(ctx context.Context, opts internal.ExecOptions) (string, string, error) {
	src := internal.StringArg(opts.Args, "src")
	dest := internal.StringArg(opts.Args, "dest")
	if src == "" || dest == "" {
		return "", "", fmt.Errorf("template: src and dest are required")
	}

	mode := internal.FileModeArg(opts.Args, "mode", 0644)
	owner := internal.StringArg(opts.Args, "owner")
	group := internal.StringArg(opts.Args, "group")

	// 1. Read template file from Source.
	data, err := internal.ReadSource(opts.Source, src)
	if err != nil {
		return "", "", fmt.Errorf("template: read %s: %w", src, err)
	}

	// 2. Get all host variables.
	vars := opts.GetAllVariables()

	// 3. Render template.
	rendered, err := tmpl.Parse(vars, string(data))
	if err != nil {
		return "", "", fmt.Errorf("template: render %s: %w", src, err)
	}

	// 4. Upload rendered content.
	if opts.Become {
		// Write to temp file first, then sudo mv to avoid SFTP permission issues
		tmpPath := fmt.Sprintf("/tmp/.ansible_tpl_%d", time.Now().UnixNano())
		if err := opts.Connector.PutFile(ctx, rendered, tmpPath, mode); err != nil {
			return "", "", fmt.Errorf("template: write temp %s: %w", tmpPath, err)
		}
		mvCmd := fmt.Sprintf("mv %s %s && chmod %04o %s", tmpPath, dest, mode, dest)
		if _, _, err := opts.Connector.ExecuteCommand(ctx, mvCmd); err != nil {
			return "", "", fmt.Errorf("template: move to %s: %w", dest, err)
		}
	} else {
		if err := opts.Connector.PutFile(ctx, rendered, dest, mode); err != nil {
			return "", "", fmt.Errorf("template: upload %s: %w", dest, err)
		}
	}

	// 5. Set ownership if specified.
	if chownArg := buildChownArg(owner, group); chownArg != "" {
		cmd := fmt.Sprintf("chown %s %s", chownArg, dest)
		if _, _, err := opts.Connector.ExecuteCommand(ctx, cmd); err != nil {
			return "", "", fmt.Errorf("template: chown %s: %w", dest, err)
		}
	}

	return fmt.Sprintf("templated %s -> %s", src, dest), "", nil
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
