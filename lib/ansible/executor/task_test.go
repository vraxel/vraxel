package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"vraxel.io/vraxel/lib/ansible"
	"vraxel.io/vraxel/lib/ansible/connector"
	"vraxel.io/vraxel/lib/ansible/modules"
	"vraxel.io/vraxel/lib/ansible/variable"
)

// setupLocalConnector creates and initializes a local connector for testing.
func setupLocalConnector(t *testing.T) connector.Connector {
	t.Helper()
	conn := connector.NewLocalConnector("")
	if err := conn.Init(context.Background()); err != nil {
		t.Fatalf("failed to init local connector: %v", err)
	}
	return conn
}

// setupTestExecutor creates a TaskExecutor with a local connector on "localhost".
func setupTestExecutor(t *testing.T) (*TaskExecutor, variable.Variable) {
	t.Helper()
	conn := setupLocalConnector(t)
	inv := ansible.Inventory{
		Hosts: map[string]map[string]any{
			"localhost": {"connection": "local"},
		},
	}
	v := variable.New(inv)
	conns := newConnectorRegistry()
	conns.Put("localhost", conn)
	var logBuf bytes.Buffer
	executor := NewTaskExecutor(v, nil, conns, &logBuf)
	return executor, v
}

func TestTaskExecutor_SimpleCommand(t *testing.T) {
	executor, _ := setupTestExecutor(t)

	task := ansible.TaskSpec{
		Name:  "echo hello",
		Hosts: []string{"localhost"},
		Module: ansible.ModuleRef{
			Name: "command",
			Args: map[string]any{"cmd": "echo hello"},
		},
	}

	results := executor.Exec(context.Background(), task)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	r := results[0]
	if r.Host != "localhost" {
		t.Errorf("expected host localhost, got %s", r.Host)
	}
	if r.Status != ansible.TaskStatusOK {
		t.Errorf("expected status OK, got %s (error: %s)", r.Status, r.Error)
	}
	if r.Error != "" {
		t.Errorf("expected no error, got %s", r.Error)
	}

	stdout := extractString(r.Output, "stdout")
	if stdout != "hello" {
		t.Errorf("expected stdout 'hello', got %q", stdout)
	}
}

func TestTaskExecutor_WhenSkip(t *testing.T) {
	executor, v := setupTestExecutor(t)

	// Set a variable that will make the when condition false
	v.Merge(variable.MergeHostRuntimeVars("localhost", map[string]any{
		"should_run": false,
	}))

	task := ansible.TaskSpec{
		Name:  "conditional task",
		Hosts: []string{"localhost"},
		Module: ansible.ModuleRef{
			Name: "command",
			Args: map[string]any{"cmd": "echo executed"},
		},
		When: []string{"{{ .should_run }}"},
	}

	results := executor.Exec(context.Background(), task)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	r := results[0]
	if r.Status != ansible.TaskStatusSkipped {
		t.Errorf("expected status skipped, got %s", r.Status)
	}
}

func TestTaskExecutor_WhenExecute(t *testing.T) {
	executor, v := setupTestExecutor(t)

	// Set a variable that will make the when condition true
	v.Merge(variable.MergeHostRuntimeVars("localhost", map[string]any{
		"should_run": true,
	}))

	task := ansible.TaskSpec{
		Name:  "conditional task",
		Hosts: []string{"localhost"},
		Module: ansible.ModuleRef{
			Name: "command",
			Args: map[string]any{"cmd": "echo executed"},
		},
		When: []string{"{{ .should_run }}"},
	}

	results := executor.Exec(context.Background(), task)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	r := results[0]
	if r.Status != ansible.TaskStatusOK {
		t.Errorf("expected status OK, got %s (error: %s)", r.Status, r.Error)
	}

	stdout := extractString(r.Output, "stdout")
	if stdout != "executed" {
		t.Errorf("expected stdout 'executed', got %q", stdout)
	}
}

func TestTaskExecutor_Loop(t *testing.T) {
	executor, _ := setupTestExecutor(t)

	task := ansible.TaskSpec{
		Name:  "loop task",
		Hosts: []string{"localhost"},
		Module: ansible.ModuleRef{
			Name: "command",
			Args: map[string]any{"cmd": "echo {{ .item }}"},
		},
		Loop: []any{"a", "b", "c"},
	}

	results := executor.Exec(context.Background(), task)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	r := results[0]
	if r.Status != ansible.TaskStatusOK {
		t.Errorf("expected status OK, got %s (error: %s)", r.Status, r.Error)
	}
}

