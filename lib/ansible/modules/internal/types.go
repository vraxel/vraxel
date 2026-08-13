package internal

import (
	"context"
	"io"

	"vraxel.io/vraxel/lib/ansible/connector"
	"vraxel.io/vraxel/lib/ansible/variable"
)

// Source provides access to playbook files (avoids import cycle with project package).
type Source interface {
	ReadFile(path string) ([]byte, error)
}

// ExecOptions contains options for module execution.
type ExecOptions struct {
	Args      map[string]any      // Module arguments
	Host      string              // Target host
	Variable  variable.Variable   // Variable system
	Connector connector.Connector // Host connector
	Source    Source              // Playbook file source
	LogOutput io.Writer           // Log output writer
	WorkDir   string              // Working directory
	Become    bool                // Whether to use privilege escalation
	// Environment holds the task's rendered "environment:" pairs. Only the
	// command/shell modules apply it: they are the ones that run a user
	// command, which is what an environment can act on.
	Environment map[string]string
}

// ModuleExecFunc is the function signature for module execution.
type ModuleExecFunc func(ctx context.Context, opts ExecOptions) (stdout, stderr string, err error)

// GetAllVariables is a helper that retrieves all variables for the host in ExecOptions.
func (o *ExecOptions) GetAllVariables() map[string]any {
	result := o.Variable.Get(variable.GetAllVariable(o.Host))
	if m, ok := result.(map[string]any); ok {
		return m
	}
	return make(map[string]any)
}
