package executor

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"gopkg.in/yaml.v3"

	"vraxel.io/vraxel/lib/ansible"
	"vraxel.io/vraxel/lib/ansible/modules"
	"vraxel.io/vraxel/lib/ansible/variable"
)

// setupPlaybookExecutor creates a PlaybookExecutor with localhost and a memSource for testing.
func setupPlaybookExecutor(t *testing.T, inv ansible.Inventory, files map[string][]byte, opts ...Option) (*PlaybookExecutor, *bytes.Buffer) {
	t.Helper()

	var logBuf bytes.Buffer
	src := &memSource{files: files}

	allOpts := []Option{WithLogOutput(&logBuf)}
	allOpts = append(allOpts, opts...)

	exec := NewPlaybookExecutor(inv, src, allOpts...)
	return exec, &logBuf
}

// localhostInventory returns an Inventory with a single localhost host.
func localhostInventory() ansible.Inventory {
	return ansible.Inventory{
		Hosts: map[string]map[string]any{
			"localhost": {"connection": "local"},
		},
	}
}

// multiHostInventory returns an Inventory with multiple hosts (all local).
func multiHostInventory(hosts ...string) ansible.Inventory {
	hostMap := make(map[string]map[string]any, len(hosts))
	for _, h := range hosts {
		hostMap[h] = map[string]any{"connection": "local"}
	}
	return ansible.Inventory{Hosts: hostMap}
}

func TestPlaybookExecutor_SimplePlaybook(t *testing.T) {
	exec, logBuf := setupPlaybookExecutor(t, localhostInventory(), nil)

	playbook := &ansible.Playbook{
		Play: []ansible.Play{
			{
				Base: ansible.Base{Name: "test play"},
				PlayHost: ansible.PlayHost{
					Hosts: []string{"localhost"},
				},
				Tasks: []ansible.Block{
					{
						BlockBase: ansible.BlockBase{
							Base: ansible.Base{Name: "echo hello"},
						},
						Task: ansible.Task{
							UnknownField: map[string]any{
								"command": map[string]any{"cmd": "echo hello"},
							},
						},
					},
				},
			},
		},
	}

	result, err := exec.Execute(context.Background(), playbook)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success=true, got false: %s", result.Error)
	}
	if result.StartTime.IsZero() {
		t.Error("expected StartTime to be set")
	}
	if result.EndTime.IsZero() {
		t.Error("expected EndTime to be set")
	}
	if !result.EndTime.After(result.StartTime) && !result.EndTime.Equal(result.StartTime) {
		t.Error("expected EndTime >= StartTime")
	}

	logOutput := logBuf.String()
	if !strings.Contains(logOutput, "PLAY [test play]") {
		t.Errorf("expected log to contain 'PLAY [test play]', got: %q", logOutput)
	}
	if !strings.Contains(logOutput, "TASKS") {
		t.Errorf("expected log to contain 'TASKS', got: %q", logOutput)
	}
}

func TestPlaybookExecutor_WithVars(t *testing.T) {
	var capturedVars map[string]any
	modules.RegisterModule("_test_pb_vars", func(ctx context.Context, opts modules.ExecOptions) (string, string, error) {
		rawVars := opts.Variable.Get(variable.GetAllVariable(opts.Host))
		if vars, ok := rawVars.(map[string]any); ok {
			capturedVars = vars
		}
		return "ok", "", nil
	})

	exec, _ := setupPlaybookExecutor(t, localhostInventory(), nil)

	playbook := &ansible.Playbook{
		Play: []ansible.Play{
			{
				Base: ansible.Base{
					Name: "vars play",
					Vars: makeVars(t, map[string]any{
						"app_name": "myapp",
						"app_port": 8080,
					}),
				},
				PlayHost: ansible.PlayHost{
					Hosts: []string{"localhost"},
				},
				Tasks: []ansible.Block{
					{
						BlockBase: ansible.BlockBase{
							Base: ansible.Base{Name: "check vars"},
						},
						Task: ansible.Task{
							UnknownField: map[string]any{
								"_test_pb_vars": map[string]any{},
							},
						},
					},
				},
			},
		},
	}

	capturedVars = nil
	result, err := exec.Execute(context.Background(), playbook)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got: %s", result.Error)
	}

	if capturedVars == nil {
		t.Fatal("expected captured vars, got nil")
	}
	if v, ok := capturedVars["app_name"]; !ok {
		t.Error("expected 'app_name' in vars")
	} else if v != "myapp" {
		t.Errorf("expected app_name=myapp, got %v", v)
	}
	if v, ok := capturedVars["app_port"]; !ok {
		t.Error("expected 'app_port' in vars")
	} else if fmt.Sprintf("%v", v) != "8080" {
		t.Errorf("expected app_port=8080, got %v", v)
	}
}

