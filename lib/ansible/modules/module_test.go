package modules

import (
	"context"
	"testing"

	"vraxel.io/vraxel/lib/ansible"
	"vraxel.io/vraxel/lib/ansible/modules/internal"
	"vraxel.io/vraxel/lib/ansible/variable"
)

func TestRegisterAndFindModule(t *testing.T) {
	cleanup := internal.SaveAndResetRegistry()
	t.Cleanup(cleanup)

	RegisterModule("test_mod", func(ctx context.Context, opts ExecOptions) (string, string, error) {
		return "ok", "", nil
	})

	fn := FindModule("test_mod")
	if fn == nil {
		t.Fatal("expected to find registered module 'test_mod', got nil")
	}

	stdout, stderr, err := fn(context.Background(), ExecOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stdout != "ok" {
		t.Errorf("expected stdout 'ok', got %q", stdout)
	}
	if stderr != "" {
		t.Errorf("expected empty stderr, got %q", stderr)
	}
}

func TestFindModule_NotFound(t *testing.T) {
	cleanup := internal.SaveAndResetRegistry()
	t.Cleanup(cleanup)

	fn := FindModule("nonexistent")
	if fn != nil {
		t.Fatal("expected nil for unregistered module, got non-nil")
	}
}

func TestIsModule(t *testing.T) {
	cleanup := internal.SaveAndResetRegistry()
	t.Cleanup(cleanup)

	RegisterModule("exists_mod", func(ctx context.Context, opts ExecOptions) (string, string, error) {
		return "", "", nil
	})

	if !IsModule("exists_mod") {
		t.Error("expected IsModule('exists_mod') to return true")
	}
	if IsModule("no_such_mod") {
		t.Error("expected IsModule('no_such_mod') to return false")
	}
}

func TestListModules(t *testing.T) {
	cleanup := internal.SaveAndResetRegistry()
	t.Cleanup(cleanup)

	RegisterModule("zeta", func(ctx context.Context, opts ExecOptions) (string, string, error) {
		return "", "", nil
	})
	RegisterModule("alpha", func(ctx context.Context, opts ExecOptions) (string, string, error) {
		return "", "", nil
	})
	RegisterModule("middle", func(ctx context.Context, opts ExecOptions) (string, string, error) {
		return "", "", nil
	})

	names := ListModules()
	expected := []string{"alpha", "middle", "zeta"}

	if len(names) != len(expected) {
		t.Fatalf("expected %d modules, got %d: %v", len(expected), len(names), names)
	}
	for i, name := range names {
		if name != expected[i] {
			t.Errorf("expected names[%d] = %q, got %q", i, expected[i], name)
		}
	}
}

func TestListModules_Empty(t *testing.T) {
	cleanup := internal.SaveAndResetRegistry()
	t.Cleanup(cleanup)

	names := ListModules()
	if len(names) != 0 {
		t.Errorf("expected empty list, got %v", names)
	}
}

func TestRegisterModule_Overwrite(t *testing.T) {
	cleanup := internal.SaveAndResetRegistry()
	t.Cleanup(cleanup)

	RegisterModule("dup", func(ctx context.Context, opts ExecOptions) (string, string, error) {
		return "first", "", nil
	})
	RegisterModule("dup", func(ctx context.Context, opts ExecOptions) (string, string, error) {
		return "second", "", nil
	})

	fn := FindModule("dup")
	if fn == nil {
		t.Fatal("expected to find module 'dup'")
	}
	stdout, _, _ := fn(context.Background(), ExecOptions{})
	if stdout != "second" {
		t.Errorf("expected last registration to win, got stdout %q", stdout)
	}
}

func TestExecOptions_GetAllVariables(t *testing.T) {
	inv := ansible.Inventory{
		Hosts: map[string]map[string]any{
			"host1": {"key1": "val1"},
		},
	}
	v := variable.New(inv)

	opts := ExecOptions{
		Host:     "host1",
		Variable: v,
	}

	vars := opts.GetAllVariables()
	if vars == nil {
		t.Fatal("expected non-nil variables map")
	}
	if vars["key1"] != "val1" {
		t.Errorf("expected key1='val1', got %v", vars["key1"])
	}
}

func TestExecOptions_GetAllVariables_UnknownHost(t *testing.T) {
	inv := ansible.Inventory{
		Hosts: map[string]map[string]any{
			"host1": {"key1": "val1"},
		},
	}
	v := variable.New(inv)

	opts := ExecOptions{
		Host:     "unknown_host",
		Variable: v,
	}

	vars := opts.GetAllVariables()
	if vars == nil {
		t.Fatal("expected non-nil variables map for unknown host")
	}
	if len(vars) != 0 {
		t.Errorf("expected empty map for unknown host, got %v", vars)
	}
}
