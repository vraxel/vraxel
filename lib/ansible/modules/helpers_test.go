package modules

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"testing"

	"vraxel.io/vraxel/lib/ansible"
	"vraxel.io/vraxel/lib/ansible/variable"
)

// mockConnector implements connector.Connector for testing across all module tests.
type mockConnector struct {
	// ExecuteCommand
	execFn     func(ctx context.Context, cmd string) ([]byte, []byte, error)
	execOutput []byte
	execErr    error

	// PutFile
	putFileData  []byte
	putFileDest  string
	putFileMode  fs.FileMode
	putFileCalls []putFileCall
	putFileErr   error

	// FetchFile
	fetchFileContent []byte
	fetchFileErr     error
}

type putFileCall struct {
	src  []byte
	dst  string
	mode fs.FileMode
}

func (m *mockConnector) Init(context.Context) error  { return nil }
func (m *mockConnector) Close(context.Context) error { return nil }
func (m *mockConnector) ExecuteCommand(ctx context.Context, cmd string) ([]byte, []byte, error) {
	if m.execFn != nil {
		return m.execFn(ctx, cmd)
	}
	return m.execOutput, nil, m.execErr
}
func (m *mockConnector) PutFile(_ context.Context, src []byte, dst string, mode fs.FileMode) error {
	m.putFileData = src
	m.putFileDest = dst
	m.putFileMode = mode
	m.putFileCalls = append(m.putFileCalls, putFileCall{src: src, dst: dst, mode: mode})
	return m.putFileErr
}
func (m *mockConnector) FetchFile(_ context.Context, _ string, dst io.Writer) error {
	if m.fetchFileErr != nil {
		return m.fetchFileErr
	}
	if m.fetchFileContent != nil {
		_, err := dst.Write(m.fetchFileContent)
		return err
	}
	return nil
}
func (m *mockConnector) Resize(_ context.Context, _, _ uint16) error { return nil }

// mockGatherFactsConnector implements both connector.Connector and connector.GatherFacts.
type mockGatherFactsConnector struct {
	mockConnector
	hostInfoFn func(ctx context.Context) (map[string]any, error)
}

func (m *mockGatherFactsConnector) HostInfo(ctx context.Context) (map[string]any, error) {
	if m.hostInfoFn != nil {
		return m.hostInfoFn(ctx)
	}
	return nil, nil
}

// mockSource provides in-memory file content for Source.ReadFile.
type mockSource struct {
	files map[string][]byte
}

func (s *mockSource) ReadFile(path string) ([]byte, error) {
	data, ok := s.files[path]
	if !ok {
		return nil, fmt.Errorf("file not found: %s", path)
	}
	return data, nil
}

// newTestVariable creates a Variable with the given hosts pre-populated.
func newTestVariable(hosts ...string) variable.Variable {
	hostMap := make(map[string]map[string]any, len(hosts))
	for _, h := range hosts {
		hostMap[h] = make(map[string]any)
	}
	return variable.New(ansible.Inventory{
		Hosts: hostMap,
	})
}

// newTestVariableWithVars creates a Variable for a single host with pre-populated host vars.
func newTestVariableWithVars(host string, hostVars map[string]any) variable.Variable {
	return variable.New(ansible.Inventory{
		Hosts: map[string]map[string]any{
			host: hostVars,
		},
	})
}

// getHostRuntimeVars reads a host's RuntimeVars from the Variable.
func getHostRuntimeVars(v variable.Variable, host string) map[string]any {
	result := v.Get(func(val *variable.Value) any {
		hv, ok := val.Hosts[host]
		if !ok {
			return map[string]any{}
		}
		return hv.RuntimeVars
	})
	if m, ok := result.(map[string]any); ok {
		return m
	}
	return map[string]any{}
}

// getHostRemoteVars reads a host's RemoteVars from the Variable.
func getHostRemoteVars(v variable.Variable, host string) map[string]any {
	result := v.Get(func(val *variable.Value) any {
		hv, ok := val.Hosts[host]
		if !ok {
			return map[string]any{}
		}
		return hv.RemoteVars
	})
	if m, ok := result.(map[string]any); ok {
		return m
	}
	return map[string]any{}
}

// --- helper function tests ---

func TestStringArg(t *testing.T) {
	args := map[string]any{
		"name":  "hello",
		"count": 42,
		"empty": "",
	}

	if got := StringArg(args, "name"); got != "hello" {
		t.Errorf("expected 'hello', got %q", got)
	}
	if got := StringArg(args, "count"); got != "" {
		t.Errorf("expected empty string for non-string value, got %q", got)
	}
	if got := StringArg(args, "missing"); got != "" {
		t.Errorf("expected empty string for missing key, got %q", got)
	}
	if got := StringArg(args, "empty"); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestFileModeArg(t *testing.T) {
	tests := []struct {
		name     string
		args     map[string]any
		key      string
		def      fs.FileMode
		expected fs.FileMode
	}{
		{"missing key", map[string]any{}, "mode", 0644, 0644},
		{"int value", map[string]any{"mode": 0755}, "mode", 0644, 0755},
		{"int64 value", map[string]any{"mode": int64(0600)}, "mode", 0644, 0600},
		{"float64 value", map[string]any{"mode": float64(0644)}, "mode", 0755, 0644},
		{"string octal", map[string]any{"mode": "0755"}, "mode", 0644, 0755},
		{"string octal 644", map[string]any{"mode": "644"}, "mode", 0755, 0644},
		{"invalid string", map[string]any{"mode": "notanumber"}, "mode", 0644, 0644},
		{"unsupported type", map[string]any{"mode": true}, "mode", 0644, 0644},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FileModeArg(tt.args, tt.key, tt.def)
			if got != tt.expected {
				t.Errorf("expected %04o, got %04o", tt.expected, got)
			}
		})
	}
}

func TestReadSource_Relative(t *testing.T) {
	source := &mockSource{
		files: map[string][]byte{
			"templates/config.yaml": []byte("key: val"),
		},
	}

	data, err := ReadSource(source, "templates/config.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "key: val" {
		t.Errorf("expected %q, got %q", "key: val", string(data))
	}
}

func TestReadSource_NilSource(t *testing.T) {
	_, err := ReadSource(nil, "relative/path.txt")
	if err == nil {
		t.Fatal("expected error with nil source")
	}
}
