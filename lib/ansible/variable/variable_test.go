package variable

import (
	"sort"
	"testing"

	"gopkg.in/yaml.v3"

	"vraxel.io/vraxel/lib/ansible"
)

// ========================================================================
// CombineVariables tests
// ========================================================================

func TestCombineVariables(t *testing.T) {
	m1 := map[string]any{
		"a": "1",
		"b": map[string]any{
			"x": "10",
			"y": "20",
		},
	}
	m2 := map[string]any{
		"a": "2",
		"b": map[string]any{
			"y": "99",
			"z": "30",
		},
		"c": "3",
	}

	result := CombineVariables(m1, m2)

	// "a" should be overridden by m2.
	if result["a"] != "2" {
		t.Errorf("expected a=2, got %v", result["a"])
	}

	// "c" from m2 should be present.
	if result["c"] != "3" {
		t.Errorf("expected c=3, got %v", result["c"])
	}

	// Nested "b" should be deep-merged.
	b, ok := result["b"].(map[string]any)
	if !ok {
		t.Fatalf("expected b to be map, got %T", result["b"])
	}
	if b["x"] != "10" {
		t.Errorf("expected b.x=10, got %v", b["x"])
	}
	if b["y"] != "99" {
		t.Errorf("expected b.y=99 (overridden), got %v", b["y"])
	}
	if b["z"] != "30" {
		t.Errorf("expected b.z=30, got %v", b["z"])
	}
}

func TestCombineVariables_SliceOverride(t *testing.T) {
	m1 := map[string]any{
		"list": []any{"a", "b"},
	}
	m2 := map[string]any{
		"list": []any{"x", "y", "z"},
	}

	result := CombineVariables(m1, m2)

	list, ok := result["list"].([]any)
	if !ok {
		t.Fatalf("expected list to be []any, got %T", result["list"])
	}
	// Slices should be replaced, not appended.
	if len(list) != 3 {
		t.Errorf("expected 3 elements, got %d", len(list))
	}
	if list[0] != "x" || list[1] != "y" || list[2] != "z" {
		t.Errorf("expected [x y z], got %v", list)
	}
}

func TestCombineVariables_NilValues(t *testing.T) {
	m1 := map[string]any{"a": "1"}
	result := CombineVariables(m1, nil)
	if result["a"] != "1" {
		t.Errorf("expected a=1, got %v", result["a"])
	}

	result2 := CombineVariables(nil, m1)
	if result2["a"] != "1" {
		t.Errorf("expected a=1, got %v", result2["a"])
	}
}

// ========================================================================
// ConvertGroup tests
// ========================================================================

func TestConvertGroup_All(t *testing.T) {
	inv := ansible.Inventory{
		Hosts: map[string]map[string]any{
			"host1": {"ip": "1.1.1.1"},
			"host2": {"ip": "2.2.2.2"},
		},
	}

	groups := ConvertGroup(inv)

	allHosts := groups[keyGroupsAll]
	if len(allHosts) != 2 {
		t.Fatalf("expected 2 hosts in 'all', got %d", len(allHosts))
	}

	sort.Strings(allHosts)
	if allHosts[0] != "host1" || allHosts[1] != "host2" {
		t.Errorf("unexpected all hosts: %v", allHosts)
	}
}

func TestConvertGroup_Ungrouped(t *testing.T) {
	inv := ansible.Inventory{
		Hosts: map[string]map[string]any{
			"host1": {},
			"host2": {},
			"host3": {},
		},
		Groups: map[string]ansible.InventoryGroup{
			"webservers": {
				Hosts: []string{"host1"},
			},
		},
	}

	groups := ConvertGroup(inv)

	ungrouped := groups[keyUngrouped]
	sort.Strings(ungrouped)

	if len(ungrouped) != 2 {
		t.Fatalf("expected 2 ungrouped hosts, got %d: %v", len(ungrouped), ungrouped)
	}
	if ungrouped[0] != "host2" || ungrouped[1] != "host3" {
		t.Errorf("unexpected ungrouped hosts: %v", ungrouped)
	}
}

