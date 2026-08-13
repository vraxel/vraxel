package template

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"strings"
	"testing"

	"vraxel.io/vraxel/lib/ansible"
	"vraxel.io/vraxel/lib/ansible/modules/internal"
	"vraxel.io/vraxel/lib/ansible/variable"
)

// mockConnector implements connector.Connector for testing.
type mockConnector struct {
	putFileData []byte
	putFileDest string
	putFileMode fs.FileMode
	putFileErr  error
}

func (m *mockConnector) Init(context.Context) error  { return nil }
func (m *mockConnector) Close(context.Context) error { return nil }
func (m *mockConnector) ExecuteCommand(_ context.Context, _ string) ([]byte, []byte, error) {
	return nil, nil, nil
}
func (m *mockConnector) PutFile(_ context.Context, src []byte, dst string, mode fs.FileMode) error {
	m.putFileData = src
	m.putFileDest = dst
	m.putFileMode = mode
	return m.putFileErr
}
func (m *mockConnector) FetchFile(_ context.Context, _ string, _ io.Writer) error { return nil }
func (m *mockConnector) Resize(_ context.Context, _, _ uint16) error              { return nil }

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

func TestModuleTemplate(t *testing.T) {
	source := &mockSource{
		files: map[string][]byte{
			"config.tmpl": []byte("Hello {{ .name }}"),
		},
	}

	conn := &mockConnector{}

	inv := ansible.Inventory{
		Hosts: map[string]map[string]any{
			"host1": {"name": "world"},
		},
	}
	v := variable.New(inv)

	opts := internal.ExecOptions{
		Args: map[string]any{
			"src":  "config.tmpl",
			"dest": "/etc/app/config.yaml",
		},
		Host:      "host1",
		Variable:  v,
		Connector: conn,
		Source:    source,
	}

	stdout, stderr, err := ModuleTemplate(context.Background(), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stderr != "" {
		t.Errorf("expected empty stderr, got %q", stderr)
	}
	if !strings.Contains(stdout, "templated") {
		t.Errorf("expected stdout to contain 'templated', got %q", stdout)
	}

	// Verify PutFile was called with rendered content.
	if string(conn.putFileData) != "Hello world" {
		t.Errorf("expected rendered content 'Hello world', got %q", string(conn.putFileData))
	}
	if conn.putFileDest != "/etc/app/config.yaml" {
		t.Errorf("expected dest '/etc/app/config.yaml', got %q", conn.putFileDest)
	}
	if conn.putFileMode != 0644 {
		t.Errorf("expected default mode 0644, got %o", conn.putFileMode)
	}
}

func TestModuleTemplate_CustomMode(t *testing.T) {
	source := &mockSource{
		files: map[string][]byte{
			"script.tmpl": []byte("#!/bin/bash\necho {{ .msg }}"),
		},
	}

	conn := &mockConnector{}

	inv := ansible.Inventory{
		Hosts: map[string]map[string]any{
			"host1": {"msg": "hello"},
		},
	}
	v := variable.New(inv)

	opts := internal.ExecOptions{
		Args: map[string]any{
			"src":  "script.tmpl",
			"dest": "/usr/local/bin/run.sh",
			"mode": 0755,
		},
		Host:      "host1",
		Variable:  v,
		Connector: conn,
		Source:    source,
	}

	_, _, err := ModuleTemplate(context.Background(), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if conn.putFileMode != 0755 {
		t.Errorf("expected mode 0755, got %o", conn.putFileMode)
	}
}

func TestModuleTemplate_MissingSrc(t *testing.T) {
	opts := internal.ExecOptions{
		Args: map[string]any{
			"dest": "/etc/app/config.yaml",
		},
	}

	_, _, err := ModuleTemplate(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error for missing src")
	}
	if !strings.Contains(err.Error(), "src and dest are required") {
		t.Errorf("expected 'src and dest are required' error, got %q", err.Error())
	}
}

func TestModuleTemplate_MissingDest(t *testing.T) {
	opts := internal.ExecOptions{
		Args: map[string]any{
			"src": "config.tmpl",
		},
	}

	_, _, err := ModuleTemplate(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error for missing dest")
	}
	if !strings.Contains(err.Error(), "src and dest are required") {
		t.Errorf("expected 'src and dest are required' error, got %q", err.Error())
	}
}

func TestModuleTemplate_MissingBoth(t *testing.T) {
	opts := internal.ExecOptions{
		Args: map[string]any{},
	}

	_, _, err := ModuleTemplate(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error for missing src and dest")
	}
	if !strings.Contains(err.Error(), "src and dest are required") {
		t.Errorf("expected 'src and dest are required' error, got %q", err.Error())
	}
}

func TestModuleTemplate_SourceReadError(t *testing.T) {
	source := &mockSource{
		files: map[string][]byte{}, // empty — file not found
	}

	inv := ansible.Inventory{
		Hosts: map[string]map[string]any{
			"host1": {},
		},
	}
	v := variable.New(inv)

	opts := internal.ExecOptions{
		Args: map[string]any{
			"src":  "missing.tmpl",
			"dest": "/etc/app/config.yaml",
		},
		Host:     "host1",
		Variable: v,
		Source:   source,
	}

	_, _, err := ModuleTemplate(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error for missing source file")
	}
	if !strings.Contains(err.Error(), "template: read") {
		t.Errorf("expected 'template: read' error, got %q", err.Error())
	}
}

func TestModuleTemplate_RenderError(t *testing.T) {
	// Use an unclosed template action to guarantee a parse error.
	source := &mockSource{
		files: map[string][]byte{
			"bad.tmpl": []byte("{{ .name | nonExistentFunc12345 }}"),
		},
	}

	inv := ansible.Inventory{
		Hosts: map[string]map[string]any{
			"host1": {"name": "test"},
		},
	}
	v := variable.New(inv)

	conn := &mockConnector{}

	opts := internal.ExecOptions{
		Args: map[string]any{
			"src":  "bad.tmpl",
			"dest": "/etc/app/config.yaml",
		},
		Host:      "host1",
		Variable:  v,
		Connector: conn,
		Source:    source,
	}

	_, _, err := ModuleTemplate(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error for invalid template")
	}
	if !strings.Contains(err.Error(), "template: render") {
		t.Errorf("expected 'template: render' error, got %q", err.Error())
	}
}

func TestModuleTemplate_PutFileError(t *testing.T) {
	source := &mockSource{
		files: map[string][]byte{
			"config.tmpl": []byte("simple content"),
		},
	}

	conn := &mockConnector{
		putFileErr: fmt.Errorf("connection refused"),
	}

	inv := ansible.Inventory{
		Hosts: map[string]map[string]any{
			"host1": {},
		},
	}
	v := variable.New(inv)

	opts := internal.ExecOptions{
		Args: map[string]any{
			"src":  "config.tmpl",
			"dest": "/etc/app/config.yaml",
		},
		Host:      "host1",
		Variable:  v,
		Connector: conn,
		Source:    source,
	}

	_, _, err := ModuleTemplate(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error for PutFile failure")
	}
	if !strings.Contains(err.Error(), "template: put file") {
		t.Errorf("expected 'template: put file' error, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "connection refused") {
		t.Errorf("expected underlying error 'connection refused', got %q", err.Error())
	}
}

func TestModuleTemplate_NoTemplateContent(t *testing.T) {
	// Content without template syntax is passed through unchanged.
	source := &mockSource{
		files: map[string][]byte{
			"plain.txt": []byte("plain content without templates"),
		},
	}

	conn := &mockConnector{}

	inv := ansible.Inventory{
		Hosts: map[string]map[string]any{
			"host1": {},
		},
	}
	v := variable.New(inv)

	opts := internal.ExecOptions{
		Args: map[string]any{
			"src":  "plain.txt",
			"dest": "/etc/app/plain.txt",
		},
		Host:      "host1",
		Variable:  v,
		Connector: conn,
		Source:    source,
	}

	stdout, _, err := ModuleTemplate(context.Background(), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "templated") {
		t.Errorf("expected stdout to contain 'templated', got %q", stdout)
	}

	if string(conn.putFileData) != "plain content without templates" {
		t.Errorf("expected plain content passed through, got %q", string(conn.putFileData))
	}
}

func TestModuleTemplate_NilSource(t *testing.T) {
	inv := ansible.Inventory{
		Hosts: map[string]map[string]any{
			"host1": {},
		},
	}
	v := variable.New(inv)

	opts := internal.ExecOptions{
		Args: map[string]any{
			"src":  "config.tmpl",
			"dest": "/etc/app/config.yaml",
		},
		Host:     "host1",
		Variable: v,
		Source:   nil,
	}

	_, _, err := ModuleTemplate(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error for nil source")
	}
	if !strings.Contains(err.Error(), "template: read") {
		t.Errorf("expected 'template: read' error, got %q", err.Error())
	}
}