func TestTaskExecutor_Register(t *testing.T) {
	executor, v := setupTestExecutor(t)

	task := ansible.TaskSpec{
		Name:  "register task",
		Hosts: []string{"localhost"},
		Module: ansible.ModuleRef{
			Name: "command",
			Args: map[string]any{"cmd": "echo registration_test"},
		},
		Register: "my_result",
	}

	executor.Exec(context.Background(), task)

	// Check that the variable was registered
	rawVars := v.Get(variable.GetAllVariable("localhost"))
	vars, ok := rawVars.(map[string]any)
	if !ok {
		t.Fatal("expected vars to be map[string]any")
	}

	regVar, ok := vars["my_result"]
	if !ok {
		t.Fatal("expected registered variable 'my_result' to exist")
	}

	regMap, ok := regVar.(map[string]any)
	if !ok {
		t.Fatalf("expected registered variable to be map, got %T", regVar)
	}

	stdout, ok := regMap["stdout"]
	if !ok {
		t.Fatal("expected 'stdout' in registered variable")
	}

	if s, ok := stdout.(string); ok {
		if s != "registration_test" {
			t.Errorf("expected stdout 'registration_test', got %q", s)
		}
	} else {
		t.Errorf("expected stdout to be string, got %T", stdout)
	}

	failed, ok := regMap["failed"]
	if !ok {
		t.Fatal("expected 'failed' in registered variable")
	}
	if failed != false {
		t.Errorf("expected failed=false, got %v", failed)
	}
}

func TestTaskExecutor_RegisterJSON(t *testing.T) {
	executor, v := setupTestExecutor(t)

	task := ansible.TaskSpec{
		Name:  "register json task",
		Hosts: []string{"localhost"},
		Module: ansible.ModuleRef{
			Name: "command",
			Args: map[string]any{"cmd": `echo '{"name":"vraxel","count":3,"big":10000000000000001}'`},
		},
		Register:     "j",
		RegisterType: "json",
	}

	executor.Exec(context.Background(), task)

	vars := v.Get(variable.GetAllVariable("localhost")).(map[string]any)
	stdout := vars["j"].(map[string]any)["stdout"]
	m, ok := stdout.(map[string]any)
	if !ok {
		t.Fatalf("expected stdout parsed into map, got %T (%v)", stdout, stdout)
	}
	if m["name"] != "vraxel" {
		t.Errorf("name = %v, want vraxel", m["name"])
	}
	// count must be numeric, not string.
	if m["count"] != int64(3) {
		t.Errorf("count = %v (%T), want int64(3)", m["count"], m["count"])
	}
	// large integer must keep precision as int64, not lose it to float64.
	if m["big"] != int64(10000000000000001) {
		t.Errorf("big = %v (%T), want int64(10000000000000001)", m["big"], m["big"])
	}
}

func TestParseRegisterStdout(t *testing.T) {
	// default: raw string preserved.
	if got := parseRegisterStdout("", "plain\n"); got != "plain\n" {
		t.Errorf("default: got %q, want %q", got, "plain\n")
	}
	// invalid json falls back to the raw string.
	if got := parseRegisterStdout("json", "not json"); got != "not json" {
		t.Errorf("json fallback: got %v, want raw string", got)
	}
	// yaml parses into a map.
	got := parseRegisterStdout("yaml", "a: 1\nb: two")
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("yaml: got %T, want map", got)
	}
	if m["a"] != 1 || m["b"] != "two" {
		t.Errorf("yaml: got %v", m)
	}
}

func TestNormalizeJSONNumbers(t *testing.T) {
	in := map[string]any{
		"i":   json.Number("42"),
		"f":   json.Number("3.14"),
		"arr": []any{json.Number("1"), json.Number("2")},
	}
	out := normalizeJSONNumbers(in).(map[string]any)
	if out["i"] != int64(42) {
		t.Errorf("i = %v (%T), want int64(42)", out["i"], out["i"])
	}
	if out["f"] != 3.14 {
		t.Errorf("f = %v (%T), want float64(3.14)", out["f"], out["f"])
	}
	arr := out["arr"].([]any)
	if arr[0] != int64(1) || arr[1] != int64(2) {
		t.Errorf("arr = %v", arr)
	}
}