func TestConvertGroup_NestedGroups(t *testing.T) {
	inv := ansible.Inventory{
		Hosts: map[string]map[string]any{
			"host1": {},
			"host2": {},
			"host3": {},
		},
		Groups: map[string]ansible.InventoryGroup{
			"webservers": {
				Hosts: []string{"host1"},
			},
			"dbservers": {
				Hosts: []string{"host2"},
			},
			"production": {
				Groups: []string{"webservers", "dbservers"},
				Hosts:  []string{"host3"},
			},
		},
	}

	groups := ConvertGroup(inv)

	prod := groups["production"]
	sort.Strings(prod)

	if len(prod) != 3 {
		t.Fatalf("expected 3 hosts in production, got %d: %v", len(prod), prod)
	}
	if prod[0] != "host1" || prod[1] != "host2" || prod[2] != "host3" {
		t.Errorf("unexpected production hosts: %v", prod)
	}
}

func TestConvertGroup_CycleDetection(t *testing.T) {
	inv := ansible.Inventory{
		Hosts: map[string]map[string]any{
			"host1": {},
		},
		Groups: map[string]ansible.InventoryGroup{
			"groupA": {
				Groups: []string{"groupB"},
				Hosts:  []string{"host1"},
			},
			"groupB": {
				Groups: []string{"groupA"},
			},
		},
	}

	// Should not panic or infinite loop.
	groups := ConvertGroup(inv)

	// groupA should still have host1.
	if len(groups["groupA"]) == 0 {
		t.Error("expected groupA to have hosts despite cycle")
	}
}

// ========================================================================
// Variable / GetAllVariable tests
// ========================================================================

func TestGetAllVariable_Priority(t *testing.T) {
	inv := ansible.Inventory{
		Hosts: map[string]map[string]any{
			"node1": {"role": "host-level", "host_only": "yes"},
		},
		Vars: map[string]any{
			"role":     "inv-level",
			"inv_only": "yes",
		},
		Groups: map[string]ansible.InventoryGroup{
			"workers": {
				Hosts: []string{"node1"},
				Vars: map[string]any{
					"role":       "group-level",
					"group_only": "yes",
				},
			},
		},
	}

	v := New(inv)

	// Set default, remote and runtime vars.
	v.Merge(MergeHostDefaultVars("node1", map[string]any{
		"role":         "default-level",
		"default_only": "yes",
	}))
	v.Merge(MergeRemoteVariable("node1", map[string]any{
		"role":        "remote-level",
		"remote_only": "yes",
	}))
	v.Merge(func(val *Value) {
		val.Hosts["node1"].RuntimeVars["role"] = "runtime-level"
		val.Hosts["node1"].RuntimeVars["runtime_only"] = "yes"
	})

	result := v.Get(GetAllVariable("node1"))
	vars, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", result)
	}

	// Runtime vars (set_fact, priority 6) should win for "role".
	if vars["role"] != "runtime-level" {
		t.Errorf("expected role=runtime-level (highest priority), got %v", vars["role"])
	}

	// All _only keys should be present (merged from each level).
	if vars["default_only"] != "yes" {
		t.Errorf("expected default_only=yes, got %v", vars["default_only"])
	}
	if vars["inv_only"] != "yes" {
		t.Errorf("expected inv_only=yes, got %v", vars["inv_only"])
	}
	if vars["group_only"] != "yes" {
		t.Errorf("expected group_only=yes, got %v", vars["group_only"])
	}
	if vars["remote_only"] != "yes" {
		t.Errorf("expected remote_only=yes, got %v", vars["remote_only"])
	}
	if vars["runtime_only"] != "yes" {
		t.Errorf("expected runtime_only=yes, got %v", vars["runtime_only"])
	}
	if vars["host_only"] != "yes" {
		t.Errorf("expected host_only=yes, got %v", vars["host_only"])
	}
}

