package connector

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalConnector_Init(t *testing.T) {
	c := NewLocalConnector("")

	// Save and restore SHELL env var.
	origShell := os.Getenv("SHELL")
	defer func() {
		if origShell != "" {
			os.Setenv("SHELL", origShell)
		} else {
			os.Unsetenv("SHELL")
		}
	}()

	// Test with SHELL set.
	os.Setenv("SHELL", "/bin/zsh")
	if err := c.Init(context.Background()); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if c.shell != "/bin/zsh" {
		t.Errorf("Init() shell = %q, want %q", c.shell, "/bin/zsh")
	}

	// Test with SHELL unset — should fall back to default.
	os.Unsetenv("SHELL")
	if err := c.Init(context.Background()); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if c.shell != defaultShell {
		t.Errorf("Init() shell = %q, want %q", c.shell, defaultShell)
	}
}

func TestLocalConnector_ExecuteCommand(t *testing.T) {
	c := NewLocalConnector("")
	ctx := context.Background()
	if err := c.Init(ctx); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	stdout, stderr, err := c.ExecuteCommand(ctx, "echo hello")
	if err != nil {
		t.Fatalf("ExecuteCommand() error = %v, stderr = %s", err, string(stderr))
	}
	got := strings.TrimSpace(string(stdout))
	if got != "hello" {
		t.Errorf("ExecuteCommand() stdout = %q, want %q", got, "hello")
	}
}

func TestLocalConnector_ExecuteCommand_Stderr(t *testing.T) {
	c := NewLocalConnector("")
	ctx := context.Background()
	if err := c.Init(ctx); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	_, stderr, err := c.ExecuteCommand(ctx, "echo error_output >&2 && false")
	if err == nil {
		t.Fatal("ExecuteCommand() expected error for failing command, got nil")
	}
	if !strings.Contains(string(stderr), "error_output") {
		t.Errorf("ExecuteCommand() stderr = %q, want it to contain %q", string(stderr), "error_output")
	}
}

func TestLocalConnector_PutFetchFile(t *testing.T) {
	c := NewLocalConnector("")
	ctx := context.Background()
	if err := c.Init(ctx); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.txt")
	content := []byte("hello, connector!")

	// Put file.
	if err := c.PutFile(ctx, content, filePath, 0o644); err != nil {
		t.Fatalf("PutFile() error = %v", err)
	}

	// Verify file exists and has correct content.
	got, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("PutFile() content = %q, want %q", string(got), string(content))
	}

	// Fetch file back via connector.
	var buf bytes.Buffer
	if err := c.FetchFile(ctx, filePath, &buf); err != nil {
		t.Fatalf("FetchFile() error = %v", err)
	}
	if !bytes.Equal(buf.Bytes(), content) {
		t.Errorf("FetchFile() content = %q, want %q", buf.String(), string(content))
	}
}

func TestLocalConnector_PutFile_CreatesDirs(t *testing.T) {
	c := NewLocalConnector("")
	ctx := context.Background()
	if err := c.Init(ctx); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	tmpDir := t.TempDir()
	// Nested path that doesn't exist yet.
	filePath := filepath.Join(tmpDir, "a", "b", "c", "nested.txt")
	content := []byte("nested content")

	if err := c.PutFile(ctx, content, filePath, 0o644); err != nil {
		t.Fatalf("PutFile() error = %v", err)
	}

	// Verify parent directories were created.
	info, err := os.Stat(filepath.Dir(filePath))
	if err != nil {
		t.Fatalf("Stat() parent dir error = %v", err)
	}
	if !info.IsDir() {
		t.Error("expected parent path to be a directory")
	}

	// Verify file content.
	got, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("PutFile() content = %q, want %q", string(got), string(content))
	}
}

func TestLocalConnector_FetchFile_NotFound(t *testing.T) {
	c := NewLocalConnector("")
	ctx := context.Background()
	if err := c.Init(ctx); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	var buf bytes.Buffer
	err := c.FetchFile(ctx, "/nonexistent/path/file.txt", &buf)
	if err == nil {
		t.Fatal("FetchFile() expected error for non-existent file, got nil")
	}
}

func TestLocalConnector_Close(t *testing.T) {
	c := NewLocalConnector("")
	ctx := context.Background()
	if err := c.Init(ctx); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	// Close should be a no-op and not error.
	if err := c.Close(ctx); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestConvertBytesToMap(t *testing.T) {
	input := []byte("FOO=bar\nBAZ = qux\ninvalid line\nKEY=value with spaces\n")
	got := ParseKeyValues(input, "=")

	tests := map[string]string{
		"FOO": "bar",
		"BAZ": "qux",
		"KEY": "value with spaces",
	}
	for k, want := range tests {
		if got[k] != want {
			t.Errorf("ParseKeyValues()[%q] = %q, want %q", k, got[k], want)
		}
	}
	if _, ok := got["invalid line"]; ok {
		t.Error("ParseKeyValues() should not include lines without delimiter")
	}
}

func TestConvertBytesToSlice(t *testing.T) {
	input := []byte("foo: bar\nbaz: qux\n\nhello: world\n")
	got := ParseLines(input, ":")

	if len(got) != 2 {
		t.Fatalf("ParseLines() returned %d groups, want 2", len(got))
	}
	if got[0]["foo"] != "bar" {
		t.Errorf("group 0 foo = %q, want %q", got[0]["foo"], "bar")
	}
	if got[0]["baz"] != "qux" {
		t.Errorf("group 0 baz = %q, want %q", got[0]["baz"], "qux")
	}
	if got[1]["hello"] != "world" {
		t.Errorf("group 1 hello = %q, want %q", got[1]["hello"], "world")
	}
}
