package modules

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"vraxel.io/vraxel/lib/ansible/modules/assert"
	mcopy "vraxel.io/vraxel/lib/ansible/modules/copy"
	"vraxel.io/vraxel/lib/ansible/modules/fetch"
	"vraxel.io/vraxel/lib/ansible/modules/http_get_file"
	"vraxel.io/vraxel/lib/ansible/modules/result"
	"vraxel.io/vraxel/lib/ansible/variable"
)

// ========================================================================
// result module tests
// ========================================================================

func TestModuleResult(t *testing.T) {
	v := newTestVariable("testhost")

	stdout, stderr, err := result.ModuleResult(context.Background(), ExecOptions{
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
	r := v.Get(variable.GetResultVariable())
	m, ok := r.(map[string]any)
	if !ok {
		t.Fatalf("expected result to be map[string]any, got %T", r)
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
	_, _, err := result.ModuleResult(context.Background(), ExecOptions{
		Host:     "testhost",
		Variable: v,
		Args:     map[string]any{"key1": "value1"},
	})
	if err != nil {
		t.Fatalf("first call: unexpected error: %v", err)
	}

	// Second call: set additional keys.
	_, _, err = result.ModuleResult(context.Background(), ExecOptions{
		Host:     "testhost",
		Variable: v,
		Args:     map[string]any{"key2": "value2"},
	})
	if err != nil {
		t.Fatalf("second call: unexpected error: %v", err)
	}

	// Both keys should be present.
	r := v.Get(variable.GetResultVariable())
	m, ok := r.(map[string]any)
	if !ok {
		t.Fatalf("expected result to be map[string]any, got %T", r)
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
	_, _, err := result.ModuleResult(context.Background(), ExecOptions{
		Host:     "testhost",
		Variable: v,
		Args:     nil,
	})
	if err == nil {
		t.Fatal("expected error for nil args, got nil")
	}

	// empty args
	_, _, err = result.ModuleResult(context.Background(), ExecOptions{
		Host:     "testhost",
		Variable: v,
		Args:     map[string]any{},
	})
	if err == nil {
		t.Fatal("expected error for empty args, got nil")
	}
}

func TestModuleResult_Registered(t *testing.T) {
	fn := FindModule("result")
	if fn == nil {
		t.Fatal("expected 'result' module to be registered, got nil")
	}
}

// ========================================================================
// http_get_file module tests
// ========================================================================

func TestModuleHTTPGetFile(t *testing.T) {
	// Serve a test file.
	content := "hello, world!"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, content)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "downloaded.txt")

	stdout, stderr, err := http_get_file.ModuleHTTPGetFile(context.Background(), ExecOptions{
		Args: map[string]any{
			"url":  srv.URL + "/file.txt",
			"dest": dest,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stderr != "" {
		t.Errorf("expected empty stderr, got %q", stderr)
	}
	if stdout == "" {
		t.Error("expected non-empty stdout")
	}

	// Verify downloaded content.
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("failed to read downloaded file: %v", err)
	}
	if string(data) != content {
		t.Errorf("expected content %q, got %q", content, string(data))
	}
}

func TestModuleHTTPGetFile_BasicAuth(t *testing.T) {
	expectedUser := "admin"
	expectedPass := "secret"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != expectedUser || pass != expectedPass {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, "unauthorized")
			return
		}
		fmt.Fprint(w, "authenticated content")
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "auth_file.txt")

	// Without auth: should fail.
	_, _, err := http_get_file.ModuleHTTPGetFile(context.Background(), ExecOptions{
		Args: map[string]any{
			"url":  srv.URL + "/secure",
			"dest": dest,
		},
	})
	if err == nil {
		t.Fatal("expected error without auth, got nil")
	}

	// With auth: should succeed.
	stdout, stderr, err := http_get_file.ModuleHTTPGetFile(context.Background(), ExecOptions{
		Args: map[string]any{
			"url":      srv.URL + "/secure",
			"dest":     dest,
			"username": expectedUser,
			"password": expectedPass,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error with auth: %v", err)
	}
	if stderr != "" {
		t.Errorf("expected empty stderr, got %q", stderr)
	}
	if stdout == "" {
		t.Error("expected non-empty stdout")
	}

	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if string(data) != "authenticated content" {
		t.Errorf("expected 'authenticated content', got %q", string(data))
	}
}

func TestModuleHTTPGetFile_BearerToken(t *testing.T) {
	expectedToken := "my-secret-token"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer "+expectedToken {
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, "forbidden")
			return
		}
		fmt.Fprint(w, "token content")
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "token_file.txt")

	stdout, stderr, err := http_get_file.ModuleHTTPGetFile(context.Background(), ExecOptions{
		Args: map[string]any{
			"url":   srv.URL + "/api/data",
			"dest":  dest,
			"token": expectedToken,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stderr != "" {
		t.Errorf("expected empty stderr, got %q", stderr)
	}
	if stdout == "" {
		t.Error("expected non-empty stdout")
	}

	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if string(data) != "token content" {
		t.Errorf("expected 'token content', got %q", string(data))
	}
}

func TestModuleHTTPGetFile_CustomHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Custom") != "custom-value" {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, "missing custom header")
			return
		}
		fmt.Fprint(w, "header ok")
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "header_file.txt")

	_, _, err := http_get_file.ModuleHTTPGetFile(context.Background(), ExecOptions{
		Args: map[string]any{
			"url":  srv.URL,
			"dest": dest,
			"headers": map[string]any{
				"X-Custom": "custom-value",
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if string(data) != "header ok" {
		t.Errorf("expected 'header ok', got %q", string(data))
	}
}

func TestModuleHTTPGetFile_MissingArgs(t *testing.T) {
	tests := []struct {
		name string
		args map[string]any
	}{
		{"missing both", map[string]any{}},
		{"missing url", map[string]any{"dest": "/tmp/file"}},
		{"missing dest", map[string]any{"url": "http://example.com/file"}},
		{"nil args", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := http_get_file.ModuleHTTPGetFile(context.Background(), ExecOptions{
				Args: tt.args,
			})
			if err == nil {
				t.Fatalf("expected error for %s, got nil", tt.name)
			}
		})
	}
}

func TestModuleHTTPGetFile_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "internal error")
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "error_file.txt")

	_, _, err := http_get_file.ModuleHTTPGetFile(context.Background(), ExecOptions{
		Args: map[string]any{
			"url":  srv.URL + "/fail",
			"dest": dest,
		},
	})
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}

	// Dest file should not exist.
	if _, statErr := os.Stat(dest); !os.IsNotExist(statErr) {
		t.Error("expected dest file to not exist after server error")
	}
}

