package debug

import (
	"bytes"
	"context"
	"io"
	"io/fs"
	"strings"
	"testing"

	"vraxel.io/vraxel/lib/ansible"
	"vraxel.io/vraxel/lib/ansible/modules/internal"
	"vraxel.io/vraxel/lib/ansible/variable"
)

// mockConnector implements connector.Connector for testing.
type mockConnector struct{}

func (m *mockConnector) Init(context.Context) error  { return nil }
func (m *mockConnector) Close(context.Context) error { return nil }
func (m *mockConnector) ExecuteCommand(_ context.Context, _ string) ([]byte, []byte, error) {
	return nil, nil, nil
}
func (m *mockConnector) PutFile(_ context.Context, _ []byte, _ string, _ fs.FileMode) error {
	return nil
}
func (m *mockConnector) FetchFile(_ context.Context, _ string, _ io.Writer) error { return nil }

func newTestVariable(hosts ...string) variable.Variable {
	hostMap := make(map[string]map[string]any, len(hosts))
	for _, h := range hosts {
		hostMap[h] = make(map[string]any)
	}
	return variable.New(ansible.Inventory{
		Hosts: hostMap,
	})
}

func newTestVariableWithVars(host string, hostVars map[string]any) variable.Variable {
	return variable.New(ansible.Inventory{
		Hosts: map[string]map[string]any{
			host: hostVars,
		},
	})
}

func TestModuleDebug_SimpleMsg(t *testing.T) {
	opts := internal.ExecOptions{
		Args:     map[string]any{"msg": "hello world"},
		Host:     "testhost",
		Variable: newTestVariable("testhost"),
	}

	stdout, stderr, err := ModuleDebug(context.Background(), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stdout != "hello world" {
		t.Errorf("expected stdout 'hello world', got %q", stdout)
	}
	if stderr != "" {
		t.Errorf("expected empty stderr, got %q", stderr)
	}
}

func TestModuleDebug_TemplateMsg(t *testing.T) {
	opts := internal.ExecOptions{
		Args:     map[string]any{"msg": "{{ .name }}"},
		Host:     "testhost",
		Variable: newTestVariableWithVars("testhost", map[string]any{"name": "test"}),
	}

	stdout, _, err := ModuleDebug(context.Background(), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stdout != "test" {
		t.Errorf("expected stdout 'test', got %q", stdout)
	}
}

func TestModuleDebug_MapMsg(t *testing.T) {
	msgMap := map[string]any{
		"key1": "value1",
		"key2": float64(42),
	}
	opts := internal.ExecOptions{
		Args:     map[string]any{"msg": msgMap},
		Host:     "testhost",
		Variable: newTestVariable("testhost"),
	}

	stdout, _, err := ModuleDebug(context.Background(), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "key1") || !strings.Contains(stdout, "value1") {
		t.Errorf("expected JSON output containing key1/value1, got %q", stdout)
	}
	if !strings.Contains(stdout, "key2") || !strings.Contains(stdout, "42") {
		t.Errorf("expected JSON output containing key2/42, got %q", stdout)
	}
}

func TestModuleDebug_LogOutput(t *testing.T) {
	var buf bytes.Buffer
	opts := internal.ExecOptions{
		Args:      map[string]any{"msg": "log this"},
		Host:      "testhost",
		Variable:  newTestVariable("testhost"),
		LogOutput: &buf,
	}

	stdout, _, err := ModuleDebug(context.Background(), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stdout != "log this" {
		t.Errorf("expected stdout 'log this', got %q", stdout)
	}
	if !strings.Contains(buf.String(), "DEBUG: log this") {
		t.Errorf("expected log output to contain 'DEBUG: log this', got %q", buf.String())
	}
}

func TestModuleDebug_NoMsg(t *testing.T) {
	opts := internal.ExecOptions{
		Args:     map[string]any{},
		Host:     "testhost",
		Variable: newTestVariable("testhost"),
	}

	_, _, err := ModuleDebug(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error for missing msg argument, got nil")
	}
	if !strings.Contains(err.Error(), "msg argument required") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestModuleDebug_NumberMsg(t *testing.T) {
	opts := internal.ExecOptions{
		Args:     map[string]any{"msg": 42},
		Host:     "testhost",
		Variable: newTestVariable("testhost"),
	}

	stdout, _, err := ModuleDebug(context.Background(), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stdout != "42" {
		t.Errorf("expected stdout '42', got %q", stdout)
	}
}

func TestModuleDebug_SliceMsg(t *testing.T) {
	opts := internal.ExecOptions{
		Args:     map[string]any{"msg": []string{"a", "b", "c"}},
		Host:     "testhost",
		Variable: newTestVariable("testhost"),
	}

	stdout, _, err := ModuleDebug(context.Background(), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "a") || !strings.Contains(stdout, "b") || !strings.Contains(stdout, "c") {
		t.Errorf("expected JSON array output, got %q", stdout)
	}
}
