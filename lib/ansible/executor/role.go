package executor

import (
	"context"
	"fmt"
	"io"

	"gopkg.in/yaml.v3"

	"vraxel.io/vraxel/lib/ansible"
	"vraxel.io/vraxel/lib/ansible/project"
	"vraxel.io/vraxel/lib/ansible/variable"
)

// RoleExecutor resolves and executes Ansible roles.
type RoleExecutor struct {
	blockExecutor *BlockExecutor
	source        project.Source
	variable      variable.Variable
	logOutput     io.Writer
	hosts         []string
}

// NewRoleExecutor creates a new role executor.
func NewRoleExecutor(
	blockExec *BlockExecutor,
	src project.Source,
	v variable.Variable,
	hosts []string,
	logOutput io.Writer,
) *RoleExecutor {
	return &RoleExecutor{
		blockExecutor: blockExec,
		source:        src,
		variable:      v,
		logOutput:     logOutput,
		hosts:         hosts,
	}
}

// Exec executes a role.
func (e *RoleExecutor) Exec(ctx context.Context, role ansible.Role) error {
	// 1. Merge role variables (defaults first, then vars — vars override defaults)
	e.mergeRoleVars(role)

	// 2. Recursively execute role dependencies
	for _, dep := range role.Dependencies {
		depExec := &RoleExecutor{
			blockExecutor: e.blockExecutor,
			source:        e.source,
			variable:      e.variable,
			logOutput:     e.logOutput,
			hosts:         e.hosts,
		}
		if err := depExec.Exec(ctx, dep); err != nil {
			return fmt.Errorf("role dependency '%s': %w", dep.Role, err)
		}
	}

	// 3. Load role tasks from standard directory structure
	blocks, err := e.loadRoleTasks(role.Role)
	if err != nil {
		return err
	}

	// 4. If role has inline blocks, use those instead
	if len(role.Blocks) > 0 {
		blocks = role.Blocks
	}

	// 5. Execute blocks via BlockExecutor
	be := *e.blockExecutor
	be.WithRole(role.Role)
	// Merge role-level when/tags
	if len(role.When.Data) > 0 {
		be.WithWhen(role.When.Data)
	}
	return be.Exec(ctx, blocks)
}

// loadRoleTasks loads tasks/main.yml from the role directory.
// Standard role directory structure:
//
//	roles/<role-name>/
//	  tasks/main.yml       — main task list
//	  handlers/main.yml    — handlers
//	  vars/main.yml        — role variables (high priority)
//	  defaults/main.yml    — default variables (low priority)
//	  templates/           — template files
//	  files/               — static files
func (e *RoleExecutor) loadRoleTasks(roleName string) ([]ansible.Block, error) {
	tasksFile := fmt.Sprintf("roles/%s/tasks/main.yml", roleName)
	data, err := e.source.ReadFile(tasksFile)
	if err != nil {
		return nil, fmt.Errorf("role '%s': read tasks: %w", roleName, err)
	}
	var blocks []ansible.Block
	if err := yaml.Unmarshal(data, &blocks); err != nil {
		return nil, fmt.Errorf("role '%s': parse tasks: %w", roleName, err)
	}
	if err := ansible.ValidateBlocks(blocks); err != nil {
		return nil, fmt.Errorf("role '%s': %w", roleName, err)
	}
	return blocks, nil
}

// mergeRoleVars loads and merges role variables.
// Priority (matching Ansible): role defaults (lowest) < inventory < role vars < set_fact (highest)
func (e *RoleExecutor) mergeRoleVars(role ansible.Role) {
	// 1. Load defaults/main.yml → DefaultVars (lowest priority)
	e.loadAndMergeDefaultsFile(fmt.Sprintf("roles/%s/defaults/main.yml", role.Role))

	// 2. Load vars/main.yml → RuntimeVars (high priority, overrides inventory)
	e.loadAndMergeVarsFile(fmt.Sprintf("roles/%s/vars/main.yml", role.Role))

	// 3. Merge inline vars from the role definition → RuntimeVars
	if len(role.Vars.Nodes) > 0 {
		e.variable.Merge(variable.MergeRuntimeVariable(role.Vars.Nodes, e.hosts...))
	}
}

// loadAndMergeDefaultsFile loads a YAML file and merges it into DefaultVars (lowest priority).
// If the file does not exist or fails to parse, it is silently ignored.
func (e *RoleExecutor) loadAndMergeDefaultsFile(path string) {
	if e.source == nil {
		return
	}
	data, err := e.source.ReadFile(path)
	if err != nil {
		return
	}
	var vars map[string]any
	if err := yaml.Unmarshal(data, &vars); err != nil {
		return
	}
	if len(vars) == 0 {
		return
	}
	for _, host := range e.hosts {
		e.variable.Merge(variable.MergeHostDefaultVars(host, vars))
	}
}

// loadAndMergeVarsFile loads a YAML variables file and merges it into RuntimeVars.
// If the file does not exist or fails to parse, it is silently ignored.
func (e *RoleExecutor) loadAndMergeVarsFile(path string) {
	if e.source == nil {
		return
	}
	data, err := e.source.ReadFile(path)
	if err != nil {
		return // file not found or read error — silently skip
	}
	var vars map[string]any
	if err := yaml.Unmarshal(data, &vars); err != nil {
		return // parse error — silently skip
	}
	if len(vars) == 0 {
		return
	}
	for _, host := range e.hosts {
		e.variable.Merge(variable.MergeHostRuntimeVars(host, vars))
	}
}
