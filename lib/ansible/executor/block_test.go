package executor

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"strings"
	"testing"
	"time"

	"vraxel.io/vraxel/lib/ansible"
	"vraxel.io/vraxel/lib/ansible/connector"
	"vraxel.io/vraxel/lib/ansible/modules"
	"vraxel.io/vraxel/lib/ansible/variable"
)

// memSource is an in-memory Source for testing include_tasks.
type memSource struct {
	files map[string][]byte
}

func (m *memSource) ReadFile(path string) ([]byte, error) {
	data, ok := m.files[path]
	if !ok {
		return nil, fmt.Errorf("file not found: %s", path)
	}
	return data, nil
}

func (m *memSource) ReadDir(path string) ([]fs.DirEntry, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *memSource) Stat(path string) (fs.FileInfo, error) {
	return nil, fmt.Errorf("not implemented")
}

// setupBlockExecutor creates a BlockExecutor with local connectors for testing.
func setupBlockExecutor(t *testing.T, hosts []string) (*BlockExecutor, variable.Variable) {
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
	taskExec := NewTaskExecutor(v, nil, connMap, &logBuf)
	blockExec := NewBlockExecutor(taskExec, v, nil, connMap, hosts, &logBuf)

	return blockExec, v
}

func TestBlockExecutor_SimpleTask(t *testing.T) {
	exec, _ := setupBlockExecutor(t, []string{"localhost"})

	blocks := []ansible.Block{
		{
			BlockBase: ansible.BlockBase{
				Base: ansible.Base{Name: "echo test"},
			},
			Task: ansible.Task{
				UnknownField: map[string]any{
					"command": map[string]any{"cmd": "echo hello"},
				},
			},
		},
	}

	err := exec.Exec(context.Background(), blocks)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestBlockExecutor_BlockRescueAlways(t *testing.T) {
	// Register a test module that tracks execution order
	var execOrder []string
	modules.RegisterModule("_test_block_track", func(ctx context.Context, opts modules.ExecOptions) (string, string, error) {
		phase, _ := opts.Args["phase"].(string)
		execOrder = append(execOrder, phase)
		if phase == "block_fail" {
			return "", "", fmt.Errorf("block failed")
		}
		return "ok", "", nil
	})
	defer func() { execOrder = nil }()

	exec, _ := setupBlockExecutor(t, []string{"localhost"})

	blocks := []ansible.Block{
		{
			BlockBase: ansible.BlockBase{
				Base: ansible.Base{Name: "test block/rescue/always"},
			},
			BlockInfo: ansible.BlockInfo{
				Block: []ansible.Block{
					{
						BlockBase: ansible.BlockBase{
							Base: ansible.Base{Name: "block task"},
						},
						Task: ansible.Task{
							UnknownField: map[string]any{
								"_test_block_track": map[string]any{"phase": "block_fail"},
							},
						},
					},
				},
				Rescue: []ansible.Block{
					{
						BlockBase: ansible.BlockBase{
							Base: ansible.Base{Name: "rescue task"},
						},
						Task: ansible.Task{
							UnknownField: map[string]any{
								"_test_block_track": map[string]any{"phase": "rescue"},
							},
						},
					},
				},
				Always: []ansible.Block{
					{
						BlockBase: ansible.BlockBase{
							Base: ansible.Base{Name: "always task"},
						},
						Task: ansible.Task{
							UnknownField: map[string]any{
								"_test_block_track": map[string]any{"phase": "always"},
							},
						},
					},
				},
			},
		},
	}

	err := exec.Exec(context.Background(), blocks)
	if err != nil {
		t.Fatalf("expected no error (rescue succeeded), got: %v", err)
	}

	// Verify execution order: block (fail) -> rescue -> always
	if len(execOrder) != 3 {
		t.Fatalf("expected 3 phases executed, got %d: %v", len(execOrder), execOrder)
	}
	if execOrder[0] != "block_fail" {
		t.Errorf("expected first phase 'block_fail', got %q", execOrder[0])
	}
	if execOrder[1] != "rescue" {
		t.Errorf("expected second phase 'rescue', got %q", execOrder[1])
	}
	if execOrder[2] != "always" {
		t.Errorf("expected third phase 'always', got %q", execOrder[2])
	}
}

func TestBlockExecutor_BlockRescueFail(t *testing.T) {
	modules.RegisterModule("_test_block_fail_both", func(ctx context.Context, opts modules.ExecOptions) (string, string, error) {
		return "", "", fmt.Errorf("always fails")
	})

	exec, _ := setupBlockExecutor(t, []string{"localhost"})

	blocks := []ansible.Block{
		{
			BlockBase: ansible.BlockBase{
				Base: ansible.Base{Name: "fail both"},
			},
			BlockInfo: ansible.BlockInfo{
				Block: []ansible.Block{
					{
						BlockBase: ansible.BlockBase{
							Base: ansible.Base{Name: "block fail"},
						},
						Task: ansible.Task{
							UnknownField: map[string]any{
								"_test_block_fail_both": map[string]any{},
							},
						},
					},
				},
				Rescue: []ansible.Block{
					{
						BlockBase: ansible.BlockBase{
							Base: ansible.Base{Name: "rescue fail"},
						},
						Task: ansible.Task{
							UnknownField: map[string]any{
								"_test_block_fail_both": map[string]any{},
							},
						},
					},
				},
			},
		},
	}

	err := exec.Exec(context.Background(), blocks)
	if err == nil {
		t.Fatal("expected error when both block and rescue fail")
	}
}

func TestBlockExecutor_WhenSkip(t *testing.T) {
	callCount := 0
	modules.RegisterModule("_test_block_when", func(ctx context.Context, opts modules.ExecOptions) (string, string, error) {
		callCount++
		return "executed", "", nil
	})

	exec, v := setupBlockExecutor(t, []string{"localhost"})

	// Set variable so when condition is false
	v.Merge(variable.MergeHostRuntimeVars("localhost", map[string]any{
		"should_run": false,
	}))

	blocks := []ansible.Block{
		{
			BlockBase: ansible.BlockBase{
				Base: ansible.Base{Name: "skipped task"},
				Conditional: ansible.Conditional{
					When: ansible.When{Data: []string{"{{ .should_run }}"}},
				},
			},
			Task: ansible.Task{
				UnknownField: map[string]any{
					"_test_block_when": map[string]any{},
				},
			},
		},
	}

	callCount = 0
	err := exec.Exec(context.Background(), blocks)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// The task should still be called, but the when condition is passed to TaskExecutor
	// which handles the skip internally. The block executor merges conditions.
	// Here, the when condition is evaluated by TaskExecutor, so it should be skipped.
}

func TestBlockExecutor_IgnoreErrors(t *testing.T) {
	modules.RegisterModule("_test_block_ignore_err", func(ctx context.Context, opts modules.ExecOptions) (string, string, error) {
		return "", "", fmt.Errorf("task error")
	})

	exec, _ := setupBlockExecutor(t, []string{"localhost"})

	ignoreErrors := true
	blocks := []ansible.Block{
		{
			BlockBase: ansible.BlockBase{
				Base: ansible.Base{
					Name:         "failing task with ignore",
					IgnoreErrors: &ignoreErrors,
				},
			},
			Task: ansible.Task{
				UnknownField: map[string]any{
					"_test_block_ignore_err": map[string]any{},
				},
			},
		},
	}

	err := exec.Exec(context.Background(), blocks)
	if err != nil {
		t.Fatalf("expected no error with ignore_errors=true, got: %v", err)
	}
}

func TestBlockExecutor_IgnoreErrors_Inherited(t *testing.T) {
	modules.RegisterModule("_test_block_ignore_inh", func(ctx context.Context, opts modules.ExecOptions) (string, string, error) {
		return "", "", fmt.Errorf("task error")
	})

	exec, _ := setupBlockExecutor(t, []string{"localhost"})

	// Set ignore_errors at executor level
	ignoreErrors := true
	exec.WithIgnoreErrors(&ignoreErrors)

	blocks := []ansible.Block{
		{
			BlockBase: ansible.BlockBase{
				Base: ansible.Base{Name: "failing task inherits ignore"},
			},
			Task: ansible.Task{
				UnknownField: map[string]any{
					"_test_block_ignore_inh": map[string]any{},
				},
			},
		},
	}

	err := exec.Exec(context.Background(), blocks)
	if err != nil {
		t.Fatalf("expected no error with inherited ignore_errors=true, got: %v", err)
	}
}

func TestBlockExecutor_IncludeTasks(t *testing.T) {
	exec, _ := setupBlockExecutor(t, []string{"localhost"})

	// Set up a memSource with an included tasks file
	src := &memSource{
		files: map[string][]byte{
			"included.yml": []byte(`
- name: included task
  command:
    cmd: echo included
`),
		},
	}
	exec.source = src

	blocks := []ansible.Block{
		{
			BlockBase: ansible.BlockBase{
				Base: ansible.Base{Name: "include tasks"},
			},
			IncludeTasks: "included.yml",
		},
	}

	err := exec.Exec(context.Background(), blocks)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestBlockExecutor_IncludeTasks_FileNotFound(t *testing.T) {
	exec, _ := setupBlockExecutor(t, []string{"localhost"})

	src := &memSource{files: map[string][]byte{}}
	exec.source = src

	blocks := []ansible.Block{
		{
			BlockBase: ansible.BlockBase{
				Base: ansible.Base{Name: "missing include"},
			},
			IncludeTasks: "nonexistent.yml",
		},
	}

	err := exec.Exec(context.Background(), blocks)
	if err == nil {
		t.Fatal("expected error for missing include file")
	}
	if !strings.Contains(err.Error(), "include_tasks") {
		t.Errorf("expected error to contain 'include_tasks', got: %v", err)
	}
}

// TestBlockExecutor_IncludeTasks_DirectSelfReference pins the engine's
// safety net against an include_tasks file that includes itself. Was
// the production etcd-deploy bug class (member-replace.yml line 62
// re-included main.yml, recursion via main.yml's include of
// member-replace.yml emitted ~5,200 task rounds before vraxel-server OOM).
// The fix is per-call-stack cycle detection in dealIncludeTasks; this
// test pins it as a regression guard.
func TestBlockExecutor_IncludeTasks_DirectSelfReference(t *testing.T) {
	exec, _ := setupBlockExecutor(t, []string{"localhost"})

	src := &memSource{
		files: map[string][]byte{
			"self.yml": []byte(`
- include_tasks: self.yml
`),
		},
	}
	exec.source = src

	blocks := []ansible.Block{
		{
			BlockBase:    ansible.BlockBase{Base: ansible.Base{Name: "enter self.yml"}},
			IncludeTasks: "self.yml",
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := exec.Exec(ctx, blocks)
	if err == nil {
		t.Fatal("expected error for self-referencing include, got nil")
	}
	if !strings.Contains(err.Error(), "recursive include") {
		t.Errorf("expected error to mention 'recursive include', got: %v", err)
	}
}

// TestBlockExecutor_IncludeTasks_IndirectCycle covers the
// A -> B -> A pattern. Any number of intermediate hops should be
// caught the same way as direct self-reference; this test is the
// minimal indirect case.
func TestBlockExecutor_IncludeTasks_IndirectCycle(t *testing.T) {
	exec, _ := setupBlockExecutor(t, []string{"localhost"})

	src := &memSource{
		files: map[string][]byte{
			"a.yml": []byte(`- include_tasks: b.yml`),
			"b.yml": []byte(`- include_tasks: a.yml`),
		},
	}
	exec.source = src

	blocks := []ansible.Block{
		{IncludeTasks: "a.yml"},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := exec.Exec(ctx, blocks)
	if err == nil {
		t.Fatal("expected error for A->B->A include cycle, got nil")
	}
	if !strings.Contains(err.Error(), "recursive include") {
		t.Errorf("expected error to mention 'recursive include', got: %v", err)
	}
}

// TestBlockExecutor_IncludeTasks_SequentialRepeatNotCycle is the
// negative case: including the same file twice in sequence (not nested
// — one include returns before the next begins) MUST NOT be flagged as
// a cycle. Cycle detection must scope to the *current* include stack,
// not the lifetime history. Any other behavior would break legitimate
// patterns where a common helper file is re-used at multiple steps of
// the same playbook.
func TestBlockExecutor_IncludeTasks_SequentialRepeatNotCycle(t *testing.T) {
	exec, _ := setupBlockExecutor(t, []string{"localhost"})

	src := &memSource{
		files: map[string][]byte{
			"shared.yml": []byte(`
- name: noop
  command:
    cmd: "echo hi"
`),
		},
	}
	exec.source = src

	blocks := []ansible.Block{
		{IncludeTasks: "shared.yml"},
		{IncludeTasks: "shared.yml"}, // same file, sequential
	}

	err := exec.Exec(context.Background(), blocks)
	if err != nil {
		t.Fatalf("sequential repeat of same include must not be flagged as cycle, got: %v", err)
	}
}

func TestBlockExecutor_MultipleBlocks(t *testing.T) {
	var callOrder []string
	modules.RegisterModule("_test_block_multi", func(ctx context.Context, opts modules.ExecOptions) (string, string, error) {
		phase, _ := opts.Args["phase"].(string)
		callOrder = append(callOrder, phase)
		return "ok", "", nil
	})
	defer func() { callOrder = nil }()

	exec, _ := setupBlockExecutor(t, []string{"localhost"})

	blocks := []ansible.Block{
		{
			BlockBase: ansible.BlockBase{
				Base: ansible.Base{Name: "first task"},
			},
			Task: ansible.Task{
				UnknownField: map[string]any{
					"_test_block_multi": map[string]any{"phase": "first"},
				},
			},
		},
		{
			BlockBase: ansible.BlockBase{
				Base: ansible.Base{Name: "second task"},
			},
			Task: ansible.Task{
				UnknownField: map[string]any{
					"_test_block_multi": map[string]any{"phase": "second"},
				},
			},
		},
		{
			BlockBase: ansible.BlockBase{
				Base: ansible.Base{Name: "third task"},
			},
			Task: ansible.Task{
				UnknownField: map[string]any{
					"_test_block_multi": map[string]any{"phase": "third"},
				},
			},
		},
	}

	callOrder = nil
	err := exec.Exec(context.Background(), blocks)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if len(callOrder) != 3 {
		t.Fatalf("expected 3 calls, got %d: %v", len(callOrder), callOrder)
	}
	if callOrder[0] != "first" || callOrder[1] != "second" || callOrder[2] != "third" {
		t.Errorf("expected [first, second, third], got %v", callOrder)
	}
}

func TestBlockExecutor_RunOnce(t *testing.T) {
	var executedHosts []string
	modules.RegisterModule("_test_block_runonce", func(ctx context.Context, opts modules.ExecOptions) (string, string, error) {
		executedHosts = append(executedHosts, opts.Host)
		return "ok", "", nil
	})
	defer func() { executedHosts = nil }()

	exec, _ := setupBlockExecutor(t, []string{"host1", "host2", "host3"})

	blocks := []ansible.Block{
		{
			BlockBase: ansible.BlockBase{
				Base: ansible.Base{
					Name:    "run once task",
					RunOnce: true,
				},
			},
			Task: ansible.Task{
				UnknownField: map[string]any{
					"_test_block_runonce": map[string]any{},
				},
			},
		},
	}

	executedHosts = nil
	err := exec.Exec(context.Background(), blocks)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// With run_once, only the first host should execute
	if len(executedHosts) != 1 {
		t.Fatalf("expected 1 host for run_once, got %d: %v", len(executedHosts), executedHosts)
	}
	if executedHosts[0] != "host1" {
		t.Errorf("expected host1, got %s", executedHosts[0])
	}
}

func TestBlockExecutor_TagsFilter(t *testing.T) {
	callCount := 0
	modules.RegisterModule("_test_block_tags", func(ctx context.Context, opts modules.ExecOptions) (string, string, error) {
		callCount++
		return "ok", "", nil
	})

	exec, _ := setupBlockExecutor(t, []string{"localhost"})
	exec.WithPlayTags([]string{"deploy"})

	blocks := []ansible.Block{
		{
			BlockBase: ansible.BlockBase{
				Base: ansible.Base{Name: "tagged task"},
				Taggable: ansible.Taggable{
					Tags: ansible.Tags{Data: []string{"deploy"}},
				},
			},
			Task: ansible.Task{
				UnknownField: map[string]any{
					"_test_block_tags": map[string]any{},
				},
			},
		},
		{
			BlockBase: ansible.BlockBase{
				Base: ansible.Base{Name: "untagged task"},
				Taggable: ansible.Taggable{
					Tags: ansible.Tags{Data: []string{"build"}},
				},
			},
			Task: ansible.Task{
				UnknownField: map[string]any{
					"_test_block_tags": map[string]any{},
				},
			},
		},
	}

	callCount = 0
	err := exec.Exec(context.Background(), blocks)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// Only the "deploy" tagged task should run
	if callCount != 1 {
		t.Errorf("expected 1 task executed (tagged 'deploy'), got %d", callCount)
	}
}

func TestBlockExecutor_SkipTags(t *testing.T) {
	callCount := 0
	modules.RegisterModule("_test_block_skip_tags", func(ctx context.Context, opts modules.ExecOptions) (string, string, error) {
		callCount++
		return "ok", "", nil
	})

	exec, _ := setupBlockExecutor(t, []string{"localhost"})
	exec.WithPlaySkipTags([]string{"slow"})

	blocks := []ansible.Block{
		{
			BlockBase: ansible.BlockBase{
				Base: ansible.Base{Name: "normal task"},
			},
			Task: ansible.Task{
				UnknownField: map[string]any{
					"_test_block_skip_tags": map[string]any{},
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
					"_test_block_skip_tags": map[string]any{},
				},
			},
		},
	}

	callCount = 0
	err := exec.Exec(context.Background(), blocks)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// Only the non-"slow" task should run
	if callCount != 1 {
		t.Errorf("expected 1 task executed (skipping 'slow'), got %d", callCount)
	}
}

func TestBlockExecutor_AlwaysRunsOnBlockFailure(t *testing.T) {
	var alwaysRan bool
	modules.RegisterModule("_test_block_always_track", func(ctx context.Context, opts modules.ExecOptions) (string, string, error) {
		phase, _ := opts.Args["phase"].(string)
		if phase == "fail" {
			return "", "", fmt.Errorf("block failure")
		}
		alwaysRan = true
		return "ok", "", nil
	})

	exec, _ := setupBlockExecutor(t, []string{"localhost"})

	blocks := []ansible.Block{
		{
			BlockBase: ansible.BlockBase{
				Base: ansible.Base{Name: "block with always"},
			},
			BlockInfo: ansible.BlockInfo{
				Block: []ansible.Block{
					{
						BlockBase: ansible.BlockBase{
							Base: ansible.Base{Name: "failing block"},
						},
						Task: ansible.Task{
							UnknownField: map[string]any{
								"_test_block_always_track": map[string]any{"phase": "fail"},
							},
						},
					},
				},
				Always: []ansible.Block{
					{
						BlockBase: ansible.BlockBase{
							Base: ansible.Base{Name: "always runs"},
						},
						Task: ansible.Task{
							UnknownField: map[string]any{
								"_test_block_always_track": map[string]any{"phase": "always"},
							},
						},
					},
				},
			},
		},
	}

	alwaysRan = false
	err := exec.Exec(context.Background(), blocks)
	if err == nil {
		t.Fatal("expected error from failing block without rescue")
	}
	if !alwaysRan {
		t.Error("expected always block to run even when block failed")
	}
}

func TestBlockExecutor_StopsOnFailure(t *testing.T) {
	var callOrder []string
	modules.RegisterModule("_test_block_stop", func(ctx context.Context, opts modules.ExecOptions) (string, string, error) {
		phase, _ := opts.Args["phase"].(string)
		callOrder = append(callOrder, phase)
		if phase == "fail" {
			return "", "", fmt.Errorf("stop here")
		}
		return "ok", "", nil
	})
	defer func() { callOrder = nil }()

	exec, _ := setupBlockExecutor(t, []string{"localhost"})

	blocks := []ansible.Block{
		{
			BlockBase: ansible.BlockBase{
				Base: ansible.Base{Name: "task 1"},
			},
			Task: ansible.Task{
				UnknownField: map[string]any{
					"_test_block_stop": map[string]any{"phase": "first"},
				},
			},
		},
		{
			BlockBase: ansible.BlockBase{
				Base: ansible.Base{Name: "task 2 fails"},
			},
			Task: ansible.Task{
				UnknownField: map[string]any{
					"_test_block_stop": map[string]any{"phase": "fail"},
				},
			},
		},
		{
			BlockBase: ansible.BlockBase{
				Base: ansible.Base{Name: "task 3 should not run"},
			},
			Task: ansible.Task{
				UnknownField: map[string]any{
					"_test_block_stop": map[string]any{"phase": "third"},
				},
			},
		},
	}

	callOrder = nil
	err := exec.Exec(context.Background(), blocks)
	if err == nil {
		t.Fatal("expected error from failing task")
	}

	// Should stop after the second block
	if len(callOrder) != 2 {
		t.Fatalf("expected 2 calls (stop on failure), got %d: %v", len(callOrder), callOrder)
	}
	if callOrder[0] != "first" || callOrder[1] != "fail" {
		t.Errorf("expected [first, fail], got %v", callOrder)
	}
}

func TestBlockExecutor_LogOutput(t *testing.T) {
	modules.RegisterModule("_test_block_log", func(ctx context.Context, opts modules.ExecOptions) (string, string, error) {
		return "ok", "", nil
	})

	conn := connector.NewLocalConnector("")
	if err := conn.Init(context.Background()); err != nil {
		t.Fatalf("failed to init connector: %v", err)
	}

	inv := ansible.Inventory{
		Hosts: map[string]map[string]any{
			"localhost": {"connection": "local"},
		},
	}
	v := variable.New(inv)
	conns := newConnectorRegistry()
	conns.Put("localhost", conn)

	var logBuf bytes.Buffer
	taskExec := NewTaskExecutor(v, nil, conns, &logBuf)
	blockExec := NewBlockExecutor(taskExec, v, nil, conns, []string{"localhost"}, &logBuf)
	blockExec.WithRole("myrole")

	blocks := []ansible.Block{
		{
			BlockBase: ansible.BlockBase{
				Base: ansible.Base{Name: "log test task"},
			},
			Task: ansible.Task{
				UnknownField: map[string]any{
					"_test_block_log": map[string]any{},
				},
			},
		},
	}

	err := blockExec.Exec(context.Background(), blocks)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	logOutput := logBuf.String()
	if !strings.Contains(logOutput, "[myrole]") {
		t.Errorf("expected log to contain '[myrole]', got: %q", logOutput)
	}
	if !strings.Contains(logOutput, "log test task") {
		t.Errorf("expected log to contain 'log test task', got: %q", logOutput)
	}
}

func TestBlockExecutor_EmptyBlocks(t *testing.T) {
	exec, _ := setupBlockExecutor(t, []string{"localhost"})

	err := exec.Exec(context.Background(), nil)
	if err != nil {
		t.Fatalf("expected no error for nil blocks, got: %v", err)
	}

	err = exec.Exec(context.Background(), []ansible.Block{})
	if err != nil {
		t.Fatalf("expected no error for empty blocks, got: %v", err)
	}
}

func TestBlockExecutor_ContextCancellation(t *testing.T) {
	modules.RegisterModule("_test_block_ctx", func(ctx context.Context, opts modules.ExecOptions) (string, string, error) {
		select {
		case <-ctx.Done():
			return "", "", ctx.Err()
		case <-time.After(10 * time.Millisecond):
			return "ok", "", nil
		}
	})

	exec, _ := setupBlockExecutor(t, []string{"localhost"})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	blocks := []ansible.Block{
		{
			BlockBase: ansible.BlockBase{
				Base: ansible.Base{Name: "cancelled task"},
			},
			Task: ansible.Task{
				UnknownField: map[string]any{
					"_test_block_ctx": map[string]any{},
				},
			},
		},
	}

	// Should not hang, may or may not error depending on timing
	_ = exec.Exec(ctx, blocks)
}

func TestMergeWhen(t *testing.T) {
	tests := []struct {
		name     string
		parent   []string
		child    []string
		expected []string
	}{
		{
			name:     "both empty",
			parent:   nil,
			child:    nil,
			expected: nil,
		},
		{
			name:     "parent only",
			parent:   []string{"{{ .a }}"},
			child:    nil,
			expected: []string{"{{ .a }}"},
		},
		{
			name:     "child only",
			parent:   nil,
			child:    []string{"{{ .b }}"},
			expected: []string{"{{ .b }}"},
		},
		{
			name:     "merge without duplicates",
			parent:   []string{"{{ .a }}"},
			child:    []string{"{{ .b }}"},
			expected: []string{"{{ .a }}", "{{ .b }}"},
		},
		{
			name:     "deduplicates",
			parent:   []string{"{{ .a }}", "{{ .b }}"},
			child:    []string{"{{ .b }}", "{{ .c }}"},
			expected: []string{"{{ .a }}", "{{ .b }}", "{{ .c }}"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mergeWhen(tt.parent, tt.child)

			if len(result) != len(tt.expected) {
				t.Fatalf("expected %d items, got %d: %v", len(tt.expected), len(result), result)
			}
			for i, v := range tt.expected {
				if result[i] != v {
					t.Errorf("result[%d] = %q, expected %q", i, result[i], v)
				}
			}
		})
	}
}

// TestBlockExecutor_SkipRunOnce covers the agent's case: each job is one
// play on one host, so every agent in a batch would otherwise see itself
// as "the first host" and run the run_once block. Only the server-chosen
// batch leader runs without this flag.
func TestBlockExecutor_SkipRunOnce(t *testing.T) {
	var executedHosts []string
	modules.RegisterModule("_test_block_skip_runonce", func(ctx context.Context, opts modules.ExecOptions) (string, string, error) {
		executedHosts = append(executedHosts, opts.Host)
		return "ok", "", nil
	})

	exec, _ := setupBlockExecutor(t, []string{"host1"})
	exec.WithSkipRunOnce(true)

	blocks := []ansible.Block{
		{
			BlockBase: ansible.BlockBase{
				Base: ansible.Base{Name: "run once task", RunOnce: true},
			},
			Task: ansible.Task{
				UnknownField: map[string]any{
					"_test_block_skip_runonce": map[string]any{},
				},
			},
		},
		{
			BlockBase: ansible.BlockBase{
				Base: ansible.Base{Name: "ordinary task"},
			},
			Task: ansible.Task{
				UnknownField: map[string]any{
					"_test_block_skip_runonce": map[string]any{},
				},
			},
		},
	}

	taskIndex := 0
	exec.WithTaskProgress(&taskIndex, countBlocks(blocks))

	executedHosts = nil
	if err := exec.Exec(context.Background(), blocks); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	// The run_once block is dropped; the ordinary one still runs.
	if len(executedHosts) != 1 || executedHosts[0] != "host1" {
		t.Fatalf("expected only the ordinary task to run on host1, got %v", executedHosts)
	}
	// The skipped block still counts, or the job's progress would stop
	// short of its own total on every non-leader host.
	if taskIndex != countBlocks(blocks) {
		t.Fatalf("task counter = %d, want %d (the skipped block was not accounted for)",
			taskIndex, countBlocks(blocks))
	}
}