func TestTaskExecutor_IgnoreErrors(t *testing.T) {
	executor, _ := setupTestExecutor(t)

	ignoreErrors := true
	task := ansible.TaskSpec{
		Name:  "failing task",
		Hosts: []string{"localhost"},
		Module: ansible.ModuleRef{
			Name: "command",
			Args: map[string]any{"cmd": "exit 1"},
		},
		IgnoreErrors: &ignoreErrors,
	}

	results := executor.Exec(context.Background(), task)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	r := results[0]
	// When ignore_errors is true, status should not be "failed"
	if r.Status == ansible.TaskStatusFailed {
		t.Errorf("expected status to not be failed when ignore_errors=true, got %s", r.Status)
	}
}

func TestTaskExecutor_FailedWhen(t *testing.T) {
	// Register a test module that returns a specific stdout
	modules.RegisterModule("_test_fail_when", func(ctx context.Context, opts modules.ExecOptions) (string, string, error) {
		return "error_code_42", "", nil
	})

	executor, v := setupTestExecutor(t)

	// Set up variables so the failed_when condition can be evaluated
	v.Merge(variable.MergeHostRuntimeVars("localhost", map[string]any{
		"expected_error": true,
	}))

	task := ansible.TaskSpec{
		Name:  "failed_when task",
		Hosts: []string{"localhost"},
		Module: ansible.ModuleRef{
			Name: "_test_fail_when",
			Args: map[string]any{},
		},
		FailedWhen: []string{"{{ .expected_error }}"},
	}

	results := executor.Exec(context.Background(), task)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	r := results[0]
	if r.Status != ansible.TaskStatusFailed {
		t.Errorf("expected status failed, got %s", r.Status)
	}
	if r.Error != "failed_when condition met" {
		t.Errorf("expected error 'failed_when condition met', got %q", r.Error)
	}
}

func TestTaskExecutor_ModuleNotFound(t *testing.T) {
	executor, _ := setupTestExecutor(t)

	task := ansible.TaskSpec{
		Name:  "unknown module",
		Hosts: []string{"localhost"},
		Module: ansible.ModuleRef{
			Name: "nonexistent_module_xyz",
			Args: map[string]any{},
		},
	}

	results := executor.Exec(context.Background(), task)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	r := results[0]
	if r.Status != ansible.TaskStatusFailed {
		t.Errorf("expected status failed, got %s", r.Status)
	}
	if !strings.Contains(r.Error, "not found") {
		t.Errorf("expected error to contain 'not found', got %q", r.Error)
	}
}

func TestTaskExecutor_RenderArgs(t *testing.T) {
	executor, v := setupTestExecutor(t)

	v.Merge(variable.MergeHostRuntimeVars("localhost", map[string]any{
		"greeting": "world",
	}))

	task := ansible.TaskSpec{
		Name:  "render args",
		Hosts: []string{"localhost"},
		Module: ansible.ModuleRef{
			Name: "command",
			Args: map[string]any{"cmd": "echo {{ .greeting }}"},
		},
	}

	results := executor.Exec(context.Background(), task)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	r := results[0]
	if r.Status != ansible.TaskStatusOK {
		t.Errorf("expected status OK, got %s (error: %s)", r.Status, r.Error)
	}

	stdout := extractString(r.Output, "stdout")
	if stdout != "world" {
		t.Errorf("expected stdout 'world', got %q", stdout)
	}
}

func TestTaskExecutor_MultiHost(t *testing.T) {
	conn := setupLocalConnector(t)
	inv := ansible.Inventory{
		Hosts: map[string]map[string]any{
			"host1": {"connection": "local"},
			"host2": {"connection": "local"},
			"host3": {"connection": "local"},
		},
	}
	v := variable.New(inv)
	conns := newConnectorRegistry()
	conns.Put("host1", conn)
	conns.Put("host2", conn)
	conns.Put("host3", conn)
	var logBuf bytes.Buffer
	executor := NewTaskExecutor(v, nil, conns, &logBuf)

	task := ansible.TaskSpec{
		Name:  "multi-host task",
		Hosts: []string{"host1", "host2", "host3"},
		Module: ansible.ModuleRef{
			Name: "command",
			Args: map[string]any{"cmd": "echo multi"},
		},
	}

	results := executor.Exec(context.Background(), task)

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// Sort results by host for deterministic checks
	sort.Slice(results, func(i, j int) bool {
		return results[i].Host < results[j].Host
	})

	hosts := []string{"host1", "host2", "host3"}
	for i, r := range results {
		if r.Host != hosts[i] {
			t.Errorf("result[%d]: expected host %s, got %s", i, hosts[i], r.Host)
		}
		if r.Status != ansible.TaskStatusOK {
			t.Errorf("result[%d]: expected status OK, got %s (error: %s)", i, r.Status, r.Error)
		}
		stdout := extractString(r.Output, "stdout")
		if stdout != "multi" {
			t.Errorf("result[%d]: expected stdout 'multi', got %q", i, stdout)
		}
	}
}

