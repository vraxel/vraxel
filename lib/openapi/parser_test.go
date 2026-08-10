package openapi

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// Locks the recursion contract: the parser walks one level deep into
// container directories whose own files have no +openapi: annotations
// (e.g. shared/), so helper packages nested under non-resource parents
// (shared/paasdeploy/) are auto-discovered without per-name special
// cases. Top-level directories that DO have annotations parse normally.
func TestParserGenericRecursion(t *testing.T) {
	root := t.TempDir()

	// Sibling 1: top-level group, parses directly.
	mustWrite(t, filepath.Join(root, "foo", "doc.go"), `// +openapi:groupName=foo
// +openapi:groupVersion=v1
package foo
`)
	mustWrite(t, filepath.Join(root, "foo", "types.go"), `package foo

// +openapi:schema
type FooThing struct {
	ID string `+"`json:\"id\"`"+`
}
`)

	// Sibling 2: container dir with no annotated types directly, but
	// holds a nested annotated child. Recursion must find it.
	mustWrite(t, filepath.Join(root, "bar", "doc.go"), `// container only -- no group annotations
package bar
`)
	mustWrite(t, filepath.Join(root, "bar", "baz", "doc.go"), `// +openapi:groupName=baz
// +openapi:groupVersion=v1
package baz
`)
	mustWrite(t, filepath.Join(root, "bar", "baz", "types.go"), `package baz

// +openapi:schema
type BazThing struct {
	Name string `+"`json:\"name\"`"+`
}
`)

	// Sibling 3: empty directory with no annotated descendants. Must
	// not crash and must not produce a phantom group.
	mustWrite(t, filepath.Join(root, "empty", "doc.go"), `// not annotated
package empty
`)

	groups, err := NewParser(root).Parse()
	if err != nil {
		t.Fatalf("parser failed: %v", err)
	}

	names := make([]string, 0, len(groups))
	for _, g := range groups {
		names = append(names, g.GroupName)
	}
	sort.Strings(names)

	want := []string{"baz", "foo"}
	if len(names) != len(want) {
		t.Fatalf("group count: want %d %v, got %d %v", len(want), want, len(names), names)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Errorf("group[%d]: want %q, got %q", i, want[i], names[i])
		}
	}

	for _, g := range groups {
		if len(g.Types) == 0 {
			t.Errorf("group %q: no types parsed", g.GroupName)
		}
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
