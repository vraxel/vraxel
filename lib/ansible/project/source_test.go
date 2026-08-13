package project

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

// ---------------------------------------------------------------------------
// LocalSource tests
// ---------------------------------------------------------------------------

func TestLocalSource_ReadFile(t *testing.T) {
	dir := t.TempDir()
	content := []byte("hello world")
	if err := os.WriteFile(filepath.Join(dir, "test.txt"), content, 0644); err != nil {
		t.Fatal(err)
	}

	src := NewLocalSource(dir)
	got, err := src.ReadFile("test.txt")
	if err != nil {
		t.Fatalf("ReadFile: unexpected error: %v", err)
	}
	if string(got) != string(content) {
		t.Fatalf("ReadFile: got %q, want %q", got, content)
	}
}

func TestLocalSource_ReadFile_NotFound(t *testing.T) {
	src := NewLocalSource(t.TempDir())
	_, err := src.ReadFile("nonexistent.txt")
	if err == nil {
		t.Fatal("ReadFile: expected error for missing file, got nil")
	}
}

func TestLocalSource_ReadDir(t *testing.T) {
	dir := t.TempDir()
	// Create sub-directory and files.
	subdir := filepath.Join(dir, "sub")
	if err := os.Mkdir(subdir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.txt", "b.txt"} {
		if err := os.WriteFile(filepath.Join(subdir, name), []byte(name), 0644); err != nil {
			t.Fatal(err)
		}
	}

	src := NewLocalSource(dir)
	entries, err := src.ReadDir("sub")
	if err != nil {
		t.Fatalf("ReadDir: unexpected error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("ReadDir: got %d entries, want 2", len(entries))
	}
	names := make(map[string]bool)
	for _, e := range entries {
		names[e.Name()] = true
	}
	for _, want := range []string{"a.txt", "b.txt"} {
		if !names[want] {
			t.Errorf("ReadDir: missing entry %q", want)
		}
	}
}

func TestLocalSource_ReadDir_Root(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "root.txt"), []byte("r"), 0644); err != nil {
		t.Fatal(err)
	}

	src := NewLocalSource(dir)
	entries, err := src.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir root: unexpected error: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("ReadDir root: expected at least one entry")
	}
}

func TestLocalSource_Stat(t *testing.T) {
	dir := t.TempDir()
	fpath := filepath.Join(dir, "info.txt")
	if err := os.WriteFile(fpath, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	src := NewLocalSource(dir)
	info, err := src.Stat("info.txt")
	if err != nil {
		t.Fatalf("Stat: unexpected error: %v", err)
	}
	if info.IsDir() {
		t.Fatal("Stat: expected file, got directory")
	}
	if info.Size() != 4 {
		t.Fatalf("Stat: got size %d, want 4", info.Size())
	}
}

func TestLocalSource_Stat_NotFound(t *testing.T) {
	src := NewLocalSource(t.TempDir())
	_, err := src.Stat("nonexistent.txt")
	if err == nil {
		t.Fatal("Stat: expected error for missing file, got nil")
	}
}

func TestLocalSource_Stat_Dir(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "mydir"), 0755); err != nil {
		t.Fatal(err)
	}

	src := NewLocalSource(dir)
	info, err := src.Stat("mydir")
	if err != nil {
		t.Fatalf("Stat dir: unexpected error: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("Stat dir: expected directory")
	}
}

// ---------------------------------------------------------------------------
// BuiltinSource tests
// ---------------------------------------------------------------------------

func TestBuiltinSource_ReadFile(t *testing.T) {
	mapFS := fstest.MapFS{
		"playbooks/site.yml":       {Data: []byte("---\n- hosts: all")},
		"playbooks/roles/main.yml": {Data: []byte("tasks:")},
	}

	src := NewBuiltinSource(mapFS, "playbooks")

	got, err := src.ReadFile("site.yml")
	if err != nil {
		t.Fatalf("ReadFile: unexpected error: %v", err)
	}
	if string(got) != "---\n- hosts: all" {
		t.Fatalf("ReadFile: got %q, want %q", got, "---\n- hosts: all")
	}

	got, err = src.ReadFile("roles/main.yml")
	if err != nil {
		t.Fatalf("ReadFile nested: unexpected error: %v", err)
	}
	if string(got) != "tasks:" {
		t.Fatalf("ReadFile nested: got %q, want %q", got, "tasks:")
	}
}

func TestBuiltinSource_ReadFile_NotFound(t *testing.T) {
	mapFS := fstest.MapFS{}
	src := NewBuiltinSource(mapFS, "playbooks")

	_, err := src.ReadFile("missing.yml")
	if err == nil {
		t.Fatal("ReadFile: expected error for missing file, got nil")
	}
}

func TestBuiltinSource_ReadDir(t *testing.T) {
	mapFS := fstest.MapFS{
		"data/a.txt": {Data: []byte("a")},
		"data/b.txt": {Data: []byte("b")},
	}

	src := NewBuiltinSource(mapFS, "data")
	entries, err := src.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir: unexpected error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("ReadDir: got %d entries, want 2", len(entries))
	}
}

func TestBuiltinSource_ReadDir_NoPrefix(t *testing.T) {
	mapFS := fstest.MapFS{
		"x.txt": {Data: []byte("x")},
		"y.txt": {Data: []byte("y")},
	}

	src := NewBuiltinSource(mapFS, "")
	entries, err := src.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir no prefix: unexpected error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("ReadDir no prefix: got %d entries, want 2", len(entries))
	}
}

func TestBuiltinSource_Stat(t *testing.T) {
	mapFS := fstest.MapFS{
		"proj/file.yml": {Data: []byte("content")},
	}

	src := NewBuiltinSource(mapFS, "proj")
	info, err := src.Stat("file.yml")
	if err != nil {
		t.Fatalf("Stat: unexpected error: %v", err)
	}
	if info.IsDir() {
		t.Fatal("Stat: expected file, got directory")
	}
	if info.Size() != 7 {
		t.Fatalf("Stat: got size %d, want 7", info.Size())
	}
}

func TestBuiltinSource_Stat_NotFound(t *testing.T) {
	mapFS := fstest.MapFS{}
	src := NewBuiltinSource(mapFS, "proj")

	_, err := src.Stat("nope.yml")
	if err == nil {
		t.Fatal("Stat: expected error for missing file, got nil")
	}
}

// ---------------------------------------------------------------------------
// Interface compliance
// ---------------------------------------------------------------------------

func TestInterfaceCompliance(t *testing.T) {
	var _ Source = (*LocalSource)(nil)
	var _ Source = (*BuiltinSource)(nil)
}
