package result

import (
	"context"
	"io"
	"io/fs"
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

func TestModuleResult(t *testing.T) {
	v := newTestVariable("testhost")

	stdout, stderr, err := ModuleResult(context.Background(), internal.ExecOptions{
		Host:     "testhost",
		Variable: v,
		Args: map[string]any{
			"app_version": "1.0.0",
			"db_port":     5432,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stdout != "result stored" {
		t.Errorf("expected stdout 'result stored', got %q", stdout)
	}
	if stderr != "" {
		t.Errorf("expected empty stderr, got %q", stderr)
	}

	// Verify the result was stored.
	result := v.Get(variable.GetResultVariable())
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected result to be map[string]any, got %T", result)
	}
	if m["app_version"] != "1.0.0" {
		t.Errorf("expected app_version='1.0.0', got %v", m["app_version"])
	}
	if m["db_port"] != 5432 {
		t.Errorf("expected db_port=5432, got %v", m["db_port"])
	}
}

func TestModuleResult_MergeMultiple(t *testing.T) {
	v := newTestVariable("testhost")

	// First call: set some keys.
	_, _, err := ModuleResult(context.Background(), internal.ExecOptions{
		Host:     "testhost",
		Variable: v,
		Args:     map[string]any{"key1": "value1"},
	})
	if err != nil {
		t.Fatalf("first call: unexpected error: %v", err)
	}

	// Second call: set additional keys.
	_, _, err = ModuleResult(context.Background(), internal.ExecOptions{
		Host:     "testhost",
		Variable: v,
		Args:     map[string]any{"key2": "value2"},
	})
	if err != nil {
		t.Fatalf("second call: unexpected error: %v", err)
	}

	// Both keys should be present.
	result := v.Get(variable.GetResultVariable())
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected result to be map[string]any, got %T", result)
	}
	if m["key1"] != "value1" {
		t.Errorf("expected key1='value1', got %v", m["key1"])
	}
	if m["key2"] != "value2" {
		t.Errorf("expected key2='value2', got %v", m["key2"])
	}
}

func TestModuleResult_Empty(t *testing.T) {
	v := newTestVariable("testhost")

	// nil args
	_, _, err := ModuleResult(context.Background(), internal.ExecOptions{
		Host:     "testhost",
		Variable: v,
		Args:     nil,
	})
	if err == nil {
		t.Fatal("expected error for nil args, got nil")
	}

	// empty args
	_, _, err = ModuleResult(context.Background(), internal.ExecOptions{
		Host:     "testhost",
		Variable: v,
		Args:     map[string]any{},
	})
	if err == nil {
		t.Fatal("expected error for empty args, got nil")
	}
}
