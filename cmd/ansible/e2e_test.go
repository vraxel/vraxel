package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"vraxel.io/vraxel/lib/ansible"
	"vraxel.io/vraxel/lib/ansible/converter"
	"vraxel.io/vraxel/lib/ansible/modules"
	"vraxel.io/vraxel/lib/ansible/template"
)

// The E2E suite only runs when someone points it at a real host, so nothing
// would otherwise notice a typo in it until that day. These tests keep it
// honest from `make check`: the playbook parses, passes the same load-time
// validation the engine applies, names only registered modules, and the
// template renders what the assertions expect.

const e2eDir = "e2e"

// playbooks are lists of plays; everything else under e2e/ that ends in .yml
// is a block list (task file) or a plain variable file.
var playbooks = []string{"site.yml", "multi.yml"}

func isPlaybook(path string) bool {
	for _, p := range playbooks {
		if filepath.Base(path) == p {
			return true
		}
	}
	return false
}

func TestE2EPlaybooksParseAndValidate(t *testing.T) {
	for _, name := range playbooks {
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(e2eDir, name))
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			// ParsePlaybook runs Validate, which is where unsupported
			// directives are rejected, so this covers both.
			pb, err := converter.ParsePlaybook(data)
			if err != nil {
				t.Fatalf("does not load: %v", err)
			}
			if len(pb.Play) == 0 {
				t.Fatal("no plays parsed")
			}
		})
	}

	data, err := os.ReadFile(filepath.Join(e2eDir, "site.yml"))
	if err != nil {
		t.Fatalf("read site.yml: %v", err)
	}
	pb, err := converter.ParsePlaybook(data)
	if err != nil {
		t.Fatalf("site.yml does not load: %v", err)
	}
	if len(pb.Play[0].Roles) == 0 {
		t.Error("the single-host suite should exercise a role")
	}
}

func TestE2ETaskFilesParseAndValidate(t *testing.T) {
	var files []string
	err := filepath.Walk(e2eDir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(p, ".yml") {
			return err
		}
		if isPlaybook(p) || strings.Contains(p, string(filepath.Separator)+"defaults"+string(filepath.Separator)) ||
			strings.Contains(p, string(filepath.Separator)+"vars"+string(filepath.Separator)) {
			return nil // plays and plain var files are not block lists
		}
		files = append(files, p)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", e2eDir, err)
	}
	if len(files) < 3 {
		t.Fatalf("expected the suite to span several task files, found %v", files)
	}

	for _, f := range files {
		t.Run(f, func(t *testing.T) {
			data, err := os.ReadFile(f)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			var blocks []ansible.Block
			if err := yaml.Unmarshal(data, &blocks); err != nil {
				t.Fatalf("parse: %v", err)
			}
			if len(blocks) == 0 {
				t.Fatal("no tasks parsed")
			}
			if err := ansible.ValidateBlocks(blocks); err != nil {
				t.Fatalf("validation: %v", err)
			}
		})
	}
}

// TestE2EReferencesOnlyRegisteredModules catches a task whose module name is
// misspelled. The engine would report "module not found" only once the run
// reached that task, which on a real host is minutes in.
func TestE2EReferencesOnlyRegisteredModules(t *testing.T) {
	// Keys that are task directives rather than modules. Anything else in
	// UnknownField is taken to be a module name.
	directives := map[string]bool{"block": true, "rescue": true, "always": true}

	var check func(t *testing.T, blocks []ansible.Block, where string)
	check = func(t *testing.T, blocks []ansible.Block, where string) {
		for _, b := range blocks {
			if len(b.BlockInfo.Block) > 0 || len(b.BlockInfo.Rescue) > 0 || len(b.BlockInfo.Always) > 0 {
				check(t, b.BlockInfo.Block, where)
				check(t, b.BlockInfo.Rescue, where)
				check(t, b.BlockInfo.Always, where)
				continue
			}
			if b.IncludeTasks != "" {
				continue
			}
			var found bool
			for k := range b.UnknownField {
				if directives[k] {
					continue
				}
				if modules.IsModule(k) {
					found = true
					continue
				}
				t.Errorf("%s: task %q names %q, which is not a registered module", where, b.Name, k)
			}
			if !found && len(b.UnknownField) > 0 {
				t.Errorf("%s: task %q has no registered module", where, b.Name)
			}
		}
	}

	err := filepath.Walk(e2eDir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(p, ".yml") {
			return err
		}
		data, readErr := os.ReadFile(p)
		if readErr != nil {
			return readErr
		}
		if isPlaybook(p) {
			pb, parseErr := converter.ParsePlaybook(data)
			if parseErr != nil {
				return parseErr
			}
			for _, play := range pb.Play {
				check(t, play.PreTasks, p)
				check(t, play.Tasks, p)
				check(t, play.PostTasks, p)
			}
			return nil
		}
		var blocks []ansible.Block
		if yaml.Unmarshal(data, &blocks) == nil && len(blocks) > 0 && blocks[0].UnknownField != nil {
			check(t, blocks, p)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
}

// TestE2ETemplateMatchesAssertions renders the report template with the same
// variables the play declares and checks the strings the play asserts on, so
// the two cannot drift apart unnoticed.
func TestE2ETemplateMatchesAssertions(t *testing.T) {
	tpl, err := os.ReadFile(filepath.Join(e2eDir, "templates", "report.conf"))
	if err != nil {
		t.Fatalf("read template: %v", err)
	}

	vars := map[string]any{
		"services": []any{
			map[string]any{"name": "kept", "tier": "db", "enabled": true},
			map[string]any{"name": "dropped", "tier": "cache", "enabled": false},
			map[string]any{"name": "also_kept", "tier": "db", "enabled": true},
		},
		"nested_groups": []any{[]any{"one", "two"}, []any{"three"}},
		"role_marker":   "role-default",
	}

	out, err := template.ParseString(vars, string(tpl))
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	// These are exactly the substrings tasks/modules.yml asserts on.
	for _, want := range []string{
		"enabled=kept,also_kept",
		"db_tier=kept,also_kept",
		"dropped=dropped",
		"flat=one,two,three",
		"tiers=cache,db",
		"from_role=role-default",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered report missing %q:\n%s", want, out)
		}
	}
}
