package debug

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"vraxel.io/vraxel/lib/ansible/modules/internal"
	"vraxel.io/vraxel/lib/ansible/template"
	"vraxel.io/vraxel/lib/ansible/variable"
)

// ModuleDebug prints debug messages or variable values.
// Args (one of):
//
//	msg: message string, supports {{ template }} syntax with filters
//	var: variable path ("{{ .a.b }}" or ".a.b") whose value is printed
func ModuleDebug(ctx context.Context, opts internal.ExecOptions) (string, string, error) {
	vars := opts.GetAllVariables()

	if v, ok := opts.Args["var"]; ok {
		return handleVar(v, vars, opts.LogOutput)
	}
	if v, ok := opts.Args["msg"]; ok {
		return handleMsg(v, vars, opts.LogOutput)
	}
	return "", "", fmt.Errorf(`debug: either "msg" or "var" is required`)
}

// handleMsg prints a message, rendering {{ }} template syntax against vars.
func handleMsg(v any, vars map[string]any, out io.Writer) (string, string, error) {
	s, ok := v.(string)
	if !ok {
		return writeDebug(v, out), "", nil
	}
	rendered, err := template.ParseString(vars, s)
	if err != nil {
		return "", "", fmt.Errorf("debug: render msg: %w", err)
	}
	return writeDebug(rendered, out), "", nil
}

// handleVar resolves a variable path and prints its value. The path may be
// wrapped in template syntax ("{{ .a.b }}") or given directly (".a.b"); it must
// start with ".".
func handleVar(v any, vars map[string]any, out io.Writer) (string, string, error) {
	s, ok := v.(string)
	if !ok {
		return writeDebug(v, out), "", nil
	}
	if template.IsTmplSyntax(s) {
		s = template.TrimTmplSyntax(s)
	}
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, ".") {
		return "", "", fmt.Errorf(`debug: var path must start with "."`)
	}
	path := strings.TrimPrefix(s, ".")
	if path == "" {
		return "", "", fmt.Errorf("debug: var path cannot be empty")
	}
	return writeDebug(variable.PrintVar(vars, path), out), "", nil
}

// writeDebug renders data (strings verbatim, other types as pretty JSON),
// echoes it to out when set, and returns it.
func writeDebug(data any, out io.Writer) string {
	var msg string
	switch v := data.(type) {
	case string:
		msg = v
	case []byte:
		msg = string(v)
	default:
		if b, err := json.MarshalIndent(v, "", "  "); err == nil {
			msg = string(b)
		}
	}
	if out != nil {
		fmt.Fprintln(out, "DEBUG:", msg)
	}
	return msg
}