func TestTaskExecutor_RetryWithUntil(t *testing.T) {
	// Register a module that succeeds on the third call
	callCount := 0
	modules.RegisterModule("_test_retry", func(ctx context.Context, opts modules.ExecOptions) (string, string, error) {
		callCount++
		if callCount >= 3 {
			return "true", "", nil
		}
		return "false", "", nil
	})

	executor, _ := setupTestExecutor(t)

	task := ansible.TaskSpec{
		Name:  "retry task",
		Hosts: []string{"localhost"},
		Module: ansible.ModuleRef{
			Name: "_test_retry",
			Args: map[string]any{},
		},
		Retries: 5,
		Delay:   0, // no delay in tests
		Until:   []string{"{{ eq .stdout \"true\" }}"},
	}

	// Use a short delay to speed up tests
	task.Delay = 0

	start := time.Now()
	results := executor.Exec(context.Background(), task)
	elapsed := time.Since(start)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	r := results[0]
	if r.Status != ansible.TaskStatusOK {
		t.Errorf("expected status OK, got %s (error: %s)", r.Status, r.Error)
	}

	if callCount != 3 {
		t.Errorf("expected 3 calls, got %d", callCount)
	}

	// With delay=0, this should complete quickly
	if elapsed > 5*time.Second {
		t.Errorf("retry took too long: %v", elapsed)
	}
}

func TestTaskExecutor_ContextCancellation(t *testing.T) {
	executor, _ := setupTestExecutor(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	task := ansible.TaskSpec{
		Name:  "cancelled task",
		Hosts: []string{"localhost"},
		Module: ansible.ModuleRef{
			Name: "command",
			Args: map[string]any{"cmd": "echo hello"},
		},
	}

	results := executor.Exec(ctx, task)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	// The task may fail or succeed depending on timing, but it should not hang
}

func TestTaskExecutor_NoHosts(t *testing.T) {
	executor, _ := setupTestExecutor(t)

	task := ansible.TaskSpec{
		Name:  "no hosts",
		Hosts: []string{},
		Module: ansible.ModuleRef{
			Name: "command",
			Args: map[string]any{"cmd": "echo hello"},
		},
	}

	results := executor.Exec(context.Background(), task)
	if len(results) != 0 {
		t.Errorf("expected 0 results for empty hosts, got %d", len(results))
	}
}

func TestTaskExecutor_FailWithoutIgnoreErrors(t *testing.T) {
	executor, _ := setupTestExecutor(t)

	task := ansible.TaskSpec{
		Name:  "failing task",
		Hosts: []string{"localhost"},
		Module: ansible.ModuleRef{
			Name: "command",
			Args: map[string]any{"cmd": "exit 1"},
		},
	}

	results := executor.Exec(context.Background(), task)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	r := results[0]
	if r.Status != ansible.TaskStatusFailed {
		t.Errorf("expected status failed, got %s", r.Status)
	}
	if r.Error == "" {
		t.Error("expected error message, got empty")
	}
}

func TestTaskExecutor_ResolveLoop_NilLoop(t *testing.T) {
	executor, _ := setupTestExecutor(t)

	task := ansible.TaskSpec{Loop: nil}
	items := executor.resolveLoop(task)
	if len(items) != 1 {
		t.Fatalf("expected 1 item for nil loop, got %d", len(items))
	}
	if items[0] != nil {
		t.Errorf("expected nil item, got %v", items[0])
	}
}

func TestTaskExecutor_ResolveLoop_SliceAny(t *testing.T) {
	executor, _ := setupTestExecutor(t)

	task := ansible.TaskSpec{Loop: []any{"x", "y", "z"}}
	items := executor.resolveLoop(task)
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
	if items[0] != "x" || items[1] != "y" || items[2] != "z" {
		t.Errorf("unexpected items: %v", items)
	}
}

func TestTaskExecutor_ResolveLoop_SliceString(t *testing.T) {
	executor, _ := setupTestExecutor(t)

	task := ansible.TaskSpec{Loop: []string{"a", "b"}}
	items := executor.resolveLoop(task)
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0] != "a" || items[1] != "b" {
		t.Errorf("unexpected items: %v", items)
	}
}

