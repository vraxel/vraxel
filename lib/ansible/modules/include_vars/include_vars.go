package include_vars

import (
	"context"
	"fmt"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"vraxel.io/vraxel/lib/ansible/modules/internal"
	"vraxel.io/vraxel/lib/ansible/variable"
)

// ModuleIncludeVars loads variables from a YAML file and merges them into the
// host's RuntimeVars.
//
// The file path is resolved from the "file" arg key. If "file" is empty, the
// module falls back to the "include_vars" arg key (supporting the shorthand
// syntax: include_vars: vars/main.yml).
//
// Only .yaml and .yml extensions are accepted. The file is read through
// opts.Source (the project file source).
func ModuleIncludeVars(_ context.Context, opts internal.ExecOptions) (string, string, error) {
	file := internal.StringArg(opts.Args, "file")
	if file == "" {
		file = internal.StringArg(opts.Args, "include_vars")
	}
	if file == "" {
		return "", "", fmt.Errorf("include_vars: no file specified")
	}

	ext := filepath.Ext(file)
	if ext != ".yaml" && ext != ".yml" {
		return "", "", fmt.Errorf("include_vars: unsupported file extension %q: only .yaml and .yml files are allowed", ext)
	}

	data, err := internal.ReadSource(opts.Source, file)
	if err != nil {
		return "", "", fmt.Errorf("include_vars: failed to read file %q: %w", file, err)
	}

	var parsed map[string]any
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		return "", "", fmt.Errorf("include_vars: failed to parse YAML from %q: %w", file, err)
	}

	opts.Variable.Merge(variable.MergeHostRuntimeVars(opts.Host, parsed))
	return "vars included", "", nil
}