func TestPlaybookExecutor_PrePostTasks(t *testing.T) {
	var execOrder []string
	modules.RegisterModule("_test_pb_order", func(ctx context.Context, opts modules.ExecOptions) (string, string, error) {
		phase, _ := opts.Args["phase"].(string)
		execOrder = append(execOrder, phase)
		return "ok", "", nil
	})
	defer func() { execOrder = nil }()

	exec, logBuf := setupPlaybookExecutor(t, localhostInventory(), nil)

	playbook := &ansible.Playbook{
		Play: []ansible.Play{
			{
				Base: ansible.Base{Name: "ordered play"},
				PlayHost: ansible.PlayHost{
					Hosts: []string{"localhost"},
				},
				PreTasks: []ansible.Block{
					{
						BlockBase: ansible.BlockBase{
							Base: ansible.Base{Name: "pre task"},
						},
						Task: ansible.Task{
							UnknownField: map[string]any{
								"_test_pb_order": map[string]any{"phase": "pre"},
							},
						},
					},
				},
				Tasks: []ansible.Block{
					{
						BlockBase: ansible.BlockBase{
							Base: ansible.Base{Name: "main task"},
						},
						Task: ansible.Task{
							UnknownField: map[string]any{
								"_test_pb_order": map[string]any{"phase": "main"},
							},
						},
					},
				},
				PostTasks: []ansible.Block{
					{
						BlockBase: ansible.BlockBase{
							Base: ansible.Base{Name: "post task"},
						},
						Task: ansible.Task{
							UnknownField: map[string]any{
								"_test_pb_order": map[string]any{"phase": "post"},
							},
						},
					},
				},
			},
		},
	}

	execOrder = nil
	result, err := exec.Execute(context.Background(), playbook)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got: %s", result.Error)
	}

	// Verify order: pre -> main -> post
	if len(execOrder) != 3 {
		t.Fatalf("expected 3 phases executed, got %d: %v", len(execOrder), execOrder)
	}
	if execOrder[0] != "pre" {
		t.Errorf("expected first phase 'pre', got %q", execOrder[0])
	}
	if execOrder[1] != "main" {
		t.Errorf("expected second phase 'main', got %q", execOrder[1])
	}
	if execOrder[2] != "post" {
		t.Errorf("expected third phase 'post', got %q", execOrder[2])
	}

	logOutput := logBuf.String()
	if !strings.Contains(logOutput, "PRE TASKS") {
		t.Errorf("expected log to contain 'PRE TASKS', got: %q", logOutput)
	}
	if !strings.Contains(logOutput, "TASKS") {
		t.Errorf("expected log to contain 'TASKS', got: %q", logOutput)
	}
	if !strings.Contains(logOutput, "POST TASKS") {
		t.Errorf("expected log to contain 'POST TASKS', got: %q", logOutput)
	}
}

func TestPlaybookExecutor_WithRole(t *testing.T) {
	var execOrder []string
	modules.RegisterModule("_test_pb_role", func(ctx context.Context, opts modules.ExecOptions) (string, string, error) {
		phase, _ := opts.Args["phase"].(string)
		execOrder = append(execOrder, phase)
		return "ok", "", nil
	})
	defer func() { execOrder = nil }()

	files := map[string][]byte{
		"roles/test-role/tasks/main.yml": []byte(`
- name: role task
  _test_pb_role:
    phase: role
`),
	}

	exec, logBuf := setupPlaybookExecutor(t, localhostInventory(), files)

	playbook := &ansible.Playbook{
		Play: []ansible.Play{
			{
				Base: ansible.Base{Name: "role play"},
				PlayHost: ansible.PlayHost{
					Hosts: []string{"localhost"},
				},
				Tasks: []ansible.Block{
					{
						BlockBase: ansible.BlockBase{
							Base: ansible.Base{Name: "before role"},
						},
						Task: ansible.Task{
							UnknownField: map[string]any{
								"_test_pb_role": map[string]any{"phase": "task"},
							},
						},
					},
				},
				Roles: []ansible.Role{
					{
						RoleInfo: ansible.RoleInfo{
							Role: "test-role",
						},
					},
				},
			},
		},
	}

	execOrder = nil
	result, err := exec.Execute(context.Background(), playbook)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got: %s", result.Error)
	}

	// Roles execute before tasks
	if len(execOrder) != 2 {
		t.Fatalf("expected 2 phases executed, got %d: %v", len(execOrder), execOrder)
	}
	if execOrder[0] != "role" {
		t.Errorf("expected first phase 'role', got %q", execOrder[0])
	}
	if execOrder[1] != "task" {
		t.Errorf("expected second phase 'task', got %q", execOrder[1])
	}

	logOutput := logBuf.String()
	if !strings.Contains(logOutput, "ROLE [test-role]") {
		t.Errorf("expected log to contain 'ROLE [test-role]', got: %q", logOutput)
	}
}

