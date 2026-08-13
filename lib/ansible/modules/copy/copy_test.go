package copy

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"strings"
	"testing"

	"vraxel.io/vraxel/lib/ansible/modules/internal"
)

// mockConnector implements connector.Connector for testing.
type mockConnector struct {
	execFn       func(ctx context.Context, cmd string) ([]byte, []byte, error)
	putFileData  []byte
	putFileDest  string
	putFileMode  fs.FileMode
	putFileErr   error
	fetchContent []byte
	fetchErr     error
}

func (m *mockConnector) Init(context.Context) error  { return nil }
func (m *mockConnector) Close(context.Context) error { return nil }
func (m *mockConnector) ExecuteCommand(ctx context.Context, cmd string) ([]byte, []byte, error) {
	if m.execFn != nil {
		return m.execFn(ctx, cmd)
	}
	return nil, nil, nil
}
func (m *mockConnector) PutFile(_ context.Context, src []byte, dst string, mode fs.FileMode) error {
	m.putFileData = src
	m.putFileDest = dst
	m.putFileMode = mode
	return m.putFileErr
}
func (m *mockConnector) FetchFile(_ context.Context, _ string, dst io.Writer) error {
	if m.fetchErr != nil {
		return m.fetchErr
	}
	if m.fetchContent != nil {
		_, err := dst.Write(m.fetchContent)
		return err
	}
	return nil
}
func (m *mockConnector) Resize(_ context.Context, _, _ uint16) error { return nil }

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

func TestModuleCopy_Content(t *testing.T) {
	conn := &mockConnector{}
	opts := internal.ExecOptions{
		Args: map[string]any{
			"content": "hello world",
			"dest":    "/tmp/hello.txt",
		},
		Connector: conn,
	}

	stdout, stderr, err := ModuleCopy(context.Background(), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stderr != "" {
		t.Errorf("expected empty stderr, got %q", stderr)
	}
	if !strings.Contains(stdout, "/tmp/hello.txt") {
		t.Errorf("expected stdout to mention dest, got %q", stdout)
	}
	if string(conn.putFileData) != "hello world" {
		t.Errorf("expected PutFile data %q, got %q", "hello world", string(conn.putFileData))
	}
	if conn.putFileDest != "/tmp/hello.txt" {
		t.Errorf("expected PutFile dest %q, got %q", "/tmp/hello.txt", conn.putFileDest)
	}
	if conn.putFileMode != 0644 {
		t.Errorf("expected default mode 0644, got %04o", conn.putFileMode)
	}
}

func TestModuleCopy_ContentWithMode(t *testing.T) {
	conn := &mockConnector{}
	opts := internal.ExecOptions{
		Args: map[string]any{
			"content": "#!/bin/bash\necho hi",
			"dest":    "/usr/local/bin/greet.sh",
			"mode":    "0755",
		},
		Connector: conn,
	}

	_, _, err := ModuleCopy(context.Background(), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conn.putFileMode != 0755 {
		t.Errorf("expected mode 0755, got %04o", conn.putFileMode)
	}
}

func TestModuleCopy_ContentWithIntMode(t *testing.T) {
	conn := &mockConnector{}
	opts := internal.ExecOptions{
		Args: map[string]any{
			"content": "data",
			"dest":    "/tmp/f",
			"mode":    0600,
		},
		Connector: conn,
	}

	_, _, err := ModuleCopy(context.Background(), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conn.putFileMode != 0600 {
		t.Errorf("expected mode 0600, got %04o", conn.putFileMode)
	}
}

func TestModuleCopy_Src(t *testing.T) {
	conn := &mockConnector{}
	source := &mockSource{
		files: map[string][]byte{
			"configs/app.yaml": []byte("key: value"),
		},
	}
	opts := internal.ExecOptions{
		Args: map[string]any{
			"src":  "configs/app.yaml",
			"dest": "/etc/app/config.yaml",
		},
		Connector: conn,
		Source:    source,
	}

	stdout, stderr, err := ModuleCopy(context.Background(), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stderr != "" {
		t.Errorf("expected empty stderr, got %q", stderr)
	}
	if !strings.Contains(stdout, "/etc/app/config.yaml") {
		t.Errorf("expected stdout to mention dest, got %q", stdout)
	}
	if string(conn.putFileData) != "key: value" {
		t.Errorf("expected PutFile data %q, got %q", "key: value", string(conn.putFileData))
	}
}

func TestModuleCopy_SrcNotFound(t *testing.T) {
	conn := &mockConnector{}
	source := &mockSource{files: map[string][]byte{}}
	opts := internal.ExecOptions{
		Args: map[string]any{
			"src":  "missing.txt",
			"dest": "/tmp/missing.txt",
		},
		Connector: conn,
		Source:    source,
	}

	_, _, err := ModuleCopy(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error for missing source file")
	}
	if !strings.Contains(err.Error(), "read source") {
		t.Errorf("expected error to mention 'read source', got %q", err.Error())
	}
}

func TestModuleCopy_NoDest(t *testing.T) {
	opts := internal.ExecOptions{
		Args: map[string]any{
			"content": "test",
		},
	}

	_, _, err := ModuleCopy(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error when dest is missing")
	}
	if !strings.Contains(err.Error(), "dest is required") {
		t.Errorf("expected 'dest is required' error, got %q", err.Error())
	}
}

func TestModuleCopy_NoSrcOrContent(t *testing.T) {
	conn := &mockConnector{}
	opts := internal.ExecOptions{
		Args: map[string]any{
			"dest": "/tmp/test.txt",
		},
		Connector: conn,
	}

	_, _, err := ModuleCopy(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error when neither src nor content is provided")
	}
	if !strings.Contains(err.Error(), "either src or content") {
		t.Errorf("expected 'either src or content' error, got %q", err.Error())
	}
}

func TestModuleCopy_PutFileError(t *testing.T) {
	conn := &mockConnector{putFileErr: fmt.Errorf("connection refused")}
	opts := internal.ExecOptions{
		Args: map[string]any{
			"content": "data",
			"dest":    "/tmp/f.txt",
		},
		Connector: conn,
	}

	_, _, err := ModuleCopy(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error from PutFile failure")
	}
	if !strings.Contains(err.Error(), "put file") {
		t.Errorf("expected 'put file' error, got %q", err.Error())
	}
}

func TestModuleCopy_NoSource(t *testing.T) {
	conn := &mockConnector{}
	opts := internal.ExecOptions{
		Args: map[string]any{
			"src":  "relative/path.txt",
			"dest": "/tmp/path.txt",
		},
		Connector: conn,
		Source:    nil,
	}

	_, _, err := ModuleCopy(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error when source is nil for relative path")
	}
}
