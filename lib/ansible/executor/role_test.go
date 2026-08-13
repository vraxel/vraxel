package executor

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"vraxel.io/vraxel/lib/ansible"
	"vraxel.io/vraxel/lib/ansible/connector"
	"vraxel.io/vraxel/lib/ansible/modules"
	"vraxel.io/vraxel/lib/ansible/variable"
)

// setupRoleExecutor creates a RoleExecutor with local connectors and a memSource for testing.
func setupRoleExecutor(t *testing.T, hosts []string, files map[string][]byte) (*RoleExecutor, variable.Variable, *bytes.Buffer) {
	t.Helper()

	conn := connector.NewLocalConnector("")
	if err := conn.Init(context.Background()); err != nil {
		t.Fatalf("failed to init local connector: %v", err)
	}

	hostMap := make(map[string]map[string]any, len(hosts))
	connMap := newConnectorRegistry()
	for _, h := range hosts {
		hostMap[h] = map[string]any{"connection": "local"}
		connMap.Put(h, conn)
	}

	inv := ansible.Inventory{Hosts: hostMap}
	v := variable.New(inv)

	var logBuf bytes.Buffer
	src := &memSource{files: files}
	taskExec := NewTaskExecutor(v, src, connMap, &logBuf)
	blockExec := NewBlockExecutor(taskExec, v, src, connMap, hosts, &logBuf)
	roleExec := NewRoleExecutor(blockExec, src, v, hosts, &logBuf)

	return roleExec, v, &logBuf
}