func TestPlaybookExecutor_Serial(t *testing.T) {
	var batches [][]string
	var currentBatch []string
	batchNum := 0
	modules.RegisterModule("_test_pb_serial", func(ctx context.Context, opts modules.ExecOptions) (string, string, error) {
		currentBatch = append(currentBatch, opts.Host)
		return "ok", "", nil
	})

	inv := ansible.Inventory{
		Hosts: map[string]map[string]any{
			"host1": {"connection": "local"},
			"host2": {"connection": "local"},
			"host3": {"connection": "local"},
		},
		Groups: map[string]ansible.InventoryGroup{
			"all": {Hosts: []string{"host1", "host2", "host3"}},
		},
	}

	exec, _ := setupPlaybookExecutor(t, inv, nil)

	// We'll use a tracking module that gets called in batches.
	// Since serial=1, we expect 3 batches of 1 host each.
	// However, since tasks execute concurrently per batch, we need to track batches differently.
	// Instead, let's verify the playbook runs successfully with serial batching.

	playbook := &ansible.Playbook{
		Play: []ansible.Play{
			{
				Base: ansible.Base{Name: "serial play"},
				PlayHost: ansible.PlayHost{
					Hosts: []string{"all"},
				},
				Serial: ansible.PlaySerial{Data: []any{1}},
				Tasks: []ansible.Block{
					{
						BlockBase: ansible.BlockBase{
							Base: ansible.Base{Name: "serial task"},
						},
						Task: ansible.Task{
							UnknownField: map[string]any{
								"_test_pb_serial": map[string]any{},
							},
						},
					},
				},
			},
		},
	}

	batches = nil
	currentBatch = nil
	batchNum = 0
	_ = batches
	_ = batchNum

	result, err := exec.Execute(context.Background(), playbook)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got: %s", result.Error)
	}

	// With serial=1 and 3 hosts, all 3 hosts should have executed
	if len(currentBatch) != 3 {
		t.Errorf("expected 3 hosts executed, got %d: %v", len(currentBatch), currentBatch)
	}
}

func TestPlaybookExecutor_ResultModule(t *testing.T) {
	exec, _ := setupPlaybookExecutor(t, localhostInventory(), nil)

	playbook := &ansible.Playbook{
		Play: []ansible.Play{
			{
				Base: ansible.Base{Name: "result play"},
				PlayHost: ansible.PlayHost{
					Hosts: []string{"localhost"},
				},
				Tasks: []ansible.Block{
					{
						BlockBase: ansible.BlockBase{
							Base: ansible.Base{Name: "store result"},
						},
						Task: ansible.Task{
							UnknownField: map[string]any{
								"result": map[string]any{
									"cluster_status": "ready",
									"version":        "1.0",
								},
							},
						},
					},
				},
			},
		},
	}

	result, err := exec.Execute(context.Background(), playbook)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got: %s", result.Error)
	}

	// Verify the result module stored data in the variable system
	resultVars := exec.variable.Get(variable.GetResultVariable())
	rv, ok := resultVars.(map[string]any)
	if !ok {
		t.Fatalf("expected result variables to be map[string]any, got %T", resultVars)
	}
	if rv["cluster_status"] != "ready" {
		t.Errorf("expected cluster_status=ready, got %v", rv["cluster_status"])
	}
	if rv["version"] != "1.0" {
		t.Errorf("expected version=1.0, got %v", rv["version"])
	}
}

func TestPlaybookExecutor_TaskFailure(t *testing.T) {
	modules.RegisterModule("_test_pb_fail", func(ctx context.Context, opts modules.ExecOptions) (string, string, error) {
		return "", "", fmt.Errorf("task failed intentionally")
	})

	exec, _ := setupPlaybookExecutor(t, localhostInventory(), nil)

	playbook := &ansible.Playbook{
		Play: []ansible.Play{
			{
				Base: ansible.Base{Name: "failing play"},
				PlayHost: ansible.PlayHost{
					Hosts: []string{"localhost"},
				},
				Tasks: []ansible.Block{
					{
						BlockBase: ansible.BlockBase{
							Base: ansible.Base{Name: "failing task"},
						},
						Task: ansible.Task{
							UnknownField: map[string]any{
								"_test_pb_fail": map[string]any{},
							},
						},
					},
				},
			},
		},
	}

	result, err := exec.Execute(context.Background(), playbook)
	if err == nil {
		t.Fatal("expected error from failing task")
	}
	if result.Success {
		t.Error("expected success=false on failure")
	}
	if result.Error == "" {
		t.Error("expected result.Error to be set")
	}
	if !strings.Contains(err.Error(), "failing play") {
		t.Errorf("expected error to mention play name, got: %v", err)
	}
}

func TestPlaybookExecutor_EmptyHosts(t *testing.T) {
	// Inventory has host1 but play targets "nonexistent"
	inv := ansible.Inventory{
		Hosts: map[string]map[string]any{
			"host1": {"connection": "local"},
		},
	}

	exec, _ := setupPlaybookExecutor(t, inv, nil)

	playbook := &ansible.Playbook{
		Play: []ansible.Play{
			{
				Base: ansible.Base{Name: "empty hosts play"},
				PlayHost: ansible.PlayHost{
					Hosts: []string{"nonexistent"},
				},
				Tasks: []ansible.Block{
					{
						BlockBase: ansible.BlockBase{
							Base: ansible.Base{Name: "should not run"},
						},
						Task: ansible.Task{
							UnknownField: map[string]any{
								"command": map[string]any{"cmd": "echo nope"},
							},
						},
					},
				},
			},
		},
	}

	result, err := exec.Execute(context.Background(), playbook)
	// Standard ansible semantics: a play matching no hosts is skipped, not a
	// fatal error -- the playbook continues and succeeds, and the skipped
	// play's task ("should not run") never executes.
	if err != nil {
		t.Fatalf("expected no error when a play matches no hosts (it should be skipped), got: %v", err)
	}
	if !result.Success {
		t.Error("expected success=true when a play matches no hosts (play skipped)")
	}
}