func TestGetAllVariable_DefaultVarsLowestPriority(t *testing.T) {
	// Verify role defaults (DefaultVars) can be overridden by inventory host vars.
	// This is the exact scenario that broke repo_url.
	inv := ansible.Inventory{
		Hosts: map[string]map[string]any{
			"node1": {"repo_url": "http://lcp.io:8088/packages"},
		},
	}

	v := New(inv)

	// Simulate role defaults setting repo_url to "" (empty).
	v.Merge(MergeHostDefaultVars("node1", map[string]any{
		"repo_url": "",
	}))

	result := v.Get(GetAllVariable("node1"))
	vars := result.(map[string]any)

	// Inventory host vars should override role defaults.
	if vars["repo_url"] != "http://lcp.io:8088/packages" {
		t.Errorf("expected repo_url from inventory (http://lcp.io:8088/packages), got %q — role defaults should NOT override inventory", vars["repo_url"])
	}
}

func TestGetAllVariable_UnknownHost(t *testing.T) {
	inv := ansible.Inventory{
		Hosts: map[string]map[string]any{
			"host1": {},
		},
	}
	v := New(inv)

	result := v.Get(GetAllVariable("nonexistent"))
	vars, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", result)
	}
	if len(vars) != 0 {
		t.Errorf("expected empty map for unknown host, got %v", vars)
	}
}

func TestGetAllVariable_DefaultVars(t *testing.T) {
	inv := ansible.Inventory{
		Hosts: map[string]map[string]any{
			"myhost": {},
		},
	}
	v := New(inv)

	result := v.Get(GetAllVariable("myhost"))
	vars := result.(map[string]any)

	// Default hostname and inventory_name should be set.
	if vars[keyHostname] != "myhost" {
		t.Errorf("expected hostname=myhost, got %v", vars[keyHostname])
	}
	if vars[keyInventoryName] != "myhost" {
		t.Errorf("expected inventory_name=myhost, got %v", vars[keyInventoryName])
	}
}

// ========================================================================
// GetHostnames tests
// ========================================================================

func TestGetHostnames_Direct(t *testing.T) {
	inv := ansible.Inventory{
		Hosts: map[string]map[string]any{
			"web1": {},
			"web2": {},
		},
	}
	v := New(inv)

	result := v.Get(GetHostnames([]string{"web1"}))
	hosts, ok := result.([]string)
	if !ok {
		t.Fatalf("expected []string, got %T", result)
	}
	if len(hosts) != 1 || hosts[0] != "web1" {
		t.Errorf("expected [web1], got %v", hosts)
	}
}

func TestGetHostnames_FromGroup(t *testing.T) {
	inv := ansible.Inventory{
		Hosts: map[string]map[string]any{
			"web1": {},
			"web2": {},
			"db1":  {},
		},
		Groups: map[string]ansible.InventoryGroup{
			"webservers": {
				Hosts: []string{"web1", "web2"},
			},
		},
	}
	v := New(inv)

	result := v.Get(GetHostnames([]string{"webservers"}))
	hosts := result.([]string)
	sort.Strings(hosts)

	if len(hosts) != 2 {
		t.Fatalf("expected 2 hosts, got %d", len(hosts))
	}
	if hosts[0] != "web1" || hosts[1] != "web2" {
		t.Errorf("expected [web1 web2], got %v", hosts)
	}
}

func TestGetHostnames_AllGroup(t *testing.T) {
	inv := ansible.Inventory{
		Hosts: map[string]map[string]any{
			"host1": {},
			"host2": {},
		},
	}
	v := New(inv)

	result := v.Get(GetHostnames([]string{"all"}))
	hosts := result.([]string)
	sort.Strings(hosts)

	if len(hosts) != 2 {
		t.Fatalf("expected 2 hosts, got %d", len(hosts))
	}
}

