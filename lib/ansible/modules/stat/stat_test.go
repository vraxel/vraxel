package stat

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"vraxel.io/vraxel/lib/ansible/connector"
	"vraxel.io/vraxel/lib/ansible/modules/internal"
)

// statPath runs the module against a real local shell and decodes its JSON.
func statPath(t *testing.T, path string) map[string]any {
	t.Helper()

	conn := connector.NewLocalConnector("")
	if err := conn.Init(context.Background()); err != nil {
		t.Fatalf("init local connector: %v", err)
	}

	stdout, _, err := ModuleStat(context.Background(), internal.ExecOptions{
		Args:      map[string]any{"path": path},
		Connector: conn,
	})
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}

	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("stat output is not JSON (%q): %v", stdout, err)
	}
	return out
}

func TestModuleStat_RegularFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.conf")
	if err := os.WriteFile(path, []byte("hello"), 0640); err != nil {
		t.Fatal(err)
	}

	got := statPath(t, path)
	if got["exists"] != true || got["isreg"] != true {
		t.Errorf("expected an existing regular file, got %v", got)
	}
	if got["isdir"] != false {
		t.Errorf("expected isdir=false, got %v", got["isdir"])
	}
	if got["size"] != float64(5) {
		t.Errorf("size = %v, want 5", got["size"])
	}
	// Mode is read through GNU stat with a BSD fallback; both must answer.
	if got["mode"] != "640" {
		t.Errorf("mode = %v, want 640", got["mode"])
	}
}

func TestModuleStat_Directory(t *testing.T) {
	got := statPath(t, t.TempDir())
	if got["exists"] != true || got["isdir"] != true {
		t.Errorf("expected an existing directory, got %v", got)
	}
	if got["isreg"] != false {
		t.Errorf("expected isreg=false, got %v", got["isreg"])
	}
}

func TestModuleStat_MissingPathIsNotAFailure(t *testing.T) {
	// stat has to stay usable as a condition, so absence is a result, not an
	// error.
	got := statPath(t, filepath.Join(t.TempDir(), "nope"))
	if got["exists"] != false {
		t.Errorf("expected exists=false, got %v", got)
	}
}

func TestModuleStat_Symlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	got := statPath(t, link)
	if got["exists"] != true || got["islnk"] != true {
		t.Errorf("expected an existing symlink, got %v", got)
	}
}

func TestModuleStat_PathWithSpaces(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a b'c.conf")
	if err := os.WriteFile(path, []byte("xy"), 0644); err != nil {
		t.Fatal(err)
	}

	got := statPath(t, path)
	if got["exists"] != true || got["size"] != float64(2) {
		t.Errorf("awkward path not handled, got %v", got)
	}
}

func TestModuleStat_PathRequired(t *testing.T) {
	if _, _, err := ModuleStat(context.Background(), internal.ExecOptions{
		Args: map[string]any{},
	}); err == nil {
		t.Error("expected an error when path is missing")
	}
}