func TestPlaybookExecutor_MultiplePlays(t *testing.T) {
	var execOrder []string
	modules.RegisterModule("_test_pb_multi_play", func(ctx context.Context, opts modules.ExecOptions) (string, string, error) {
		phase, _ := opts.Args["phase"].(string)
		execOrder = append(execOrder, phase)
		return "ok", "", nil
	})
	defer func() { execOrder = nil }()

	exec, _ := setupPlaybookExecutor(t, localhostInventory(), nil)

	playbook := &ansible.Playbook{
		Play: []ansible.Play{
			{
				Base: ansible.Base{Name: "play one"},
				PlayHost: ansible.PlayHost{
					Hosts: []string{"localhost"},
				},
				Tasks: []ansible.Block{
					{
						BlockBase: ansible.BlockBase{
							Base: ansible.Base{Name: "play 1 task"},
						},
						Task: ansible.Task{
							UnknownField: map[string]any{
								"_test_pb_multi_play": map[string]any{"phase": "play1"},
							},
						},
					},
				},
			},
			{
				Base: ansible.Base{Name: "play two"},
				PlayHost: ansible.PlayHost{
					Hosts: []string{"localhost"},
				},
				Tasks: []ansible.Block{
					{
						BlockBase: ansible.BlockBase{
							Base: ansible.Base{Name: "play 2 task"},
						},
						Task: ansible.Task{
							UnknownField: map[string]any{
								"_test_pb_multi_play": map[string]any{"phase": "play2"},
							},
						},
					},
				},
			},
		},
	}

	execOrder = nil
	result, err := exec.Execute(context.Background(), playbook)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got: %s", result.Error)
	}

	if len(execOrder) != 2 {
		t.Fatalf("expected 2 phases executed, got %d: %v", len(execOrder), execOrder)
	}
	if execOrder[0] != "play1" {
		t.Errorf("expected first phase 'play1', got %q", execOrder[0])
	}
	if execOrder[1] != "play2" {
		t.Errorf("expected second phase 'play2', got %q", execOrder[1])
	}
}

func TestPlaybookExecutor_FullOrchestration(t *testing.T) {
	// Test full orchestration: pre_tasks -> roles -> tasks -> post_tasks
	var execOrder []string
	modules.RegisterModule("_test_pb_full", func(ctx context.Context, opts modules.ExecOptions) (string, string, error) {
		phase, _ := opts.Args["phase"].(string)
		execOrder = append(execOrder, phase)
		return "ok", "", nil
	})
	defer func() { execOrder = nil }()

	files := map[string][]byte{
		"roles/setup-role/tasks/main.yml": []byte(`
- name: role setup task
  _test_pb_full:
    phase: role
`),
	}

	exec, _ := setupPlaybookExecutor(t, localhostInventory(), files)

	playbook := &ansible.Playbook{
		Play: []ansible.Play{
			{
				Base: ansible.Base{Name: "full orchestration"},
				PlayHost: ansible.PlayHost{
					Hosts: []string{"localhost"},
				},
				PreTasks: []ansible.Block{
					{
						BlockBase: ansible.BlockBase{
							Base: ansible.Base{Name: "pre task"},
						},
						Task: ansible.Task{
							UnknownField: map[string]any{
								"_test_pb_full": map[string]any{"phase": "pre"},
							},
						},
					},
				},
				Roles: []ansible.Role{
					{
						RoleInfo: ansible.RoleInfo{
							Role: "setup-role",
						},
					},
				},
				Tasks: []ansible.Block{
					{
						BlockBase: ansible.BlockBase{
							Base: ansible.Base{Name: "main task"},
						},
						Task: ansible.Task{
							UnknownField: map[string]any{
								"_test_pb_full": map[string]any{"phase": "task"},
							},
						},
					},
				},
				PostTasks: []ansible.Block{
					{
						BlockBase: ansible.BlockBase{
							Base: ansible.Base{Name: "post task"},
						},
						Task: ansible.Task{
							UnknownField: map[string]any{
								"_test_pb_full": map[string]any{"phase": "post"},
							},
						},
					},
				},
			},
		},
	}

	execOrder = nil
	result, err := exec.Execute(context.Background(), playbook)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got: %s", result.Error)
	}

	// Verify order: pre -> role -> task -> post
	if len(execOrder) != 4 {
		t.Fatalf("expected 4 phases executed, got %d: %v", len(execOrder), execOrder)
	}
	expected := []string{"pre", "role", "task", "post"}
	for i, exp := range expected {
		if execOrder[i] != exp {
			t.Errorf("expected phase %d to be %q, got %q", i, exp, execOrder[i])
		}
	}
}