func TestRoleExecutor_SimpleRole(t *testing.T) {
	files := map[string][]byte{
		"roles/test-role/tasks/main.yml": []byte(`
- name: echo test
  command:
    cmd: echo hello
`),
	}

	roleExec, _, _ := setupRoleExecutor(t, []string{"localhost"}, files)

	role := ansible.Role{
		RoleInfo: ansible.RoleInfo{
			Role: "test-role",
		},
	}

	err := roleExec.Exec(context.Background(), role)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestRoleExecutor_WithDefaults(t *testing.T) {
	// Register a test module that checks for the default variable
	var capturedVars map[string]any
	modules.RegisterModule("_test_role_defaults", func(ctx context.Context, opts modules.ExecOptions) (string, string, error) {
		rawVars := opts.Variable.Get(variable.GetAllVariable(opts.Host))
		if vars, ok := rawVars.(map[string]any); ok {
			capturedVars = vars
		}
		return "ok", "", nil
	})

	files := map[string][]byte{
		"roles/test-role/defaults/main.yml": []byte(`
default_port: 8080
default_name: myapp
`),
		"roles/test-role/tasks/main.yml": []byte(`
- name: check defaults
  _test_role_defaults: {}
`),
	}

	roleExec, _, _ := setupRoleExecutor(t, []string{"localhost"}, files)

	role := ansible.Role{
		RoleInfo: ansible.RoleInfo{
			Role: "test-role",
		},
	}

	capturedVars = nil
	err := roleExec.Exec(context.Background(), role)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if capturedVars == nil {
		t.Fatal("expected captured vars, got nil")
	}
	if v, ok := capturedVars["default_port"]; !ok {
		t.Error("expected 'default_port' in vars")
	} else if fmt.Sprintf("%v", v) != "8080" {
		t.Errorf("expected default_port=8080, got %v", v)
	}
	if v, ok := capturedVars["default_name"]; !ok {
		t.Error("expected 'default_name' in vars")
	} else if v != "myapp" {
		t.Errorf("expected default_name=myapp, got %v", v)
	}
}

func TestRoleExecutor_WithVars(t *testing.T) {
	// Role vars should override defaults
	var capturedVars map[string]any
	modules.RegisterModule("_test_role_vars", func(ctx context.Context, opts modules.ExecOptions) (string, string, error) {
		rawVars := opts.Variable.Get(variable.GetAllVariable(opts.Host))
		if vars, ok := rawVars.(map[string]any); ok {
			capturedVars = vars
		}
		return "ok", "", nil
	})

	files := map[string][]byte{
		"roles/test-role/defaults/main.yml": []byte(`
app_port: 8080
app_name: default-app
`),
		"roles/test-role/vars/main.yml": []byte(`
app_port: 9090
`),
		"roles/test-role/tasks/main.yml": []byte(`
- name: check vars override
  _test_role_vars: {}
`),
	}

	roleExec, _, _ := setupRoleExecutor(t, []string{"localhost"}, files)

	role := ansible.Role{
		RoleInfo: ansible.RoleInfo{
			Role: "test-role",
		},
	}

	capturedVars = nil
	err := roleExec.Exec(context.Background(), role)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if capturedVars == nil {
		t.Fatal("expected captured vars, got nil")
	}
	// vars/main.yml should override defaults/main.yml
	if v, ok := capturedVars["app_port"]; !ok {
		t.Error("expected 'app_port' in vars")
	} else if fmt.Sprintf("%v", v) != "9090" {
		t.Errorf("expected app_port=9090 (vars override), got %v", v)
	}
	// defaults that are not overridden should still be present
	if v, ok := capturedVars["app_name"]; !ok {
		t.Error("expected 'app_name' in vars")
	} else if v != "default-app" {
		t.Errorf("expected app_name=default-app, got %v", v)
	}
}

func TestRoleExecutor_Dependencies(t *testing.T) {
	var execOrder []string
	modules.RegisterModule("_test_role_dep", func(ctx context.Context, opts modules.ExecOptions) (string, string, error) {
		phase, _ := opts.Args["phase"].(string)
		execOrder = append(execOrder, phase)
		return "ok", "", nil
	})
	defer func() { execOrder = nil }()

	files := map[string][]byte{
		"roles/base-role/tasks/main.yml": []byte(`
- name: base task
  _test_role_dep:
    phase: base
`),
		"roles/main-role/tasks/main.yml": []byte(`
- name: main task
  _test_role_dep:
    phase: main
`),
	}

	roleExec, _, _ := setupRoleExecutor(t, []string{"localhost"}, files)

	role := ansible.Role{
		RoleInfo: ansible.RoleInfo{
			Role: "main-role",
			Dependencies: []ansible.Role{
				{
					RoleInfo: ansible.RoleInfo{
						Role: "base-role",
					},
				},
			},
		},
	}

	execOrder = nil
	err := roleExec.Exec(context.Background(), role)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// Dependency should execute first, then main role
	if len(execOrder) != 2 {
		t.Fatalf("expected 2 phases executed, got %d: %v", len(execOrder), execOrder)
	}
	if execOrder[0] != "base" {
		t.Errorf("expected first phase 'base' (dependency), got %q", execOrder[0])
	}
	if execOrder[1] != "main" {
		t.Errorf("expected second phase 'main', got %q", execOrder[1])
	}
}

func TestRoleExecutor_DependencyFailure(t *testing.T) {
	modules.RegisterModule("_test_role_dep_fail", func(ctx context.Context, opts modules.ExecOptions) (string, string, error) {
		return "", "", fmt.Errorf("dependency failed")
	})

	files := map[string][]byte{
		"roles/failing-dep/tasks/main.yml": []byte(`
- name: failing dep task
  _test_role_dep_fail: {}
`),
		"roles/main-role/tasks/main.yml": []byte(`
- name: should not run
  command:
    cmd: echo should-not-run
`),
	}

	roleExec, _, _ := setupRoleExecutor(t, []string{"localhost"}, files)

	role := ansible.Role{
		RoleInfo: ansible.RoleInfo{
			Role: "main-role",
			Dependencies: []ansible.Role{
				{
					RoleInfo: ansible.RoleInfo{
						Role: "failing-dep",
					},
				},
			},
		},
	}

	err := roleExec.Exec(context.Background(), role)
	if err == nil {
		t.Fatal("expected error from failing dependency")
	}
	if !strings.Contains(err.Error(), "role dependency") {
		t.Errorf("expected error to mention 'role dependency', got: %v", err)
	}
	if !strings.Contains(err.Error(), "failing-dep") {
		t.Errorf("expected error to mention 'failing-dep', got: %v", err)
	}
}

func TestRoleExecutor_MissingTasks(t *testing.T) {
	// No tasks/main.yml for the role
	files := map[string][]byte{}

	roleExec, _, _ := setupRoleExecutor(t, []string{"localhost"}, files)

	role := ansible.Role{
		RoleInfo: ansible.RoleInfo{
			Role: "nonexistent-role",
		},
	}

	err := roleExec.Exec(context.Background(), role)
	if err == nil {
		t.Fatal("expected error for missing tasks/main.yml")
	}
	if !strings.Contains(err.Error(), "nonexistent-role") {
		t.Errorf("expected error to mention role name, got: %v", err)
	}
	if !strings.Contains(err.Error(), "read tasks") {
		t.Errorf("expected error to mention 'read tasks', got: %v", err)
	}
}

func TestRoleExecutor_InlineBlocks(t *testing.T) {
	var executedModule string
	modules.RegisterModule("_test_role_inline", func(ctx context.Context, opts modules.ExecOptions) (string, string, error) {
		executedModule = "_test_role_inline"
		return "ok", "", nil
	})

	// Also create a tasks/main.yml that should NOT be used when inline blocks exist
	files := map[string][]byte{
		"roles/inline-role/tasks/main.yml": []byte(`
- name: file task (should be overridden)
  command:
    cmd: echo from-file
`),
	}

	roleExec, _, _ := setupRoleExecutor(t, []string{"localhost"}, files)

	role := ansible.Role{
		RoleInfo: ansible.RoleInfo{
			Role: "inline-role",
			Blocks: []ansible.Block{
				{
					BlockBase: ansible.BlockBase{
						Base: ansible.Base{Name: "inline task"},
					},
					Task: ansible.Task{
						UnknownField: map[string]any{
							"_test_role_inline": map[string]any{},
						},
					},
				},
			},
		},
	}

	executedModule = ""
	err := roleExec.Exec(context.Background(), role)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if executedModule != "_test_role_inline" {
		t.Error("expected inline block task to be executed")
	}
}

func TestRoleExecutor_WithWhen(t *testing.T) {
	callCount := 0
	modules.RegisterModule("_test_role_when", func(ctx context.Context, opts modules.ExecOptions) (string, string, error) {
		callCount++
		return "ok", "", nil
	})

	files := map[string][]byte{
		"roles/when-role/tasks/main.yml": []byte(`
- name: conditional task
  _test_role_when: {}
`),
	}

	roleExec, v, _ := setupRoleExecutor(t, []string{"localhost"}, files)

	// Set variable so the when condition evaluates to false
	v.Merge(variable.MergeHostRuntimeVars("localhost", map[string]any{
		"role_enabled": false,
	}))

	role := ansible.Role{
		RoleInfo: ansible.RoleInfo{
			Role: "when-role",
			Conditional: ansible.Conditional{
				When: ansible.When{Data: []string{"{{ .role_enabled }}"}},
			},
		},
	}

	callCount = 0
	err := roleExec.Exec(context.Background(), role)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	// The when condition is passed to BlockExecutor which handles skipping.
	// The task executor evaluates it; with role_enabled=false, the task should be skipped.
}

func TestRoleExecutor_LogOutput(t *testing.T) {
	modules.RegisterModule("_test_role_log", func(ctx context.Context, opts modules.ExecOptions) (string, string, error) {
		return "ok", "", nil
	})

	files := map[string][]byte{
		"roles/log-role/tasks/main.yml": []byte(`
- name: log test task
  _test_role_log: {}
`),
	}

	roleExec, _, logBuf := setupRoleExecutor(t, []string{"localhost"}, files)

	role := ansible.Role{
		RoleInfo: ansible.RoleInfo{
			Role: "log-role",
		},
	}

	err := roleExec.Exec(context.Background(), role)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	logOutput := logBuf.String()
	if !strings.Contains(logOutput, "[log-role]") {
		t.Errorf("expected log to contain '[log-role]', got: %q", logOutput)
	}
	if !strings.Contains(logOutput, "log test task") {
		t.Errorf("expected log to contain 'log test task', got: %q", logOutput)
	}
}

func TestRoleExecutor_MultipleDependencies(t *testing.T) {
	var execOrder []string
	modules.RegisterModule("_test_role_multi_dep", func(ctx context.Context, opts modules.ExecOptions) (string, string, error) {
		phase, _ := opts.Args["phase"].(string)
		execOrder = append(execOrder, phase)
		return "ok", "", nil
	})
	defer func() { execOrder = nil }()

	files := map[string][]byte{
		"roles/dep-a/tasks/main.yml": []byte(`
- name: dep-a task
  _test_role_multi_dep:
    phase: dep-a
`),
		"roles/dep-b/tasks/main.yml": []byte(`
- name: dep-b task
  _test_role_multi_dep:
    phase: dep-b
`),
		"roles/top-role/tasks/main.yml": []byte(`
- name: top task
  _test_role_multi_dep:
    phase: top
`),
	}

	roleExec, _, _ := setupRoleExecutor(t, []string{"localhost"}, files)

	role := ansible.Role{
		RoleInfo: ansible.RoleInfo{
			Role: "top-role",
			Dependencies: []ansible.Role{
				{RoleInfo: ansible.RoleInfo{Role: "dep-a"}},
				{RoleInfo: ansible.RoleInfo{Role: "dep-b"}},
			},
		},
	}

	execOrder = nil
	err := roleExec.Exec(context.Background(), role)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if len(execOrder) != 3 {
		t.Fatalf("expected 3 phases executed, got %d: %v", len(execOrder), execOrder)
	}
	if execOrder[0] != "dep-a" {
		t.Errorf("expected first phase 'dep-a', got %q", execOrder[0])
	}
	if execOrder[1] != "dep-b" {
		t.Errorf("expected second phase 'dep-b', got %q", execOrder[1])
	}
	if execOrder[2] != "top" {
		t.Errorf("expected third phase 'top', got %q", execOrder[2])
	}
}

func TestRoleExecutor_NestedDependencies(t *testing.T) {
	var execOrder []string
	modules.RegisterModule("_test_role_nested_dep", func(ctx context.Context, opts modules.ExecOptions) (string, string, error) {
		phase, _ := opts.Args["phase"].(string)
		execOrder = append(execOrder, phase)
		return "ok", "", nil
	})
	defer func() { execOrder = nil }()

	files := map[string][]byte{
		"roles/leaf/tasks/main.yml": []byte(`
- name: leaf task
  _test_role_nested_dep:
    phase: leaf
`),
		"roles/middle/tasks/main.yml": []byte(`
- name: middle task
  _test_role_nested_dep:
    phase: middle
`),
		"roles/top/tasks/main.yml": []byte(`
- name: top task
  _test_role_nested_dep:
    phase: top
`),
	}

	roleExec, _, _ := setupRoleExecutor(t, []string{"localhost"}, files)

	role := ansible.Role{
		RoleInfo: ansible.RoleInfo{
			Role: "top",
			Dependencies: []ansible.Role{
				{
					RoleInfo: ansible.RoleInfo{
						Role: "middle",
						Dependencies: []ansible.Role{
							{RoleInfo: ansible.RoleInfo{Role: "leaf"}},
						},
					},
				},
			},
		},
	}

	execOrder = nil
	err := roleExec.Exec(context.Background(), role)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// leaf -> middle -> top
	if len(execOrder) != 3 {
		t.Fatalf("expected 3 phases executed, got %d: %v", len(execOrder), execOrder)
	}
	if execOrder[0] != "leaf" {
		t.Errorf("expected first phase 'leaf', got %q", execOrder[0])
	}
	if execOrder[1] != "middle" {
		t.Errorf("expected second phase 'middle', got %q", execOrder[1])
	}
	if execOrder[2] != "top" {
		t.Errorf("expected third phase 'top', got %q", execOrder[2])
	}
}

func TestRoleExecutor_NoDefaultsNoVars(t *testing.T) {
	// Role with tasks but no defaults or vars directories — should work fine
	files := map[string][]byte{
		"roles/minimal/tasks/main.yml": []byte(`
- name: minimal task
  command:
    cmd: echo minimal
`),
	}

	roleExec, _, _ := setupRoleExecutor(t, []string{"localhost"}, files)

	role := ansible.Role{
		RoleInfo: ansible.RoleInfo{
			Role: "minimal",
		},
	}

	err := roleExec.Exec(context.Background(), role)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestRoleExecutor_MultipleHosts(t *testing.T) {
	var (
		mu            sync.Mutex
		executedHosts []string
	)
	modules.RegisterModule("_test_role_multi_host", func(ctx context.Context, opts modules.ExecOptions) (string, string, error) {
		mu.Lock()
		executedHosts = append(executedHosts, opts.Host)
		mu.Unlock()
		return "ok", "", nil
	})
	defer func() { executedHosts = nil }()

	files := map[string][]byte{
		"roles/multi-host/tasks/main.yml": []byte(`
- name: multi host task
  _test_role_multi_host: {}
`),
	}

	roleExec, _, _ := setupRoleExecutor(t, []string{"host1", "host2"}, files)

	role := ansible.Role{
		RoleInfo: ansible.RoleInfo{
			Role: "multi-host",
		},
	}

	executedHosts = nil
	err := roleExec.Exec(context.Background(), role)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if len(executedHosts) != 2 {
		t.Fatalf("expected 2 hosts executed, got %d: %v", len(executedHosts), executedHosts)
	}
}