func TestGetHostnames_IndexedAccess(t *testing.T) {
	inv := ansible.Inventory{
		Hosts: map[string]map[string]any{
			"web1": {},
			"web2": {},
		},
		Groups: map[string]ansible.InventoryGroup{
			"webservers": {
				Hosts: []string{"web1", "web2"},
			},
		},
	}
	v := New(inv)

	result := v.Get(GetHostnames([]string{"webservers[1]"}))
	hosts := result.([]string)

	if len(hosts) != 1 || hosts[0] != "web2" {
		t.Errorf("expected [web2], got %v", hosts)
	}
}

func TestGetHostnames_Empty(t *testing.T) {
	inv := ansible.Inventory{
		Hosts: map[string]map[string]any{},
	}
	v := New(inv)

	result := v.Get(GetHostnames([]string{}))
	hosts := result.([]string)
	if len(hosts) != 0 {
		t.Errorf("expected empty, got %v", hosts)
	}
}

// ========================================================================
// MergeRuntimeVariable tests
// ========================================================================

func TestMergeRuntimeVariable(t *testing.T) {
	inv := ansible.Inventory{
		Hosts: map[string]map[string]any{
			"host1": {},
		},
	}
	v := New(inv)

	// Build a YAML node representing {key1: val1, key2: val2}.
	yamlStr := "key1: val1\nkey2: val2"
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(yamlStr), &node); err != nil {
		t.Fatalf("failed to parse YAML: %v", err)
	}

	// node.Content is []*yaml.Node, convert to []yaml.Node for MergeRuntimeVariable.
	nodes := make([]yaml.Node, len(node.Content))
	for i, n := range node.Content {
		nodes[i] = *n
	}
	v.Merge(MergeRuntimeVariable(nodes, "host1"))

	result := v.Get(GetAllVariable("host1"))
	vars := result.(map[string]any)

	if vars["key1"] != "val1" {
		t.Errorf("expected key1=val1, got %v", vars["key1"])
	}
	if vars["key2"] != "val2" {
		t.Errorf("expected key2=val2, got %v", vars["key2"])
	}
}

func TestMergeRuntimeVariable_EmptyNodes(t *testing.T) {
	inv := ansible.Inventory{
		Hosts: map[string]map[string]any{
			"host1": {},
		},
	}
	v := New(inv)

	// Merging empty nodes should be a no-op.
	v.Merge(MergeRuntimeVariable(nil, "host1"))

	result := v.Get(func(val *Value) any {
		return val.Hosts["host1"].RuntimeVars
	})
	vars := result.(map[string]any)
	if len(vars) != 0 {
		t.Errorf("expected empty runtime vars, got %v", vars)
	}
}

// ========================================================================
// MergeRemoteVariable tests
// ========================================================================

func TestMergeRemoteVariable(t *testing.T) {
	inv := ansible.Inventory{
		Hosts: map[string]map[string]any{
			"host1": {},
		},
	}
	v := New(inv)

	remoteData := map[string]any{
		"os": map[string]any{
			"family":   "debian",
			"hostname": "myserver",
		},
	}
	v.Merge(MergeRemoteVariable("host1", remoteData))

	result := v.Get(func(val *Value) any {
		return val.Hosts["host1"].RemoteVars
	})
	vars := result.(map[string]any)

	osInfo, ok := vars["os"].(map[string]any)
	if !ok {
		t.Fatalf("expected os to be map, got %T", vars["os"])
	}
	if osInfo["family"] != "debian" {
		t.Errorf("expected family=debian, got %v", osInfo["family"])
	}
}

func TestMergeRemoteVariable_NonexistentHost(t *testing.T) {
	inv := ansible.Inventory{
		Hosts: map[string]map[string]any{
			"host1": {},
		},
	}
	v := New(inv)

	// Should not panic for a nonexistent host (just no-op).
	v.Merge(MergeRemoteVariable("nonexistent", map[string]any{"key": "val"}))
}