func TestPlaybookExecutor_Tags(t *testing.T) {
	callCount := 0
	modules.RegisterModule("_test_pb_tags", func(ctx context.Context, opts modules.ExecOptions) (string, string, error) {
		callCount++
		return "ok", "", nil
	})

	exec, _ := setupPlaybookExecutor(t, localhostInventory(), nil, WithTags([]string{"deploy"}))

	playbook := &ansible.Playbook{
		Play: []ansible.Play{
			{
				Base: ansible.Base{Name: "tags play"},
				PlayHost: ansible.PlayHost{
					Hosts: []string{"localhost"},
				},
				Tasks: []ansible.Block{
					{
						BlockBase: ansible.BlockBase{
							Base: ansible.Base{Name: "tagged deploy"},
							Taggable: ansible.Taggable{
								Tags: ansible.Tags{Data: []string{"deploy"}},
							},
						},
						Task: ansible.Task{
							UnknownField: map[string]any{
								"_test_pb_tags": map[string]any{},
							},
						},
					},
					{
						BlockBase: ansible.BlockBase{
							Base: ansible.Base{Name: "tagged build"},
							Taggable: ansible.Taggable{
								Tags: ansible.Tags{Data: []string{"build"}},
							},
						},
						Task: ansible.Task{
							UnknownField: map[string]any{
								"_test_pb_tags": map[string]any{},
							},
						},
					},
				},
			},
		},
	}

	callCount = 0
	result, err := exec.Execute(context.Background(), playbook)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got: %s", result.Error)
	}

	// Only the "deploy" tagged task should run
	if callCount != 1 {
		t.Errorf("expected 1 task executed (tagged 'deploy'), got %d", callCount)
	}
}

func TestPlaybookExecutor_SkipTags(t *testing.T) {
	callCount := 0
	modules.RegisterModule("_test_pb_skip_tags", func(ctx context.Context, opts modules.ExecOptions) (string, string, error) {
		callCount++
		return "ok", "", nil
	})

	exec, _ := setupPlaybookExecutor(t, localhostInventory(), nil, WithSkipTags([]string{"slow"}))

	playbook := &ansible.Playbook{
		Play: []ansible.Play{
			{
				Base: ansible.Base{Name: "skip tags play"},
				PlayHost: ansible.PlayHost{
					Hosts: []string{"localhost"},
				},
				Tasks: []ansible.Block{
					{
						BlockBase: ansible.BlockBase{
							Base: ansible.Base{Name: "fast task"},
						},
						Task: ansible.Task{
							UnknownField: map[string]any{
								"_test_pb_skip_tags": map[string]any{},
							},
						},
					},
					{
						BlockBase: ansible.BlockBase{
							Base: ansible.Base{Name: "slow task"},
							Taggable: ansible.Taggable{
								Tags: ansible.Tags{Data: []string{"slow"}},
							},
						},
						Task: ansible.Task{
							UnknownField: map[string]any{
								"_test_pb_skip_tags": map[string]any{},
							},
						},
					},
				},
			},
		},
	}

	callCount = 0
	result, err := exec.Execute(context.Background(), playbook)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got: %s", result.Error)
	}

	// Only the non-"slow" task should run
	if callCount != 1 {
		t.Errorf("expected 1 task executed (skipping 'slow'), got %d", callCount)
	}
}

func TestPlaybookExecutor_VarsFiles(t *testing.T) {
	var capturedVars map[string]any
	modules.RegisterModule("_test_pb_vars_files", func(ctx context.Context, opts modules.ExecOptions) (string, string, error) {
		rawVars := opts.Variable.Get(variable.GetAllVariable(opts.Host))
		if vars, ok := rawVars.(map[string]any); ok {
			capturedVars = vars
		}
		return "ok", "", nil
	})

	files := map[string][]byte{
		"vars/extra.yml": []byte(`
db_host: localhost
db_port: 5432
`),
	}

	exec, _ := setupPlaybookExecutor(t, localhostInventory(), files)

	playbook := &ansible.Playbook{
		Play: []ansible.Play{
			{
				Base: ansible.Base{Name: "vars files play"},
				PlayHost: ansible.PlayHost{
					Hosts: []string{"localhost"},
				},
				VarsFiles: []string{"vars/extra.yml"},
				Tasks: []ansible.Block{
					{
						BlockBase: ansible.BlockBase{
							Base: ansible.Base{Name: "check vars files"},
						},
						Task: ansible.Task{
							UnknownField: map[string]any{
								"_test_pb_vars_files": map[string]any{},
							},
						},
					},
				},
			},
		},
	}

	capturedVars = nil
	result, err := exec.Execute(context.Background(), playbook)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got: %s", result.Error)
	}

	if capturedVars == nil {
		t.Fatal("expected captured vars, got nil")
	}
	if v, ok := capturedVars["db_host"]; !ok {
		t.Error("expected 'db_host' in vars")
	} else if v != "localhost" {
		t.Errorf("expected db_host=localhost, got %v", v)
	}
	if v, ok := capturedVars["db_port"]; !ok {
		t.Error("expected 'db_port' in vars")
	} else if fmt.Sprintf("%v", v) != "5432" {
		t.Errorf("expected db_port=5432, got %v", v)
	}
}

