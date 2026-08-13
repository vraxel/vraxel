package debug

import (
	"context"
	"encoding/json"
	"fmt"

	"vraxel.io/vraxel/lib/ansible/modules/internal"
	"vraxel.io/vraxel/lib/ansible/template"
)

// ModuleDebug prints debug messages.
// Args:
//
//	msg: message string (supports {{ template }} syntax)
func ModuleDebug(ctx context.Context, opts internal.ExecOptions) (string, string, error) {
	msg, ok := opts.Args["msg"]
	if !ok {
		return "", "", fmt.Errorf("debug: msg argument required")
	}

	vars := opts.GetAllVariables()

	var output string
	switch v := msg.(type) {
	case string:
		// If contains template syntax, render it.
		rendered, err := template.ParseString(vars, v)
		if err != nil {
			return "", "", fmt.Errorf("debug: render msg: %w", err)
		}
		output = rendered
	default:
		// For non-string (maps, arrays, numbers), pretty-print as JSON.
		data, _ := json.MarshalIndent(v, "", "  ")
		output = string(data)
	}

	// Write to LogOutput if available.
	if opts.LogOutput != nil {
		fmt.Fprintln(opts.LogOutput, "DEBUG:", output)
	}

	return output, "", nil
}