// ========================================================================
// MergeResultVariable tests
// ========================================================================

func TestMergeResultVariable(t *testing.T) {
	inv := ansible.Inventory{
		Hosts: map[string]map[string]any{
			"host1": {},
		},
	}
	v := New(inv)

	v.Merge(MergeResultVariable(map[string]any{"status": "ok"}))
	v.Merge(MergeResultVariable(map[string]any{"count": 42}))

	result := v.Get(GetResultVariable())
	vars := result.(map[string]any)

	if vars["status"] != "ok" {
		t.Errorf("expected status=ok, got %v", vars["status"])
	}
	if vars["count"] != 42 {
		t.Errorf("expected count=42, got %v", vars["count"])
	}
}

// ========================================================================
// GetHostMaxLength tests
// ========================================================================

func TestGetHostMaxLength(t *testing.T) {
	inv := ansible.Inventory{
		Hosts: map[string]map[string]any{
			"a":           {},
			"longername":  {},
			"longestname": {},
		},
	}
	v := New(inv)

	result := v.Get(GetHostMaxLength())
	maxLen := result.(int)

	if maxLen != len("longestname") {
		t.Errorf("expected %d, got %d", len("longestname"), maxLen)
	}
}

// ========================================================================
// Type extraction tests
// ========================================================================

func TestStringVar(t *testing.T) {
	vars := map[string]any{
		"name":  "hello",
		"count": 42,
		"nested": map[string]any{
			"key": "deep",
		},
	}

	if got := StringVar(vars, "name"); got != "hello" {
		t.Errorf("expected hello, got %s", got)
	}

	// Non-string values should be converted via Sprintf.
	if got := StringVar(vars, "count"); got != "42" {
		t.Errorf("expected 42, got %s", got)
	}

	// Nested access.
	if got := StringVar(vars, "nested", "key"); got != "deep" {
		t.Errorf("expected deep, got %s", got)
	}

	// Missing key returns "".
	if got := StringVar(vars, "missing"); got != "" {
		t.Errorf("expected empty string, got %s", got)
	}
}

func TestIntVar(t *testing.T) {
	vars := map[string]any{
		"count":     42,
		"float_val": 3.14,
		"str_val":   "99",
		"int64_val": int64(100),
	}

	if got := IntVar(vars, "count"); got != 42 {
		t.Errorf("expected 42, got %d", got)
	}
	if got := IntVar(vars, "float_val"); got != 3 {
		t.Errorf("expected 3, got %d", got)
	}
	if got := IntVar(vars, "str_val"); got != 99 {
		t.Errorf("expected 99, got %d", got)
	}
	if got := IntVar(vars, "int64_val"); got != 100 {
		t.Errorf("expected 100, got %d", got)
	}
	if got := IntVar(vars, "missing"); got != 0 {
		t.Errorf("expected 0, got %d", got)
	}
}

func TestBoolVar(t *testing.T) {
	vars := map[string]any{
		"enabled":  true,
		"disabled": false,
		"str_true": "true",
		"str_no":   "false",
	}

	if got := BoolVar(vars, "enabled"); got != true {
		t.Errorf("expected true, got %v", got)
	}
	if got := BoolVar(vars, "disabled"); got != false {
		t.Errorf("expected false, got %v", got)
	}
	if got := BoolVar(vars, "str_true"); got != true {
		t.Errorf("expected true, got %v", got)
	}
	if got := BoolVar(vars, "str_no"); got != false {
		t.Errorf("expected false, got %v", got)
	}
	if got := BoolVar(vars, "missing"); got != false {
		t.Errorf("expected false, got %v", got)
	}
}