func TestPlaybookExecutor_IgnoreErrors(t *testing.T) {
	modules.RegisterModule("_test_pb_ignore", func(ctx context.Context, opts modules.ExecOptions) (string, string, error) {
		return "", "", fmt.Errorf("task error")
	})

	ignoreErrors := true

	exec, _ := setupPlaybookExecutor(t, localhostInventory(), nil)

	playbook := &ansible.Playbook{
		Play: []ansible.Play{
			{
				Base: ansible.Base{
					Name:         "ignore errors play",
					IgnoreErrors: &ignoreErrors,
				},
				PlayHost: ansible.PlayHost{
					Hosts: []string{"localhost"},
				},
				Tasks: []ansible.Block{
					{
						BlockBase: ansible.BlockBase{
							Base: ansible.Base{Name: "failing but ignored task"},
						},
						Task: ansible.Task{
							UnknownField: map[string]any{
								"_test_pb_ignore": map[string]any{},
							},
						},
					},
				},
			},
		},
	}

	result, err := exec.Execute(context.Background(), playbook)
	if err != nil {
		t.Fatalf("expected no error with ignore_errors=true, got: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success with ignore_errors=true, got: %s", result.Error)
	}
}

func TestPlaybookExecutor_GroupHostResolution(t *testing.T) {
	var (
		execMu        sync.Mutex
		executedHosts []string
	)
	modules.RegisterModule("_test_pb_groups", func(ctx context.Context, opts modules.ExecOptions) (string, string, error) {
		execMu.Lock()
		executedHosts = append(executedHosts, opts.Host)
		execMu.Unlock()
		return "ok", "", nil
	})
	defer func() { executedHosts = nil }()

	inv := ansible.Inventory{
		Hosts: map[string]map[string]any{
			"web1": {"connection": "local"},
			"web2": {"connection": "local"},
			"db1":  {"connection": "local"},
		},
		Groups: map[string]ansible.InventoryGroup{
			"webservers": {Hosts: []string{"web1", "web2"}},
			"databases":  {Hosts: []string{"db1"}},
		},
	}

	exec, _ := setupPlaybookExecutor(t, inv, nil)

	playbook := &ansible.Playbook{
		Play: []ansible.Play{
			{
				Base: ansible.Base{Name: "group play"},
				PlayHost: ansible.PlayHost{
					Hosts: []string{"webservers"},
				},
				Tasks: []ansible.Block{
					{
						BlockBase: ansible.BlockBase{
							Base: ansible.Base{Name: "web task"},
						},
						Task: ansible.Task{
							UnknownField: map[string]any{
								"_test_pb_groups": map[string]any{},
							},
						},
					},
				},
			},
		},
	}

	executedHosts = nil
	result, err := exec.Execute(context.Background(), playbook)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got: %s", result.Error)
	}

	// Should execute on web1 and web2 only (not db1)
	if len(executedHosts) != 2 {
		t.Fatalf("expected 2 hosts, got %d: %v", len(executedHosts), executedHosts)
	}
	hasWeb1, hasWeb2 := false, false
	for _, h := range executedHosts {
		if h == "web1" {
			hasWeb1 = true
		}
		if h == "web2" {
			hasWeb2 = true
		}
	}
	if !hasWeb1 || !hasWeb2 {
		t.Errorf("expected web1 and web2, got %v", executedHosts)
	}
}

func TestPlaybookExecutor_EmptyPlaybook(t *testing.T) {
	exec, _ := setupPlaybookExecutor(t, localhostInventory(), nil)

	playbook := &ansible.Playbook{
		Play: []ansible.Play{},
	}

	result, err := exec.Execute(context.Background(), playbook)
	if err != nil {
		t.Fatalf("expected no error for empty playbook, got: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success for empty playbook")
	}
}

func TestPlaybookExecutor_SecondPlayFailsAfterFirstSucceeds(t *testing.T) {
	callCount := 0
	modules.RegisterModule("_test_pb_second_fail", func(ctx context.Context, opts modules.ExecOptions) (string, string, error) {
		callCount++
		if callCount == 2 {
			return "", "", fmt.Errorf("second play fails")
		}
		return "ok", "", nil
	})

	exec, _ := setupPlaybookExecutor(t, localhostInventory(), nil)

	playbook := &ansible.Playbook{
		Play: []ansible.Play{
			{
				Base: ansible.Base{Name: "play one"},
				PlayHost: ansible.PlayHost{
					Hosts: []string{"localhost"},
				},
				Tasks: []ansible.Block{
					{
						BlockBase: ansible.BlockBase{
							Base: ansible.Base{Name: "succeeds"},
						},
						Task: ansible.Task{
							UnknownField: map[string]any{
								"_test_pb_second_fail": map[string]any{},
							},
						},
					},
				},
			},
			{
				Base: ansible.Base{Name: "play two"},
				PlayHost: ansible.PlayHost{
					Hosts: []string{"localhost"},
				},
				Tasks: []ansible.Block{
					{
						BlockBase: ansible.BlockBase{
							Base: ansible.Base{Name: "fails"},
						},
						Task: ansible.Task{
							UnknownField: map[string]any{
								"_test_pb_second_fail": map[string]any{},
							},
						},
					},
				},
			},
		},
	}

	callCount = 0
	result, err := exec.Execute(context.Background(), playbook)
	if err == nil {
		t.Fatal("expected error from second play")
	}
	if result.Success {
		t.Error("expected success=false")
	}
	if !strings.Contains(err.Error(), "play two") {
		t.Errorf("expected error to mention 'play two', got: %v", err)
	}
}