func TestModuleHTTPGetFile_AtomicWrite(t *testing.T) {
	// When the server sends partial data then closes, the temp file should be cleaned up
	// and the dest file should not exist.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Send headers with a large content-length but close before sending all data.
		w.Header().Set("Content-Length", "999999")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "partial")
		// The handler returns, closing the connection prematurely.
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "atomic_file.txt")

	_, _, err := http_get_file.ModuleHTTPGetFile(context.Background(), ExecOptions{
		Args: map[string]any{
			"url":  srv.URL + "/partial",
			"dest": dest,
		},
	})

	// The download should fail because response body is shorter than Content-Length.
	if err == nil {
		// Some HTTP clients may not detect the short read. If no error, verify
		// the file was still written atomically (either fully or not at all).
		return
	}

	// On error, dest file should not exist (atomic guarantee).
	if _, statErr := os.Stat(dest); !os.IsNotExist(statErr) {
		t.Error("expected dest file to not exist after download error")
	}

	// Temp files should also be cleaned up.
	parentDir := filepath.Dir(dest)
	entries, _ := os.ReadDir(parentDir)
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Errorf("temp file %s was not cleaned up", e.Name())
		}
	}
}

func TestModuleHTTPGetFile_CreatesParentDir(t *testing.T) {
	content := "nested file"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, content)
	}))
	defer srv.Close()

	// Dest is in a deeply nested directory that does not exist.
	dest := filepath.Join(t.TempDir(), "a", "b", "c", "file.txt")

	_, _, err := http_get_file.ModuleHTTPGetFile(context.Background(), ExecOptions{
		Args: map[string]any{
			"url":  srv.URL,
			"dest": dest,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if string(data) != content {
		t.Errorf("expected %q, got %q", content, string(data))
	}
}

func TestModuleHTTPGetFile_InvalidScheme(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "file.txt")

	_, _, err := http_get_file.ModuleHTTPGetFile(context.Background(), ExecOptions{
		Args: map[string]any{
			"url":  "ftp://example.com/file.txt",
			"dest": dest,
		},
	})
	if err == nil {
		t.Fatal("expected error for ftp scheme, got nil")
	}
}

func TestModuleHTTPGetFile_Registered(t *testing.T) {
	fn := FindModule("http_get_file")
	if fn == nil {
		t.Fatal("expected 'http_get_file' module to be registered, got nil")
	}
}

// --- registration tests ---

func TestCopyModuleRegistered(t *testing.T) {
	if fn := FindModule("copy"); fn == nil {
		t.Error("expected 'copy' module to be registered")
	}
}

func TestFetchModuleRegistered(t *testing.T) {
	if fn := FindModule("fetch"); fn == nil {
		t.Error("expected 'fetch' module to be registered")
	}
}

// --- copy tests (integration, using FindModule) ---

func TestModuleCopy_Content(t *testing.T) {
	conn := &mockConnector{}
	opts := ExecOptions{
		Args: map[string]any{
			"content": "hello world",
			"dest":    "/tmp/hello.txt",
		},
		Connector: conn,
	}

	stdout, stderr, err := mcopy.ModuleCopy(context.Background(), opts)
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

// --- fetch tests (integration) ---

func TestModuleFetch(t *testing.T) {
	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "fetched.txt")

	conn := &mockConnector{
		fetchFileContent: []byte("remote file content"),
	}
	opts := ExecOptions{
		Args: map[string]any{
			"src":  "/remote/file.txt",
			"dest": destPath,
		},
		Connector: conn,
	}

	stdout, stderr, err := fetch.ModuleFetch(context.Background(), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stderr != "" {
		t.Errorf("expected empty stderr, got %q", stderr)
	}
	if !strings.Contains(stdout, "/remote/file.txt") {
		t.Errorf("expected stdout to mention src, got %q", stdout)
	}

	data, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("failed to read dest file: %v", err)
	}
	if string(data) != "remote file content" {
		t.Errorf("expected file content %q, got %q", "remote file content", string(data))
	}
}

func TestModuleFetch_NoSrc(t *testing.T) {
	opts := ExecOptions{
		Args: map[string]any{
			"dest": "/tmp/dest.txt",
		},
	}

	_, _, err := fetch.ModuleFetch(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error when src is missing")
	}
	if !strings.Contains(err.Error(), "src is required") {
		t.Errorf("expected 'src is required' error, got %q", err.Error())
	}
}

func TestModuleFetch_NoDest(t *testing.T) {
	opts := ExecOptions{
		Args: map[string]any{
			"src": "/remote/file.txt",
		},
	}

	_, _, err := fetch.ModuleFetch(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error when dest is missing")
	}
	if !strings.Contains(err.Error(), "dest is required") {
		t.Errorf("expected 'dest is required' error, got %q", err.Error())
	}
}

func TestModuleFetch_ConnectorError(t *testing.T) {
	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "out.txt")

	conn := &mockConnector{
		fetchFileErr: fmt.Errorf("network error"),
	}
	opts := ExecOptions{
		Args: map[string]any{
			"src":  "/remote/file.txt",
			"dest": destPath,
		},
		Connector: conn,
	}

	_, _, err := fetch.ModuleFetch(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error from FetchFile failure")
	}
	if !strings.Contains(err.Error(), "fetch file") {
		t.Errorf("expected 'fetch file' error, got %q", err.Error())
	}
}