func TestSliceVar(t *testing.T) {
	vars := map[string]any{
		"items": []any{"a", "b", "c"},
	}

	got := SliceVar(vars, "items")
	if len(got) != 3 {
		t.Fatalf("expected 3 items, got %d", len(got))
	}
	if got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Errorf("expected [a b c], got %v", got)
	}

	// Missing key.
	if got := SliceVar(vars, "missing"); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestPrintVar_Nested(t *testing.T) {
	vars := map[string]any{
		"os": map[string]any{
			"family": "debian",
			"release": map[string]any{
				"major": "12",
			},
		},
	}

	if got := PrintVar(vars, "os.family"); got != "debian" {
		t.Errorf("expected debian, got %v", got)
	}
	if got := PrintVar(vars, "os.release.major"); got != "12" {
		t.Errorf("expected 12, got %v", got)
	}
	if got := PrintVar(vars, "os.missing"); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
	if got := PrintVar(vars, "nonexistent.path"); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

// ========================================================================
// New() initialization tests
// ========================================================================

func TestNew_InitializesHosts(t *testing.T) {
	inv := ansible.Inventory{
		Hosts: map[string]map[string]any{
			"host1": {"ip": "1.1.1.1"},
			"host2": {"ip": "2.2.2.2"},
		},
	}

	v := New(inv)
	result := v.Get(func(val *Value) any {
		return len(val.Hosts)
	})

	if result.(int) != 2 {
		t.Errorf("expected 2 hosts, got %d", result)
	}
}

func TestNew_IncludesGroupReferencedHosts(t *testing.T) {
	inv := ansible.Inventory{
		Hosts: map[string]map[string]any{
			"host1": {},
		},
		Groups: map[string]ansible.InventoryGroup{
			"workers": {
				Hosts: []string{"host1", "host2"}, // host2 not in Hosts map
			},
		},
	}

	v := New(inv)
	result := v.Get(func(val *Value) any {
		_, exists := val.Hosts["host2"]
		return exists
	})

	if !result.(bool) {
		t.Error("expected host2 to be created from group reference")
	}
}

// ========================================================================
// Concurrency safety test
// ========================================================================

func TestVariable_ConcurrentAccess(t *testing.T) {
	inv := ansible.Inventory{
		Hosts: map[string]map[string]any{
			"host1": {},
		},
	}
	v := New(inv)

	done := make(chan struct{})
	for i := 0; i < 100; i++ {
		go func(i int) {
			defer func() { done <- struct{}{} }()
			if i%2 == 0 {
				v.Get(GetAllVariable("host1"))
			} else {
				v.Merge(func(val *Value) {
					val.Hosts["host1"].RuntimeVars["key"] = i
				})
			}
		}(i)
	}

	for i := 0; i < 100; i++ {
		<-done
	}
}

// ========================================================================
// GetResultVariable tests
// ========================================================================

func TestGetResultVariable_Empty(t *testing.T) {
	inv := ansible.Inventory{
		Hosts: map[string]map[string]any{
			"host1": {},
		},
	}
	v := New(inv)

	result := v.Get(GetResultVariable())
	vars := result.(map[string]any)
	if len(vars) != 0 {
		t.Errorf("expected empty result, got %v", vars)
	}
}

// ========================================================================
// deepMerge edge cases
// ========================================================================

func TestCombineVariables_DeepNested(t *testing.T) {
	m1 := map[string]any{
		"level1": map[string]any{
			"level2": map[string]any{
				"a": "from_m1",
				"b": "from_m1",
			},
		},
	}
	m2 := map[string]any{
		"level1": map[string]any{
			"level2": map[string]any{
				"b": "from_m2",
				"c": "from_m2",
			},
		},
	}

	result := CombineVariables(m1, m2)
	level1 := result["level1"].(map[string]any)
	level2 := level1["level2"].(map[string]any)

	if level2["a"] != "from_m1" {
		t.Errorf("expected a=from_m1, got %v", level2["a"])
	}
	if level2["b"] != "from_m2" {
		t.Errorf("expected b=from_m2, got %v", level2["b"])
	}
	if level2["c"] != "from_m2" {
		t.Errorf("expected c=from_m2, got %v", level2["c"])
	}
}