func TestPlaybookExecutor_GatherFacts(t *testing.T) {
	// gather_facts calls the "setup" module which requires GatherFacts interface.
	// LocalConnector implements GatherFacts. On non-Linux it returns an empty map,
	// but it should still succeed.
	exec, _ := setupPlaybookExecutor(t, localhostInventory(), nil)

	playbook := &ansible.Playbook{
		Play: []ansible.Play{
			{
				Base: ansible.Base{Name: "gather facts play"},
				PlayHost: ansible.PlayHost{
					Hosts: []string{"localhost"},
				},
				GatherFacts: true,
				Tasks: []ansible.Block{
					{
						BlockBase: ansible.BlockBase{
							Base: ansible.Base{Name: "after facts"},
						},
						Task: ansible.Task{
							UnknownField: map[string]any{
								"command": map[string]any{"cmd": "echo gathered"},
							},
						},
					},
				},
			},
		},
	}

	result, err := exec.Execute(context.Background(), playbook)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got: %s", result.Error)
	}
}

// makeVars creates an ansible.Vars from a map for testing.
func makeVars(t *testing.T, m map[string]any) ansible.Vars {
	t.Helper()
	var node yaml.Node
	if err := node.Encode(m); err != nil {
		t.Fatalf("failed to encode vars: %v", err)
	}
	return ansible.Vars{Nodes: []yaml.Node{node}}
}

func TestPlaybookExecutor_EventCallback(t *testing.T) {
	var events []Event
	cb := func(e Event) { events = append(events, e) }

	exec, _ := setupPlaybookExecutor(t, localhostInventory(), nil, WithEventCallback(cb))

	playbook := &ansible.Playbook{
		Play: []ansible.Play{
			{
				Base:     ansible.Base{Name: "event play"},
				PlayHost: ansible.PlayHost{Hosts: []string{"localhost"}},
				Tasks: []ansible.Block{
					{
						BlockBase: ansible.BlockBase{Base: ansible.Base{Name: "task one"}},
						Task:      ansible.Task{UnknownField: map[string]any{"command": map[string]any{"cmd": "echo one"}}},
					},
					{
						BlockBase: ansible.BlockBase{Base: ansible.Base{Name: "task two"}},
						Task:      ansible.Task{UnknownField: map[string]any{"command": map[string]any{"cmd": "echo two"}}},
					},
				},
			},
		},
	}

	result, err := exec.Execute(context.Background(), playbook)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Error)
	}

	// Expect: play_start, task_start, task_end, task_start, task_end, play_end
	expectedTypes := []EventType{
		EventPlayStart, EventTaskStart, EventTaskEnd, EventTaskStart, EventTaskEnd, EventPlayEnd,
	}
	if len(events) != len(expectedTypes) {
		t.Fatalf("expected %d events, got %d: %+v", len(expectedTypes), len(events), events)
	}
	for i, et := range expectedTypes {
		if events[i].Type != et {
			t.Errorf("event[%d]: expected type %s, got %s", i, et, events[i].Type)
		}
	}

	// Verify play name
	if events[0].Play != "event play" {
		t.Errorf("expected play name 'event play', got %q", events[0].Play)
	}

	// Verify task index/total
	if events[1].Index != 1 || events[1].Total != 2 {
		t.Errorf("task_start[0]: expected index=1 total=2, got index=%d total=%d", events[1].Index, events[1].Total)
	}
	if events[3].Index != 2 || events[3].Total != 2 {
		t.Errorf("task_start[1]: expected index=2 total=2, got index=%d total=%d", events[3].Index, events[3].Total)
	}

	// Verify task_end has status
	if events[2].Status == "" {
		t.Error("task_end[0]: expected non-empty status")
	}
	if events[4].Status == "" {
		t.Error("task_end[1]: expected non-empty status")
	}

	// Verify task_end has duration
	if events[2].Duration <= 0 {
		t.Error("task_end[0]: expected positive duration")
	}

	// Verify play_end
	if events[5].Play != "event play" {
		t.Errorf("play_end: expected play name 'event play', got %q", events[5].Play)
	}
}

