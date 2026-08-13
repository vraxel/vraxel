package command

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"testing"

	"vraxel.io/vraxel/lib/ansible/modules/internal"
)

// mockConnector implements connector.Connector for testing.
type mockConnector struct {
	execFn     func(ctx context.Context, cmd string) ([]byte, []byte, error)
	execOutput []byte
	execErr    error
}

func (m *mockConnector) Init(context.Context) error  { return nil }
func (m *mockConnector) Close(context.Context) error { return nil }
func (m *mockConnector) ExecuteCommand(ctx context.Context, cmd string) ([]byte, []byte, error) {
	if m.execFn != nil {
		return m.execFn(ctx, cmd)
	}
	return m.execOutput, nil, m.execErr
}
func (m *mockConnector) PutFile(_ context.Context, _ []byte, _ string, _ fs.FileMode) error {
	return nil
}
func (m *mockConnector) FetchFile(_ context.Context, _ string, _ io.Writer) error {
	return nil
}
func (m *mockConnector) Resize(_ context.Context, _, _ uint16) error { return nil }

func TestModuleCommand_Simple(t *testing.T) {
	mock := &mockConnector{
		execFn: func(_ context.Context, cmd string) ([]byte, []byte, error) {
			if cmd != "echo hello" {
				t.Errorf("expected command 'echo hello', got %q", cmd)
			}
			return []byte("hello"), nil, nil
		},
	}

	stdout, stderr, err := ModuleCommand(context.Background(), internal.ExecOptions{
		Args:      map[string]any{"cmd": "echo hello"},
		Connector: mock,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stdout != "hello" {
		t.Errorf("expected stdout 'hello', got %q", stdout)
	}
	if stderr != "" {
		t.Errorf("expected empty stderr, got %q", stderr)
	}
}

func TestModuleCommand_CommandKey(t *testing.T) {
	mock := &mockConnector{
		execFn: func(_ context.Context, cmd string) ([]byte, []byte, error) {
			return []byte("output"), nil, nil
		},
	}

	stdout, _, err := ModuleCommand(context.Background(), internal.ExecOptions{
		Args:      map[string]any{"command": "ls -la"},
		Connector: mock,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stdout != "output" {
		t.Errorf("expected stdout 'output', got %q", stdout)
	}
}

func TestModuleCommand_ShellKey(t *testing.T) {
	mock := &mockConnector{
		execFn: func(_ context.Context, cmd string) ([]byte, []byte, error) {
			if cmd != "cat /etc/hosts" {
				t.Errorf("expected 'cat /etc/hosts', got %q", cmd)
			}
			return []byte("127.0.0.1 localhost"), nil, nil
		},
	}

	stdout, _, err := ModuleCommand(context.Background(), internal.ExecOptions{
		Args:      map[string]any{"shell": "cat /etc/hosts"},
		Connector: mock,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stdout != "127.0.0.1 localhost" {
		t.Errorf("unexpected stdout: %q", stdout)
	}
}

func TestModuleCommand_NoCommand(t *testing.T) {
	mock := &mockConnector{}

	_, _, err := ModuleCommand(context.Background(), internal.ExecOptions{
		Args:      map[string]any{},
		Connector: mock,
	})
	if err == nil {
		t.Fatal("expected error for empty args, got nil")
	}
	if err.Error() != "command module: no command specified" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestModuleCommand_NilArgs(t *testing.T) {
	mock := &mockConnector{}

	_, _, err := ModuleCommand(context.Background(), internal.ExecOptions{
		Args:      nil,
		Connector: mock,
	})
	if err == nil {
		t.Fatal("expected error for nil args, got nil")
	}
}

func TestModuleCommand_WithStderr(t *testing.T) {
	mock := &mockConnector{
		execFn: func(_ context.Context, cmd string) ([]byte, []byte, error) {
			return []byte("partial"), []byte("warning: something"), nil
		},
	}

	stdout, stderr, err := ModuleCommand(context.Background(), internal.ExecOptions{
		Args:      map[string]any{"cmd": "some-cmd"},
		Connector: mock,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stdout != "partial" {
		t.Errorf("expected stdout 'partial', got %q", stdout)
	}
	if stderr != "warning: something" {
		t.Errorf("expected stderr 'warning: something', got %q", stderr)
	}
}

func TestModuleCommand_ExecutionError(t *testing.T) {
	mock := &mockConnector{
		execFn: func(_ context.Context, cmd string) ([]byte, []byte, error) {
			return nil, []byte("command not found"), fmt.Errorf("exit status 127")
		},
	}

	_, stderr, err := ModuleCommand(context.Background(), internal.ExecOptions{
		Args:      map[string]any{"cmd": "nonexistent-cmd"},
		Connector: mock,
	})
	if err == nil {
		t.Fatal("expected error from command execution, got nil")
	}
	if stderr != "command not found" {
		t.Errorf("expected stderr 'command not found', got %q", stderr)
	}
}

func TestModuleCommand_KeyPriority(t *testing.T) {
	// When both "cmd" and "command" are present, "cmd" should win.
	mock := &mockConnector{
		execFn: func(_ context.Context, cmd string) ([]byte, []byte, error) {
			return []byte(cmd), nil, nil
		},
	}

	stdout, _, err := ModuleCommand(context.Background(), internal.ExecOptions{
		Args: map[string]any{
			"cmd":     "from-cmd",
			"command": "from-command",
		},
		Connector: mock,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stdout != "from-cmd" {
		t.Errorf("expected 'cmd' key to take priority, got %q", stdout)
	}
}

func TestModuleShell(t *testing.T) {
	mock := &mockConnector{
		execFn: func(_ context.Context, cmd string) ([]byte, []byte, error) {
			return []byte("shell-output"), nil, nil
		},
	}

	stdout, stderr, err := ModuleShell(context.Background(), internal.ExecOptions{
		Args:      map[string]any{"cmd": "echo test"},
		Connector: mock,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stdout != "shell-output" {
		t.Errorf("expected stdout 'shell-output', got %q", stdout)
	}
	if stderr != "" {
		t.Errorf("expected empty stderr, got %q", stderr)
	}
}

func TestModuleShell_NoCommand(t *testing.T) {
	mock := &mockConnector{}

	_, _, err := ModuleShell(context.Background(), internal.ExecOptions{
		Args:      map[string]any{},
		Connector: mock,
	})
	if err == nil {
		t.Fatal("expected error for empty args, got nil")
	}
}

func TestExtractCommand_EmptyString(t *testing.T) {
	result := ExtractCommand(map[string]any{"cmd": ""})
	if result != "" {
		t.Errorf("expected empty string for empty cmd value, got %q", result)
	}
}

func TestExtractCommand_NonStringValue(t *testing.T) {
	result := ExtractCommand(map[string]any{"cmd": 123})
	if result != "" {
		t.Errorf("expected empty string for non-string cmd value, got %q", result)
	}
}
