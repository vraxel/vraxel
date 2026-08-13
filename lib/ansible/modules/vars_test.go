package modules

import (
	"context"
	"fmt"
	"testing"

	"vraxel.io/vraxel/lib/ansible/modules/add_hostvars"
	"vraxel.io/vraxel/lib/ansible/modules/include_vars"
	"vraxel.io/vraxel/lib/ansible/modules/set_fact"
	"vraxel.io/vraxel/lib/ansible/modules/setup"
)

// ========================================================================
// setup module tests
// ========================================================================

func TestModuleSetup_GathersFacts(t *testing.T) {
	v := newTestVariable("node1")
	conn := &mockGatherFactsConnector{
		hostInfoFn: func(_ context.Context) (map[string]any, error) {
			return map[string]any{
				"os": map[string]any{
					"family":   "debian",
					"hostname": "node1-host",
				},
				"arch": "amd64",
			}, nil
		},
	}

	stdout, stderr, err := setup.ModuleSetup(context.Background(), ExecOptions{
		Host:      "node1",
		Variable:  v,
		Connector: conn,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stdout != "gathered facts" {
		t.Errorf("expected stdout 'gathered facts', got %q", stdout)
	}
	if stderr != "" {
		t.Errorf("expected empty stderr, got %q", stderr)
	}

	remote := getHostRemoteVars(v, "node1")
	if remote["arch"] != "amd64" {
		t.Errorf("expected arch=amd64, got %v", remote["arch"])
	}
	osInfo, ok := remote["os"].(map[string]any)
	if !ok {
		t.Fatalf("expected os to be map, got %T", remote["os"])
	}
	if osInfo["family"] != "debian" {
		t.Errorf("expected os.family=debian, got %v", osInfo["family"])
	}
}

func TestModuleSetup_NoGatherFacts(t *testing.T) {
	v := newTestVariable("node1")
	// mockConnector does not implement GatherFacts.
	conn := &mockConnector{}

	_, _, err := setup.ModuleSetup(context.Background(), ExecOptions{
		Host:      "node1",
		Variable:  v,
		Connector: conn,
	})
	if err == nil {
		t.Fatal("expected error when connector does not support gather_facts")
	}
	if err.Error() != "setup: connector does not support gather_facts" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestModuleSetup_HostInfoError(t *testing.T) {
	v := newTestVariable("node1")
	conn := &mockGatherFactsConnector{
		hostInfoFn: func(_ context.Context) (map[string]any, error) {
			return nil, fmt.Errorf("connection refused")
		},
	}

	_, _, err := setup.ModuleSetup(context.Background(), ExecOptions{
		Host:      "node1",
		Variable:  v,
		Connector: conn,
	})
	if err == nil {
		t.Fatal("expected error from HostInfo failure")
	}
}

// ========================================================================
// set_fact module tests
// ========================================================================

func TestModuleSetFact_SingleFact(t *testing.T) {
	v := newTestVariable("host1")

	stdout, stderr, err := set_fact.ModuleSetFact(context.Background(), ExecOptions{
		Host:     "host1",
		Variable: v,
		Args:     map[string]any{"app_version": "1.0.0"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stdout != "facts set" {
		t.Errorf("expected stdout 'facts set', got %q", stdout)
	}
	if stderr != "" {
		t.Errorf("expected empty stderr, got %q", stderr)
	}

	vars := getHostRuntimeVars(v, "host1")
	if vars["app_version"] != "1.0.0" {
		t.Errorf("expected app_version=1.0.0, got %v", vars["app_version"])
	}
}

func TestModuleSetFact_MultipleFacts(t *testing.T) {
	v := newTestVariable("host1")

	_, _, err := set_fact.ModuleSetFact(context.Background(), ExecOptions{
		Host:     "host1",
		Variable: v,
		Args: map[string]any{
			"db_host": "localhost",
			"db_port": 5432,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	vars := getHostRuntimeVars(v, "host1")
	if vars["db_host"] != "localhost" {
		t.Errorf("expected db_host=localhost, got %v", vars["db_host"])
	}
	if vars["db_port"] != 5432 {
		t.Errorf("expected db_port=5432, got %v", vars["db_port"])
	}
}

func TestModuleSetFact_EmptyArgs(t *testing.T) {
	v := newTestVariable("host1")

	_, _, err := set_fact.ModuleSetFact(context.Background(), ExecOptions{
		Host:     "host1",
		Variable: v,
		Args:     map[string]any{},
	})
	if err == nil {
		t.Fatal("expected error for empty args")
	}
}

func TestModuleSetFact_NilArgs(t *testing.T) {
	v := newTestVariable("host1")

	_, _, err := set_fact.ModuleSetFact(context.Background(), ExecOptions{
		Host:     "host1",
		Variable: v,
		Args:     nil,
	})
	if err == nil {
		t.Fatal("expected error for nil args")
	}
}

func TestModuleSetFact_OverwriteExisting(t *testing.T) {
	v := newTestVariable("host1")

	// Set initial value.
	_, _, _ = set_fact.ModuleSetFact(context.Background(), ExecOptions{
		Host:     "host1",
		Variable: v,
		Args:     map[string]any{"key": "old"},
	})

	// Overwrite.
	_, _, _ = set_fact.ModuleSetFact(context.Background(), ExecOptions{
		Host:     "host1",
		Variable: v,
		Args:     map[string]any{"key": "new"},
	})

	vars := getHostRuntimeVars(v, "host1")
	if vars["key"] != "new" {
		t.Errorf("expected key=new, got %v", vars["key"])
	}
}

// ========================================================================
// include_vars module tests
// ========================================================================

func TestModuleIncludeVars_FromFile(t *testing.T) {
	v := newTestVariable("host1")
	src := &mockSource{
		files: map[string][]byte{
			"vars/main.yml": []byte("db_host: postgres\ndb_port: 5432\n"),
		},
	}

	stdout, stderr, err := include_vars.ModuleIncludeVars(context.Background(), ExecOptions{
		Host:     "host1",
		Variable: v,
		Source:   src,
		Args:     map[string]any{"file": "vars/main.yml"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stdout != "vars included" {
		t.Errorf("expected stdout 'vars included', got %q", stdout)
	}
	if stderr != "" {
		t.Errorf("expected empty stderr, got %q", stderr)
	}

	vars := getHostRuntimeVars(v, "host1")
	if vars["db_host"] != "postgres" {
		t.Errorf("expected db_host=postgres, got %v", vars["db_host"])
	}
	if vars["db_port"] != 5432 {
		t.Errorf("expected db_port=5432, got %v", vars["db_port"])
	}
}

func TestModuleIncludeVars_ShorthandSyntax(t *testing.T) {
	v := newTestVariable("host1")
	src := &mockSource{
		files: map[string][]byte{
			"vars/config.yaml": []byte("enabled: true\n"),
		},
	}

	_, _, err := include_vars.ModuleIncludeVars(context.Background(), ExecOptions{
		Host:     "host1",
		Variable: v,
		Source:   src,
		Args:     map[string]any{"include_vars": "vars/config.yaml"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	vars := getHostRuntimeVars(v, "host1")
	if vars["enabled"] != true {
		t.Errorf("expected enabled=true, got %v", vars["enabled"])
	}
}

func TestModuleIncludeVars_NoFile(t *testing.T) {
	v := newTestVariable("host1")

	_, _, err := include_vars.ModuleIncludeVars(context.Background(), ExecOptions{
		Host:     "host1",
		Variable: v,
		Args:     map[string]any{},
	})
	if err == nil {
		t.Fatal("expected error for missing file argument")
	}
}

func TestModuleIncludeVars_UnsupportedExtension(t *testing.T) {
	v := newTestVariable("host1")

	_, _, err := include_vars.ModuleIncludeVars(context.Background(), ExecOptions{
		Host:     "host1",
		Variable: v,
		Args:     map[string]any{"file": "config.json"},
	})
	if err == nil {
		t.Fatal("expected error for unsupported file extension")
	}
}

func TestModuleIncludeVars_FileNotFound(t *testing.T) {
	v := newTestVariable("host1")
	src := &mockSource{files: map[string][]byte{}}

	_, _, err := include_vars.ModuleIncludeVars(context.Background(), ExecOptions{
		Host:     "host1",
		Variable: v,
		Source:   src,
		Args:     map[string]any{"file": "vars/missing.yaml"},
	})
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestModuleIncludeVars_InvalidYAML(t *testing.T) {
	v := newTestVariable("host1")
	src := &mockSource{
		files: map[string][]byte{
			"vars/bad.yaml": []byte(":\n  :\n    - [invalid"),
		},
	}

	_, _, err := include_vars.ModuleIncludeVars(context.Background(), ExecOptions{
		Host:     "host1",
		Variable: v,
		Source:   src,
		Args:     map[string]any{"file": "vars/bad.yaml"},
	})
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestModuleIncludeVars_NestedVars(t *testing.T) {
	v := newTestVariable("host1")
	src := &mockSource{
		files: map[string][]byte{
			"vars/nested.yml": []byte("database:\n  host: localhost\n  port: 5432\n"),
		},
	}

	_, _, err := include_vars.ModuleIncludeVars(context.Background(), ExecOptions{
		Host:     "host1",
		Variable: v,
		Source:   src,
		Args:     map[string]any{"file": "vars/nested.yml"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	vars := getHostRuntimeVars(v, "host1")
	db, ok := vars["database"].(map[string]any)
	if !ok {
		t.Fatalf("expected database to be map, got %T", vars["database"])
	}
	if db["host"] != "localhost" {
		t.Errorf("expected database.host=localhost, got %v", db["host"])
	}
	if db["port"] != 5432 {
		t.Errorf("expected database.port=5432, got %v", db["port"])
	}
}

// ========================================================================
// add_hostvars module tests
// ========================================================================

func TestModuleAddHostvars_CurrentHost(t *testing.T) {
	v := newTestVariable("host1")

	stdout, stderr, err := add_hostvars.ModuleAddHostvars(context.Background(), ExecOptions{
		Host:     "host1",
		Variable: v,
		Args: map[string]any{
			"custom_var": "value1",
			"count":      42,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stdout != "hostvars added" {
		t.Errorf("expected stdout 'hostvars added', got %q", stdout)
	}
	if stderr != "" {
		t.Errorf("expected empty stderr, got %q", stderr)
	}

	vars := getHostRuntimeVars(v, "host1")
	if vars["custom_var"] != "value1" {
		t.Errorf("expected custom_var=value1, got %v", vars["custom_var"])
	}
	if vars["count"] != 42 {
		t.Errorf("expected count=42, got %v", vars["count"])
	}
}

func TestModuleAddHostvars_SpecificHost(t *testing.T) {
	v := newTestVariable("host1", "host2")

	_, _, err := add_hostvars.ModuleAddHostvars(context.Background(), ExecOptions{
		Host:     "host1",
		Variable: v,
		Args: map[string]any{
			"host":    "host2",
			"role":    "worker",
			"enabled": true,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// host2 should have the vars.
	vars := getHostRuntimeVars(v, "host2")
	if vars["role"] != "worker" {
		t.Errorf("expected role=worker on host2, got %v", vars["role"])
	}
	if vars["enabled"] != true {
		t.Errorf("expected enabled=true on host2, got %v", vars["enabled"])
	}

	// host1 should NOT have the vars (they were directed at host2).
	host1Vars := getHostRuntimeVars(v, "host1")
	if _, ok := host1Vars["role"]; ok {
		t.Errorf("expected host1 to not have 'role' var, but it does")
	}
}

func TestModuleAddHostvars_NoVars(t *testing.T) {
	v := newTestVariable("host1")

	_, _, err := add_hostvars.ModuleAddHostvars(context.Background(), ExecOptions{
		Host:     "host1",
		Variable: v,
		Args:     map[string]any{"host": "host1"},
	})
	if err == nil {
		t.Fatal("expected error when only 'host' key is provided (no actual vars)")
	}
}

func TestModuleAddHostvars_EmptyArgs(t *testing.T) {
	v := newTestVariable("host1")

	_, _, err := add_hostvars.ModuleAddHostvars(context.Background(), ExecOptions{
		Host:     "host1",
		Variable: v,
		Args:     map[string]any{},
	})
	if err == nil {
		t.Fatal("expected error for empty args")
	}
}

func TestModuleAddHostvars_DefaultsToCurrentHost(t *testing.T) {
	v := newTestVariable("host1")

	_, _, err := add_hostvars.ModuleAddHostvars(context.Background(), ExecOptions{
		Host:     "host1",
		Variable: v,
		Args:     map[string]any{"key": "value"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	vars := getHostRuntimeVars(v, "host1")
	if vars["key"] != "value" {
		t.Errorf("expected key=value, got %v", vars["key"])
	}
}

func TestModuleAddHostvars_HostKeyExcluded(t *testing.T) {
	v := newTestVariable("host1", "target")

	_, _, err := add_hostvars.ModuleAddHostvars(context.Background(), ExecOptions{
		Host:     "host1",
		Variable: v,
		Args: map[string]any{
			"host":  "target",
			"myvar": "myval",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The "host" key should NOT appear in target's vars.
	vars := getHostRuntimeVars(v, "target")
	if _, ok := vars["host"]; ok {
		t.Error("expected 'host' key to be excluded from target vars")
	}
	if vars["myvar"] != "myval" {
		t.Errorf("expected myvar=myval, got %v", vars["myvar"])
	}
}
