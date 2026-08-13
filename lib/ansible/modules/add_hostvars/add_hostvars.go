package add_hostvars

import (
	"context"
	"fmt"

	"vraxel.io/vraxel/lib/ansible/modules/internal"
	"vraxel.io/vraxel/lib/ansible/variable"
)

// ModuleAddHostvars adds variables to a specific host's RuntimeVars.
//
// Args:
//   - "host": target hostname (optional; defaults to opts.Host)
//   - All other key-value pairs are merged as variables.
func ModuleAddHostvars(_ context.Context, opts internal.ExecOptions) (string, string, error) {
	host := internal.StringArg(opts.Args, "host")
	if host == "" {
		host = opts.Host
	}

	vars := make(map[string]any, len(opts.Args))
	for k, v := range opts.Args {
		if k == "host" {
			continue
		}
		vars[k] = v
	}

	if len(vars) == 0 {
		return "", "", fmt.Errorf("add_hostvars: no variables provided")
	}

	opts.Variable.Merge(variable.MergeHostRuntimeVars(host, vars))
	return "hostvars added", "", nil
}
