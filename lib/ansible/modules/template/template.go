package template

import (
	"context"
	"fmt"

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
	if err := internal.WriteFile(ctx, opts, rendered, dest, mode); err != nil {
		return "", "", fmt.Errorf("template: %w", err)
	}

	// 5. Set ownership if specified.
	if chownArg := internal.ChownArg(owner, group); chownArg != "" {
		cmd := fmt.Sprintf("chown %s %s", chownArg, internal.ShellQuote(dest))
		if _, _, err := opts.Connector.ExecuteCommand(ctx, cmd); err != nil {
			return "", "", fmt.Errorf("template: chown %s: %w", dest, err)
		}
	}

	return fmt.Sprintf("templated %s -> %s", src, dest), "", nil
}
