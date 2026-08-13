package planner_test

import (
	"reflect"
	"strings"
	"testing"

	"vraxel.io/vraxel/lib/ansible"
	"vraxel.io/vraxel/lib/ansible/converter"
	"vraxel.io/vraxel/lib/ansible/planner"
)

// parsePlays parses playbook YAML via the real converter so serial values
// carry their real runtime types (YAML scalars arrive as strings).
func parsePlays(t *testing.T, yml string) []ansible.Play {
	t.Helper()
	pb, err := converter.ParsePlaybook([]byte(yml))
	if err != nil {
		t.Fatalf("parse playbook: %v", err)
	}
	return pb.Play
}

// groupInventory builds an inventory whose Hosts map contains every host
// referenced by the given groups.
func groupInventory(groups map[string]ansible.InventoryGroup) ansible.Inventory {
	hosts := make(map[string]map[string]any)
	for _, g := range groups {
		for _, h := range g.Hosts {
			hosts[h] = map[string]any{}
		}
	}
	return ansible.Inventory{Hosts: hosts, Groups: groups}
}

func TestBuildPlayPlan_SerialOne(t *testing.T) {
	// kafka/restart.yml shape: one play, serial: 1, three brokers.
	plays := parsePlays(t, `
- name: restart kafka
  hosts: kafka
  serial: 1
`)
	inv := groupInventory(map[string]ansible.InventoryGroup{
		"kafka": {Hosts: []string{"k1", "k2", "k3"}},
	})

	plan, err := planner.BuildPlayPlan(plays, inv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []planner.PlayPlan{
		{PlayIndex: 0, BatchIndex: 0, Hosts: []string{"k1"}, RunOnceTarget: "k1"},
		{PlayIndex: 0, BatchIndex: 1, Hosts: []string{"k2"}, RunOnceTarget: "k2"},
		{PlayIndex: 0, BatchIndex: 2, Hosts: []string{"k3"}, RunOnceTarget: "k3"},
	}
	if !reflect.DeepEqual(plan, want) {
		t.Errorf("plan mismatch:\n got: %+v\nwant: %+v", plan, want)
	}
}

func TestBuildPlayPlan_SerialPercent(t *testing.T) {
	plays := parsePlays(t, `
- name: rolling
  hosts: web
  serial: 40%
`)
	inv := groupInventory(map[string]ansible.InventoryGroup{
		"web": {Hosts: []string{"h1", "h2", "h3", "h4", "h5"}},
	})

	plan, err := planner.BuildPlayPlan(plays, inv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// ceil(5*40%) = 2; last value repeats: 2, 2, 1.
	want := []planner.PlayPlan{
		{PlayIndex: 0, BatchIndex: 0, Hosts: []string{"h1", "h2"}, RunOnceTarget: "h1"},
		{PlayIndex: 0, BatchIndex: 1, Hosts: []string{"h3", "h4"}, RunOnceTarget: "h3"},
		{PlayIndex: 0, BatchIndex: 2, Hosts: []string{"h5"}, RunOnceTarget: "h5"},
	}
	if !reflect.DeepEqual(plan, want) {
		t.Errorf("plan mismatch:\n got: %+v\nwant: %+v", plan, want)
	}
}

func TestBuildPlayPlan_SerialListLastValueRepeats(t *testing.T) {
	plays := parsePlays(t, `
- name: canary then rest
  hosts: web
  serial:
    - 1
    - 50%
`)
	inv := groupInventory(map[string]ansible.InventoryGroup{
		"web": {Hosts: []string{"h1", "h2", "h3", "h4", "h5"}},
	})

	plan, err := planner.BuildPlayPlan(plays, inv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Sizes 1, ceil(5*50%)=3, then 3 repeats to cover the remainder: 1, 3, 1.
	want := []planner.PlayPlan{
		{PlayIndex: 0, BatchIndex: 0, Hosts: []string{"h1"}, RunOnceTarget: "h1"},
		{PlayIndex: 0, BatchIndex: 1, Hosts: []string{"h2", "h3", "h4"}, RunOnceTarget: "h2"},
		{PlayIndex: 0, BatchIndex: 2, Hosts: []string{"h5"}, RunOnceTarget: "h5"},
	}
	if !reflect.DeepEqual(plan, want) {
		t.Errorf("plan mismatch:\n got: %+v\nwant: %+v", plan, want)
	}
}

func TestBuildPlayPlan_AddNodesGolden(t *testing.T) {
	// kubernetes/add-nodes.yml shape: multiple plays across overlapping
	// groups, two of them serial: 1. Expected sequence hand-derived.
	plays := parsePlays(t, `
- name: Prepare new nodes
  hosts: new_all

- name: Join new master nodes
  hosts: new_masters
  serial: 1

- name: Join new worker nodes
  hosts: new_workers
  serial: 1

- name: Verify cluster
  hosts: verify_master

- name: Rerender haproxy on existing nodes
  hosts: existing_all
`)
	inv := groupInventory(map[string]ansible.InventoryGroup{
		"new_all":       {Hosts: []string{"n1", "n2", "n3"}},
		"new_masters":   {Hosts: []string{"n1"}},
		"new_workers":   {Hosts: []string{"n2", "n3"}},
		"verify_master": {Hosts: []string{"e1"}},
		"existing_all":  {Hosts: []string{"e1", "e2"}},
	})

	plan, err := planner.BuildPlayPlan(plays, inv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []planner.PlayPlan{
		{PlayIndex: 0, BatchIndex: 0, Hosts: []string{"n1", "n2", "n3"}, RunOnceTarget: "n1"},
		{PlayIndex: 1, BatchIndex: 0, Hosts: []string{"n1"}, RunOnceTarget: "n1"},
		{PlayIndex: 2, BatchIndex: 0, Hosts: []string{"n2"}, RunOnceTarget: "n2"},
		{PlayIndex: 2, BatchIndex: 1, Hosts: []string{"n3"}, RunOnceTarget: "n3"},
		{PlayIndex: 3, BatchIndex: 0, Hosts: []string{"e1"}, RunOnceTarget: "e1"},
		{PlayIndex: 4, BatchIndex: 0, Hosts: []string{"e1", "e2"}, RunOnceTarget: "e1"},
	}
	if !reflect.DeepEqual(plan, want) {
		t.Errorf("plan mismatch:\n got: %+v\nwant: %+v", plan, want)
	}
}

func TestBuildPlayPlan_MasterIndexSyntax(t *testing.T) {
	// kubernetes/install.yml shape: hosts: "master[0]".
	plays := parsePlays(t, `
- name: init first master
  hosts: "master[0]"
`)
	inv := groupInventory(map[string]ansible.InventoryGroup{
		"master": {Hosts: []string{"m1", "m2", "m3"}},
	})

	plan, err := planner.BuildPlayPlan(plays, inv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []planner.PlayPlan{
		{PlayIndex: 0, BatchIndex: 0, Hosts: []string{"m1"}, RunOnceTarget: "m1"},
	}
	if !reflect.DeepEqual(plan, want) {
		t.Errorf("plan mismatch:\n got: %+v\nwant: %+v", plan, want)
	}
}

func TestBuildPlayPlan_MasterIndexOutOfRange(t *testing.T) {
	plays := parsePlays(t, `
- name: out of range
  hosts: "master[5]"
`)
	inv := groupInventory(map[string]ansible.InventoryGroup{
		"master": {Hosts: []string{"m1"}},
	})

	plan, err := planner.BuildPlayPlan(plays, inv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plan) != 0 {
		t.Errorf("expected empty plan for out-of-range index, got %+v", plan)
	}
}

func TestBuildPlayPlan_RunOnceTargetIsBatchLeader(t *testing.T) {
	plays := parsePlays(t, `
- name: run once per batch
  hosts: web
  serial: 2
`)
	inv := groupInventory(map[string]ansible.InventoryGroup{
		"web": {Hosts: []string{"h1", "h2", "h3"}},
	})

	plan, err := planner.BuildPlayPlan(plays, inv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plan) != 2 {
		t.Fatalf("expected 2 batches, got %d: %+v", len(plan), plan)
	}
	// run_once is per batch in this engine (block.go takes hosts[:1] of the
	// batch), so each batch has its own leader.
	if plan[0].RunOnceTarget != "h1" || plan[1].RunOnceTarget != "h3" {
		t.Errorf("expected batch leaders h1 and h3, got %q and %q",
			plan[0].RunOnceTarget, plan[1].RunOnceTarget)
	}
}

func TestBuildPlayPlan_EmptyAndUnknownGroups(t *testing.T) {
	// Unknown group, empty group, and empty group with an invalid serial:
	// all yield no plan entries and no error, matching the executor's
	// skip-before-serial-validation behavior. The trailing play proves
	// planning continues past skipped plays.
	plays := parsePlays(t, `
- name: unknown group
  hosts: nonexistent

- name: empty group
  hosts: empty_group

- name: empty group with bad serial
  hosts: empty_group
  serial: notanumber

- name: real play
  hosts: web
`)
	inv := groupInventory(map[string]ansible.InventoryGroup{
		"empty_group": {Hosts: []string{}},
		"web":         {Hosts: []string{"h1"}},
	})

	plan, err := planner.BuildPlayPlan(plays, inv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []planner.PlayPlan{
		{PlayIndex: 3, BatchIndex: 0, Hosts: []string{"h1"}, RunOnceTarget: "h1"},
	}
	if !reflect.DeepEqual(plan, want) {
		t.Errorf("plan mismatch:\n got: %+v\nwant: %+v", plan, want)
	}
}

func TestBuildPlayPlan_InvalidSerial(t *testing.T) {
	plays := parsePlays(t, `
- name: bad serial
  hosts: web
  serial: notanumber
`)
	inv := groupInventory(map[string]ansible.InventoryGroup{
		"web": {Hosts: []string{"h1"}},
	})

	_, err := planner.BuildPlayPlan(plays, inv)
	if err == nil {
		t.Fatal("expected error for invalid serial with matching hosts")
	}
	if !strings.Contains(err.Error(), "serial batching") {
		t.Errorf("expected 'serial batching' in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "bad serial") {
		t.Errorf("expected play name in error, got: %v", err)
	}
}

func TestBuildPlayPlan_SerialEntriesExceedHosts(t *testing.T) {
	// More explicit serial entries than hosts: GroupHostBySerial yields a
	// trailing empty batch; the plan preserves it (empty RunOnceTarget).
	plays := parsePlays(t, `
- name: too many batches
  hosts: web
  serial:
    - 1
    - 1
    - 1
`)
	inv := groupInventory(map[string]ansible.InventoryGroup{
		"web": {Hosts: []string{"h1", "h2"}},
	})

	plan, err := planner.BuildPlayPlan(plays, inv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []planner.PlayPlan{
		{PlayIndex: 0, BatchIndex: 0, Hosts: []string{"h1"}, RunOnceTarget: "h1"},
		{PlayIndex: 0, BatchIndex: 1, Hosts: []string{"h2"}, RunOnceTarget: "h2"},
		{PlayIndex: 0, BatchIndex: 2, Hosts: []string{}, RunOnceTarget: ""},
	}
	if !reflect.DeepEqual(plan, want) {
		t.Errorf("plan mismatch:\n got: %+v\nwant: %+v", plan, want)
	}
}