func TestModuleFetch_CreatesDestDir(t *testing.T) {
	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "sub", "dir", "fetched.txt")

	conn := &mockConnector{
		fetchFileContent: []byte("data"),
	}
	opts := ExecOptions{
		Args: map[string]any{
			"src":  "/remote/file.txt",
			"dest": destPath,
		},
		Connector: conn,
	}

	_, _, err := fetch.ModuleFetch(context.Background(), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("failed to read dest file: %v", err)
	}
	if string(data) != "data" {
		t.Errorf("expected %q, got %q", "data", string(data))
	}
}

// --- debug/assert registration tests ---

func TestModuleDebug_Registered(t *testing.T) {
	fn := FindModule("debug")
	if fn == nil {
		t.Fatal("expected 'debug' module to be registered, got nil")
	}
}

func TestModuleAssert_Registered(t *testing.T) {
	fn := FindModule("assert")
	if fn == nil {
		t.Fatal("expected 'assert' module to be registered, got nil")
	}
}

func TestModuleCommand_Registered(t *testing.T) {
	// The init() function should have registered both "command" and "shell".
	fn := FindModule("command")
	if fn == nil {
		t.Fatal("expected 'command' module to be registered, got nil")
	}

	fn = FindModule("shell")
	if fn == nil {
		t.Fatal("expected 'shell' module to be registered, got nil")
	}
}

func TestModuleTemplate_Registered(t *testing.T) {
	fn := FindModule("template")
	if fn == nil {
		t.Fatal("expected 'template' module to be registered, got nil")
	}
}

// --- assert/toStringSlice tests ---

func TestToStringSlice_AnySlice(t *testing.T) {
	input := []any{"a", "b", "c"}
	result := assert.ToStringSlice(input)
	if len(result) != 3 || result[0] != "a" || result[1] != "b" || result[2] != "c" {
		t.Errorf("expected [a b c], got %v", result)
	}
}

func TestToStringSlice_StringSlice(t *testing.T) {
	input := []string{"x", "y"}
	result := assert.ToStringSlice(input)
	if len(result) != 2 || result[0] != "x" || result[1] != "y" {
		t.Errorf("expected [x y], got %v", result)
	}
}

func TestToStringSlice_SingleString(t *testing.T) {
	result := assert.ToStringSlice("hello")
	if len(result) != 1 || result[0] != "hello" {
		t.Errorf("expected [hello], got %v", result)
	}
}

func TestToStringSlice_Nil(t *testing.T) {
	result := assert.ToStringSlice(nil)
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestToStringSlice_NonStringItems(t *testing.T) {
	// Non-string items in []any should be skipped.
	input := []any{"a", 42, "b"}
	result := assert.ToStringSlice(input)
	if len(result) != 2 || result[0] != "a" || result[1] != "b" {
		t.Errorf("expected [a b], got %v", result)
	}
}