func TestTaskExecutor_RenderArgs_Unit(t *testing.T) {
	executor, _ := setupTestExecutor(t)

	vars := map[string]any{
		"name": "world",
		"port": 8080,
	}

	args := map[string]any{
		"msg":     "hello {{ .name }}",
		"count":   42,
		"nested":  map[string]any{"key": "val-{{ .name }}"},
		"literal": "no-template-here",
	}

	rendered := executor.renderArgs(args, vars)

	if rendered["msg"] != "hello world" {
		t.Errorf("expected 'hello world', got %q", rendered["msg"])
	}
	if rendered["count"] != 42 {
		t.Errorf("expected 42, got %v", rendered["count"])
	}
	if rendered["literal"] != "no-template-here" {
		t.Errorf("expected 'no-template-here', got %q", rendered["literal"])
	}

	nested, ok := rendered["nested"].(map[string]any)
	if !ok {
		t.Fatalf("expected nested to be map, got %T", rendered["nested"])
	}
	if nested["key"] != "val-world" {
		t.Errorf("expected 'val-world', got %q", nested["key"])
	}
}

func TestTaskExecutor_ToArgsMap(t *testing.T) {
	executor, _ := setupTestExecutor(t)

	// nil args
	m := executor.toArgsMap(nil)
	if len(m) != 0 {
		t.Errorf("expected empty map for nil args, got %v", m)
	}

	// map[string]any args
	m = executor.toArgsMap(map[string]any{"key": "val"})
	if m["key"] != "val" {
		t.Errorf("expected key=val, got %v", m)
	}

	// string args (fallback)
	m = executor.toArgsMap("some-string")
	if m["raw"] != "some-string" {
		t.Errorf("expected raw='some-string', got %v", m)
	}
}

func TestTaskExecutor_LoopWithFailure(t *testing.T) {
	// Register a module that fails on the second item
	callIdx := 0
	modules.RegisterModule("_test_loop_fail", func(ctx context.Context, opts modules.ExecOptions) (string, string, error) {
		callIdx++
		if callIdx == 2 {
			return "", "", fmt.Errorf("intentional failure on item 2")
		}
		return "ok", "", nil
	})

	executor, _ := setupTestExecutor(t)

	task := ansible.TaskSpec{
		Name:  "loop with failure",
		Hosts: []string{"localhost"},
		Module: ansible.ModuleRef{
			Name: "_test_loop_fail",
			Args: map[string]any{},
		},
		Loop: []any{"a", "b", "c"},
	}

	callIdx = 0 // reset
	results := executor.Exec(context.Background(), task)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	r := results[0]
	if r.Status != ansible.TaskStatusFailed {
		t.Errorf("expected status failed, got %s", r.Status)
	}
	// Should have stopped at item 2 (b), so only 2 calls
	if callIdx != 2 {
		t.Errorf("expected 2 calls (stop on failure), got %d", callIdx)
	}
}

func TestTaskExecutor_LoopWithIgnoreErrors(t *testing.T) {
	// Register a module that always fails
	modules.RegisterModule("_test_loop_ignore", func(ctx context.Context, opts modules.ExecOptions) (string, string, error) {
		return "", "", fmt.Errorf("always fails")
	})

	executor, _ := setupTestExecutor(t)

	ignoreErrors := true
	task := ansible.TaskSpec{
		Name:  "loop ignore errors",
		Hosts: []string{"localhost"},
		Module: ansible.ModuleRef{
			Name: "_test_loop_ignore",
			Args: map[string]any{},
		},
		Loop:         []any{"a", "b", "c"},
		IgnoreErrors: &ignoreErrors,
	}

	results := executor.Exec(context.Background(), task)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	r := results[0]
	// With ignore_errors, status should not be "failed"
	if r.Status == ansible.TaskStatusFailed {
		t.Errorf("expected status not to be failed with ignore_errors, got %s", r.Status)
	}
}