func TestPlaybookExecutor_EventCallback_TaskFailure(t *testing.T) {
	modules.RegisterModule("_test_cb_fail", func(ctx context.Context, opts modules.ExecOptions) (string, string, error) {
		return "", "error output", fmt.Errorf("intentional failure")
	})

	var events []Event
	cb := func(e Event) { events = append(events, e) }

	exec, _ := setupPlaybookExecutor(t, localhostInventory(), nil, WithEventCallback(cb))

	playbook := &ansible.Playbook{
		Play: []ansible.Play{
			{
				Base:     ansible.Base{Name: "fail play"},
				PlayHost: ansible.PlayHost{Hosts: []string{"localhost"}},
				Tasks: []ansible.Block{
					{
						BlockBase: ansible.BlockBase{Base: ansible.Base{Name: "ok task"}},
						Task:      ansible.Task{UnknownField: map[string]any{"command": map[string]any{"cmd": "echo ok"}}},
					},
					{
						BlockBase: ansible.BlockBase{Base: ansible.Base{Name: "fail task"}},
						Task:      ansible.Task{UnknownField: map[string]any{"_test_cb_fail": map[string]any{}}},
					},
				},
			},
		},
	}

	_, _ = exec.Execute(context.Background(), playbook)

	// Expect: play_start, task_start(ok), task_end(ok), task_start(fail), task_end(fail), play_end(error)
	if len(events) != 6 {
		t.Fatalf("expected 6 events, got %d: %+v", len(events), events)
	}

	// Failed task_end should have status=failed and error
	failEnd := events[4]
	if failEnd.Type != EventTaskEnd {
		t.Errorf("expected task_end, got %s", failEnd.Type)
	}
	if failEnd.Status != "failed" {
		t.Errorf("expected status 'failed', got %q", failEnd.Status)
	}
	if failEnd.Error == "" {
		t.Error("expected error message in failed task_end")
	}

	// play_end should have error
	playEnd := events[5]
	if playEnd.Type != EventPlayEnd {
		t.Errorf("expected play_end, got %s", playEnd.Type)
	}
	if playEnd.Error == "" {
		t.Error("expected error in play_end")
	}
}

func TestPlaybookExecutor_EventCallback_PrePostTasks(t *testing.T) {
	var events []Event
	cb := func(e Event) { events = append(events, e) }

	exec, _ := setupPlaybookExecutor(t, localhostInventory(), nil, WithEventCallback(cb))

	playbook := &ansible.Playbook{
		Play: []ansible.Play{
			{
				Base:     ansible.Base{Name: "full play"},
				PlayHost: ansible.PlayHost{Hosts: []string{"localhost"}},
				PreTasks: []ansible.Block{
					{
						BlockBase: ansible.BlockBase{Base: ansible.Base{Name: "pre"}},
						Task:      ansible.Task{UnknownField: map[string]any{"command": map[string]any{"cmd": "echo pre"}}},
					},
				},
				Tasks: []ansible.Block{
					{
						BlockBase: ansible.BlockBase{Base: ansible.Base{Name: "main"}},
						Task:      ansible.Task{UnknownField: map[string]any{"command": map[string]any{"cmd": "echo main"}}},
					},
				},
				PostTasks: []ansible.Block{
					{
						BlockBase: ansible.BlockBase{Base: ansible.Base{Name: "post"}},
						Task:      ansible.Task{UnknownField: map[string]any{"command": map[string]any{"cmd": "echo post"}}},
					},
				},
			},
		},
	}

	result, err := exec.Execute(context.Background(), playbook)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Error)
	}

	// play_start + 3*(task_start+task_end) + play_end = 8
	if len(events) != 8 {
		t.Fatalf("expected 8 events, got %d", len(events))
	}

	// Verify task indices are sequential
	taskEvents := []Event{}
	for _, e := range events {
		if e.Type == EventTaskStart || e.Type == EventTaskEnd {
			taskEvents = append(taskEvents, e)
		}
	}
	if len(taskEvents) != 6 {
		t.Fatalf("expected 6 task events, got %d", len(taskEvents))
	}
	// Total should be 3 for all
	for i, te := range taskEvents {
		if te.Total != 3 {
			t.Errorf("taskEvent[%d]: expected total=3, got %d", i, te.Total)
		}
	}
	// Indices: 1,1, 2,2, 3,3
	expectedIndices := []int{1, 1, 2, 2, 3, 3}
	for i, te := range taskEvents {
		if te.Index != expectedIndices[i] {
			t.Errorf("taskEvent[%d]: expected index=%d, got %d", i, expectedIndices[i], te.Index)
		}
	}
}

func TestPlaybookExecutor_NilCallback(t *testing.T) {
	// Ensure existing tests still work — no callback option means nil callback
	exec, _ := setupPlaybookExecutor(t, localhostInventory(), nil)

	playbook := &ansible.Playbook{
		Play: []ansible.Play{
			{
				Base:     ansible.Base{Name: "no callback"},
				PlayHost: ansible.PlayHost{Hosts: []string{"localhost"}},
				Tasks: []ansible.Block{
					{
						BlockBase: ansible.BlockBase{Base: ansible.Base{Name: "task"}},
						Task:      ansible.Task{UnknownField: map[string]any{"command": map[string]any{"cmd": "echo ok"}}},
					},
				},
			},
		},
	}

	result, err := exec.Execute(context.Background(), playbook)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success")
	}
}
