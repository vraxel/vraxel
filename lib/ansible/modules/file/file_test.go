package file

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"vraxel.io/vraxel/lib/ansible/connector"
	"vraxel.io/vraxel/lib/ansible/modules/internal"
)

// localOpts runs the module against a real local shell, so the emitted
// commands are exercised rather than merely inspected.
func localOpts(t *testing.T, args map[string]any) internal.ExecOptions {
	t.Helper()
	conn := connector.NewLocalConnector("")
	if err := conn.Init(context.Background()); err != nil {
		t.Fatalf("init local connector: %v", err)
	}
	return internal.ExecOptions{Args: args, Connector: conn}
}

func TestModuleFile_Directory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "deep")

	if _, _, err := ModuleFile(context.Background(), localOpts(t, map[string]any{
		"path": dir, "state": "directory", "mode": "0750",
	})); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected a directory")
	}
	if got := info.Mode().Perm(); got != 0750 {
		t.Errorf("mode = %o, want 750", got)
	}
}

func TestModuleFile_TouchCreatesParents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "app.conf")

	if _, _, err := ModuleFile(context.Background(), localOpts(t, map[string]any{
		"path": path, "state": "touch",
	})); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file not created: %v", err)
	}
}

func TestModuleFile_Absent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gone.txt")
	if err := os.WriteFile(path, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	if _, _, err := ModuleFile(context.Background(), localOpts(t, map[string]any{
		"path": path, "state": "absent",
	})); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("expected the path to be removed")
	}
}

func TestModuleFile_Link(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "target")
	if err := os.WriteFile(src, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")

	if _, _, err := ModuleFile(context.Background(), localOpts(t, map[string]any{
		"path": link, "state": "link", "src": src,
	})); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("not a symlink: %v", err)
	}
	if got != src {
		t.Errorf("link target = %q, want %q", got, src)
	}
}

func TestModuleFile_PathWithSpacesAndQuote(t *testing.T) {
	// The command is interpolated into a shell, so the path must survive it.
	dir := filepath.Join(t.TempDir(), "a b'c")

	if _, _, err := ModuleFile(context.Background(), localOpts(t, map[string]any{
		"path": dir, "state": "directory",
	})); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("directory with an awkward name not created: %v", err)
	}
}

func TestModuleFile_ArgumentErrors(t *testing.T) {
	if _, _, err := ModuleFile(context.Background(), localOpts(t, map[string]any{})); err == nil {
		t.Error("expected an error when path is missing")
	}
	if _, _, err := ModuleFile(context.Background(), localOpts(t, map[string]any{
		"path": "/tmp/x", "state": "link",
	})); err == nil {
		t.Error("expected an error when state=link has no src")
	}
	if _, _, err := ModuleFile(context.Background(), localOpts(t, map[string]any{
		"path": "/tmp/x", "state": "bogus",
	})); err == nil {
		t.Error("expected an error for an unsupported state")
	}
}

func TestModuleFile_StateFileAssertsExistence(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope")
	if _, _, err := ModuleFile(context.Background(), localOpts(t, map[string]any{
		"path": missing, "state": "file",
	})); err == nil {
		t.Error("state=file must fail on a missing path rather than create it")
	}
}
