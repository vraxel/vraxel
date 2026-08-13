package converter

import (
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"

	"vraxel.io/vraxel/lib/ansible"
)

// ============================================================================
// ParsePlaybook tests
// ============================================================================

func TestParsePlaybook_Simple(t *testing.T) {
	yamlData := `
- hosts: all
  gather_facts: true
  tasks:
    - name: test task
      shell: echo hello
      when: condition1
`
	pb, err := ParsePlaybook([]byte(yamlData))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pb.Play) != 1 {
		t.Fatalf("expected 1 play, got %d", len(pb.Play))
	}

	play := pb.Play[0]
	if len(play.PlayHost.Hosts) != 1 || play.PlayHost.Hosts[0] != "all" {
		t.Fatalf("expected hosts=['all'], got %v", play.PlayHost.Hosts)
	}
	if !play.GatherFacts {
		t.Fatal("expected gather_facts=true")
	}
	if len(play.Tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(play.Tasks))
	}
	task := play.Tasks[0]
	if task.Name != "test task" {
		t.Fatalf("expected task name 'test task', got %q", task.Name)
	}
	if v, ok := task.UnknownField["shell"]; !ok {
		t.Fatal("expected 'shell' in UnknownField")
	} else if v != "echo hello" {
		t.Fatalf("expected shell='echo hello', got %v", v)
	}
	if len(task.When.Data) != 1 {
		t.Fatalf("expected 1 when condition, got %d", len(task.When.Data))
	}
}

func TestParsePlaybook_MultiPlay(t *testing.T) {
	yamlData := `
- hosts: webservers
  tasks:
    - name: install nginx
      shell: apt-get install -y nginx

- hosts: dbservers
  tasks:
    - name: install postgres
      shell: apt-get install -y postgresql
`
	pb, err := ParsePlaybook([]byte(yamlData))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pb.Play) != 2 {
		t.Fatalf("expected 2 plays, got %d", len(pb.Play))
	}
	if pb.Play[0].PlayHost.Hosts[0] != "webservers" {
		t.Fatalf("expected first play hosts=['webservers'], got %v", pb.Play[0].PlayHost.Hosts)
	}
	if pb.Play[1].PlayHost.Hosts[0] != "dbservers" {
		t.Fatalf("expected second play hosts=['dbservers'], got %v", pb.Play[1].PlayHost.Hosts)
	}
}

func TestParsePlaybook_WithRoles(t *testing.T) {
	yamlData := `
- hosts: all
  roles:
    - common
    - role: nginx
      become: true
`
	pb, err := ParsePlaybook([]byte(yamlData))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pb.Play) != 1 {
		t.Fatalf("expected 1 play, got %d", len(pb.Play))
	}
	if len(pb.Play[0].Roles) != 2 {
		t.Fatalf("expected 2 roles, got %d", len(pb.Play[0].Roles))
	}
	if pb.Play[0].Roles[0].Role != "common" {
		t.Fatalf("expected first role 'common', got %q", pb.Play[0].Roles[0].Role)
	}
	if pb.Play[0].Roles[1].Role != "nginx" {
		t.Fatalf("expected second role 'nginx', got %q", pb.Play[0].Roles[1].Role)
	}
	if !pb.Play[0].Roles[1].Become {
		t.Fatal("expected second role become=true")
	}
}

func TestParsePlaybook_ImportPlaybookRejected(t *testing.T) {
	// Silently dropping the import used to turn "also run these plays" into
	// "run nothing", so it is now a load-time error.
	yamlData := `
- import_playbook: other.yml

- hosts: all
  tasks:
    - name: hello
      shell: echo hi
`
	if _, err := ParsePlaybook([]byte(yamlData)); err == nil {
		t.Fatal("expected import_playbook to be rejected, got nil")
	}
}

func TestParsePlaybook_InvalidYAML(t *testing.T) {
	yamlData := `not: valid: playbook: [[[`
	_, err := ParsePlaybook([]byte(yamlData))
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}

func TestParsePlaybook_EmptyHosts(t *testing.T) {
	yamlData := `
- tasks:
    - name: no hosts
      shell: echo hi
`
	_, err := ParsePlaybook([]byte(yamlData))
	if err == nil {
		t.Fatal("expected error for play with empty hosts, got nil")
	}
}

// ============================================================================
// BlockToTaskSpec tests
// ============================================================================

func TestBlockToTaskSpec(t *testing.T) {
	block := ansible.Block{}
	block.Name = "install package"
	block.Become = true
	block.BecomeUser = "root"
	block.DelegateTo = "localhost"
	block.Register = "install_result"
	block.Retries = 3
	block.Delay = 5
	block.Notify = "restart service"
	block.UnknownField = map[string]any{
		"shell": "apt-get install -y nginx",
	}

	moduleFinder := func(name string) bool {
		return name == "shell" || name == "copy" || name == "command"
	}

	spec := BlockToTaskSpec(block, []string{"host1", "host2"}, "myrole", moduleFinder)

	if spec.Name != "install package" {
		t.Fatalf("expected name 'install package', got %q", spec.Name)
	}
	if !reflect.DeepEqual(spec.Hosts, []string{"host1", "host2"}) {
		t.Fatalf("expected hosts [host1 host2], got %v", spec.Hosts)
	}
	if spec.Module.Name != "shell" {
		t.Fatalf("expected module 'shell', got %q", spec.Module.Name)
	}
	// String args should be stored as map[name]=value.
	argsMap, ok := spec.Module.Args.(map[string]any)
	if !ok {
		t.Fatalf("expected module args to be map[string]any, got %T", spec.Module.Args)
	}
	if argsMap["shell"] != "apt-get install -y nginx" {
		t.Fatalf("expected shell arg, got %v", argsMap)
	}
	if !spec.Become {
		t.Fatal("expected become=true")
	}
	if spec.BecomeUser != "root" {
		t.Fatalf("expected become_user 'root', got %q", spec.BecomeUser)
	}
	if spec.DelegateTo != "localhost" {
		t.Fatalf("expected delegate_to 'localhost', got %q", spec.DelegateTo)
	}
	if spec.Register != "install_result" {
		t.Fatalf("expected register 'install_result', got %q", spec.Register)
	}
	if spec.Retries != 3 {
		t.Fatalf("expected retries 3, got %d", spec.Retries)
	}
	if spec.Delay != 5 {
		t.Fatalf("expected delay 5, got %d", spec.Delay)
	}
	if spec.Notify != "restart service" {
		t.Fatalf("expected notify 'restart service', got %q", spec.Notify)
	}
}

func TestBlockToTaskSpec_LoopKinds(t *testing.T) {
	moduleFinder := func(name string) bool { return name == "shell" }

	cases := []struct {
		name     string
		set      func(*ansible.Block)
		wantLoop any
		wantKind ansible.LoopKind
	}{
		{"loop", func(b *ansible.Block) { b.Loop = []any{"a"} }, []any{"a"}, ansible.LoopKindLoop},
		{"with_items", func(b *ansible.Block) { b.WithItems = "{{ .l }}" }, "{{ .l }}", ansible.LoopKindItems},
		{"with_dict", func(b *ansible.Block) { b.WithDict = "{{ .m }}" }, "{{ .m }}", ansible.LoopKindDict},
		{"none", func(*ansible.Block) {}, nil, ansible.LoopKindLoop},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			block := ansible.Block{}
			block.UnknownField = map[string]any{"shell": "true"}
			tc.set(&block)

			spec := BlockToTaskSpec(block, []string{"h"}, "", moduleFinder)

			if !reflect.DeepEqual(spec.Loop, tc.wantLoop) {
				t.Errorf("Loop = %v, want %v", spec.Loop, tc.wantLoop)
			}
			if spec.LoopKind != tc.wantKind {
				t.Errorf("LoopKind = %q, want %q", spec.LoopKind, tc.wantKind)
			}
		})
	}
}

func TestBlockToTaskSpec_ChangedWhenAndEnvironment(t *testing.T) {
	moduleFinder := func(name string) bool { return name == "shell" }

	block := ansible.Block{}
	block.UnknownField = map[string]any{"shell": "true"}
	block.ChangedWhen = ansible.When{Data: []string{"{{ false }}"}}
	block.Environment = map[string]any{"HTTP_PROXY": "http://p"}

	spec := BlockToTaskSpec(block, []string{"h"}, "", moduleFinder)

	if !reflect.DeepEqual(spec.ChangedWhen, []string{"{{ false }}"}) {
		t.Errorf("ChangedWhen = %v", spec.ChangedWhen)
	}
	if !reflect.DeepEqual(spec.Environment, map[string]any{"HTTP_PROXY": "http://p"}) {
		t.Errorf("Environment = %v", spec.Environment)
	}
}

func TestBlockToTaskSpec_MapArgs(t *testing.T) {
	block := ansible.Block{}
	block.Name = "copy file"
	block.UnknownField = map[string]any{
		"copy": map[string]any{
			"src":  "/tmp/foo",
			"dest": "/tmp/bar",
		},
	}

	moduleFinder := func(name string) bool {
		return name == "copy"
	}

	spec := BlockToTaskSpec(block, []string{"host1"}, "", moduleFinder)

	if spec.Module.Name != "copy" {
		t.Fatalf("expected module 'copy', got %q", spec.Module.Name)
	}
	argsMap, ok := spec.Module.Args.(map[string]any)
	if !ok {
		t.Fatalf("expected module args to be map[string]any, got %T", spec.Module.Args)
	}
	if argsMap["src"] != "/tmp/foo" {
		t.Fatalf("expected src '/tmp/foo', got %v", argsMap["src"])
	}
	if argsMap["dest"] != "/tmp/bar" {
		t.Fatalf("expected dest '/tmp/bar', got %v", argsMap["dest"])
	}
}

func TestBlockToTaskSpec_NoModuleFound(t *testing.T) {
	block := ansible.Block{}
	block.Name = "unknown task"
	block.UnknownField = map[string]any{
		"unknown_module": "some args",
	}

	moduleFinder := func(name string) bool {
		return false // no module matches
	}

	spec := BlockToTaskSpec(block, []string{"host1"}, "", moduleFinder)

	if spec.Module.Name != "" {
		t.Fatalf("expected empty module name, got %q", spec.Module.Name)
	}
}

func TestBlockToTaskSpec_WithWhenAndFailedWhen(t *testing.T) {
	block := ansible.Block{}
	block.Name = "conditional task"
	block.When.Data = []string{"{{ ansible_os_family == 'Debian' }}"}
	block.FailedWhen.Data = []string{"{{ result.rc != 0 }}"}
	block.Until.Data = []string{"{{ result.rc == 0 }}"}
	block.UnknownField = map[string]any{
		"shell": "echo hello",
	}

	moduleFinder := func(name string) bool { return name == "shell" }
	spec := BlockToTaskSpec(block, []string{"host1"}, "", moduleFinder)

	if len(spec.When) != 1 || spec.When[0] != "{{ ansible_os_family == 'Debian' }}" {
		t.Fatalf("unexpected when: %v", spec.When)
	}
	if len(spec.FailedWhen) != 1 || spec.FailedWhen[0] != "{{ result.rc != 0 }}" {
		t.Fatalf("unexpected failed_when: %v", spec.FailedWhen)
	}
	if len(spec.Until) != 1 || spec.Until[0] != "{{ result.rc == 0 }}" {
		t.Fatalf("unexpected until: %v", spec.Until)
	}
}

func TestBlockToTaskSpec_WithLoopAndAsync(t *testing.T) {
	block := ansible.Block{}
	block.Name = "async loop task"
	block.Loop = []any{"item1", "item2"}
	block.Task.LoopControl = ansible.LoopControl{
		LoopVar: "my_item",
		Pause:   2.0,
	}
	block.AsyncVal = 30
	block.Poll = 5
	block.UnknownField = map[string]any{
		"command": "long-running-cmd",
	}

	moduleFinder := func(name string) bool { return name == "command" }
	spec := BlockToTaskSpec(block, []string{"host1"}, "", moduleFinder)

	if spec.Async != 30 {
		t.Fatalf("expected async 30, got %d", spec.Async)
	}
	if spec.Poll != 5 {
		t.Fatalf("expected poll 5, got %d", spec.Poll)
	}
	loopItems, ok := spec.Loop.([]any)
	if !ok {
		t.Fatalf("expected loop to be []any, got %T", spec.Loop)
	}
	if len(loopItems) != 2 {
		t.Fatalf("expected 2 loop items, got %d", len(loopItems))
	}
	if spec.LoopControl.LoopVar != "my_item" {
		t.Fatalf("expected loop_var 'my_item', got %q", spec.LoopControl.LoopVar)
	}
	if spec.LoopControl.Pause != 2.0 {
		t.Fatalf("expected pause 2.0, got %f", spec.LoopControl.Pause)
	}
}

// ============================================================================
// GroupHostBySerial tests
// ============================================================================

func TestGroupHostBySerial_Integer(t *testing.T) {
	hosts := []string{"h1", "h2", "h3", "h4"}
	groups, err := GroupHostBySerial(hosts, []any{1, 2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Batch 1: 1 host, Batch 2: 2 hosts, remaining 1 host repeats last serial (2) → 1 more batch.
	if len(groups) != 3 {
		t.Fatalf("expected 3 groups, got %d: %v", len(groups), groups)
	}
	if !reflect.DeepEqual(groups[0], []string{"h1"}) {
		t.Fatalf("group[0]: expected [h1], got %v", groups[0])
	}
	if !reflect.DeepEqual(groups[1], []string{"h2", "h3"}) {
		t.Fatalf("group[1]: expected [h2 h3], got %v", groups[1])
	}
	if !reflect.DeepEqual(groups[2], []string{"h4"}) {
		t.Fatalf("group[2]: expected [h4], got %v", groups[2])
	}
}

func TestGroupHostBySerial_Percentage(t *testing.T) {
	hosts := []string{"h1", "h2", "h3", "h4"}
	groups, err := GroupHostBySerial(hosts, []any{"50%"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 50% of 4 = 2 per batch. Last serial repeats for remaining.
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d: %v", len(groups), groups)
	}
	if !reflect.DeepEqual(groups[0], []string{"h1", "h2"}) {
		t.Fatalf("group[0]: expected [h1 h2], got %v", groups[0])
	}
	if !reflect.DeepEqual(groups[1], []string{"h3", "h4"}) {
		t.Fatalf("group[1]: expected [h3 h4], got %v", groups[1])
	}
}

func TestGroupHostBySerial_Empty(t *testing.T) {
	hosts := []string{"h1", "h2", "h3"}
	groups, err := GroupHostBySerial(hosts, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if !reflect.DeepEqual(groups[0], hosts) {
		t.Fatalf("expected all hosts in one group, got %v", groups[0])
	}
}

func TestGroupHostBySerial_EmptyHosts(t *testing.T) {
	groups, err := GroupHostBySerial(nil, []any{1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if groups[0] != nil {
		t.Fatalf("expected nil hosts in group, got %v", groups[0])
	}
}

func TestGroupHostBySerial_SingleHost(t *testing.T) {
	hosts := []string{"h1"}
	groups, err := GroupHostBySerial(hosts, []any{1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if !reflect.DeepEqual(groups[0], []string{"h1"}) {
		t.Fatalf("expected [h1], got %v", groups[0])
	}
}

func TestGroupHostBySerial_ExactFit(t *testing.T) {
	hosts := []string{"h1", "h2", "h3", "h4", "h5", "h6"}
	groups, err := GroupHostBySerial(hosts, []any{2, 4})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d: %v", len(groups), groups)
	}
	if !reflect.DeepEqual(groups[0], []string{"h1", "h2"}) {
		t.Fatalf("group[0]: expected [h1 h2], got %v", groups[0])
	}
	if !reflect.DeepEqual(groups[1], []string{"h3", "h4", "h5", "h6"}) {
		t.Fatalf("group[1]: expected [h3 h4 h5 h6], got %v", groups[1])
	}
}

func TestGroupHostBySerial_PercentageRoundsUp(t *testing.T) {
	hosts := []string{"h1", "h2", "h3"}
	groups, err := GroupHostBySerial(hosts, []any{"34%"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 34% of 3 = 1.02 → ceil = 2 per batch, then remaining 1.
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d: %v", len(groups), groups)
	}
	if !reflect.DeepEqual(groups[0], []string{"h1", "h2"}) {
		t.Fatalf("group[0]: expected [h1 h2], got %v", groups[0])
	}
	if !reflect.DeepEqual(groups[1], []string{"h3"}) {
		t.Fatalf("group[1]: expected [h3], got %v", groups[1])
	}
}

func TestGroupHostBySerial_StringInteger(t *testing.T) {
	hosts := []string{"h1", "h2", "h3"}
	groups, err := GroupHostBySerial(hosts, []any{"2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d: %v", len(groups), groups)
	}
	if !reflect.DeepEqual(groups[0], []string{"h1", "h2"}) {
		t.Fatalf("group[0]: expected [h1 h2], got %v", groups[0])
	}
	if !reflect.DeepEqual(groups[1], []string{"h3"}) {
		t.Fatalf("group[1]: expected [h3], got %v", groups[1])
	}
}

func TestGroupHostBySerial_InvalidType(t *testing.T) {
	hosts := []string{"h1", "h2"}
	_, err := GroupHostBySerial(hosts, []any{3.14})
	if err == nil {
		t.Fatal("expected error for float serial type, got nil")
	}
}

func TestGroupHostBySerial_ZeroSerial(t *testing.T) {
	hosts := []string{"h1", "h2"}
	_, err := GroupHostBySerial(hosts, []any{0})
	if err == nil {
		t.Fatal("expected error for zero serial, got nil")
	}
}

func TestGroupHostBySerial_InvalidPercentage(t *testing.T) {
	hosts := []string{"h1"}
	_, err := GroupHostBySerial(hosts, []any{"abc%"})
	if err == nil {
		t.Fatal("expected error for invalid percentage, got nil")
	}
}

func TestGroupHostBySerial_InvalidString(t *testing.T) {
	hosts := []string{"h1"}
	_, err := GroupHostBySerial(hosts, []any{"notanumber"})
	if err == nil {
		t.Fatal("expected error for invalid string serial, got nil")
	}
}

// ============================================================================
// ConvertVarsNodes tests
// ============================================================================

func TestConvertVarsNodes(t *testing.T) {
	// Create two yaml nodes representing different variable maps.
	node1 := yaml.Node{}
	data1 := `
foo: bar
num: 42
`
	if err := yaml.Unmarshal([]byte(data1), &node1); err != nil {
		t.Fatalf("failed to unmarshal node1: %v", err)
	}

	node2 := yaml.Node{}
	data2 := `
baz: qux
num: 99
`
	if err := yaml.Unmarshal([]byte(data2), &node2); err != nil {
		t.Fatalf("failed to unmarshal node2: %v", err)
	}

	result, err := ConvertVarsNodes([]yaml.Node{node1, node2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result["foo"] != "bar" {
		t.Fatalf("expected foo='bar', got %v", result["foo"])
	}
	if result["baz"] != "qux" {
		t.Fatalf("expected baz='qux', got %v", result["baz"])
	}
	// Later node overrides earlier value.
	if result["num"] != 99 {
		t.Fatalf("expected num=99 (overridden), got %v", result["num"])
	}
}

func TestConvertVarsNodes_Empty(t *testing.T) {
	result, err := ConvertVarsNodes(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("expected empty map, got %v", result)
	}
}

func TestConvertVarsNodes_Single(t *testing.T) {
	node := yaml.Node{}
	data := `
key1: value1
key2: value2
`
	if err := yaml.Unmarshal([]byte(data), &node); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	result, err := ConvertVarsNodes([]yaml.Node{node})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["key1"] != "value1" {
		t.Fatalf("expected key1='value1', got %v", result["key1"])
	}
	if result["key2"] != "value2" {
		t.Fatalf("expected key2='value2', got %v", result["key2"])
	}
}

func TestConvertVarsNodes_FromPlaybook(t *testing.T) {
	// Parse a playbook with vars and then convert the vars nodes.
	yamlData := `
- hosts: all
  vars:
    app_name: myapp
    version: "2.0"
  tasks:
    - name: echo
      shell: echo hello
`
	pb, err := ParsePlaybook([]byte(yamlData))
	if err != nil {
		t.Fatalf("unexpected error parsing playbook: %v", err)
	}

	vars, err := ConvertVarsNodes(pb.Play[0].Vars.Nodes)
	if err != nil {
		t.Fatalf("unexpected error converting vars: %v", err)
	}
	if vars["app_name"] != "myapp" {
		t.Fatalf("expected app_name='myapp', got %v", vars["app_name"])
	}
	if vars["version"] != "2.0" {
		t.Fatalf("expected version='2.0', got %v", vars["version"])
	}
}

// ============================================================================
// Integration test: parse and convert
// ============================================================================

func TestParseAndConvert(t *testing.T) {
	yamlData := `
- hosts:
    - web1
    - web2
    - web3
  become: true
  serial: 1
  tasks:
    - name: deploy app
      copy:
        src: /local/app.tar.gz
        dest: /opt/app.tar.gz
      when: deploy_enabled
      register: deploy_result
      retries: 3
      delay: 10
      until: deploy_result is succeeded
`
	pb, err := ParsePlaybook([]byte(yamlData))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	play := pb.Play[0]

	// Convert serial.
	groups, err := GroupHostBySerial(play.PlayHost.Hosts, play.Serial.Data)
	if err != nil {
		t.Fatalf("unexpected error grouping hosts: %v", err)
	}
	if len(groups) != 3 {
		t.Fatalf("expected 3 groups (serial=1, 3 hosts), got %d", len(groups))
	}

	// Convert block to task spec.
	moduleFinder := func(name string) bool {
		return name == "copy" || name == "shell" || name == "command"
	}
	spec := BlockToTaskSpec(play.Tasks[0], play.PlayHost.Hosts, "", moduleFinder)

	if spec.Name != "deploy app" {
		t.Fatalf("expected name 'deploy app', got %q", spec.Name)
	}
	if spec.Module.Name != "copy" {
		t.Fatalf("expected module 'copy', got %q", spec.Module.Name)
	}
	if spec.Register != "deploy_result" {
		t.Fatalf("expected register 'deploy_result', got %q", spec.Register)
	}
	if spec.Retries != 3 {
		t.Fatalf("expected retries 3, got %d", spec.Retries)
	}
	if spec.Delay != 10 {
		t.Fatalf("expected delay 10, got %d", spec.Delay)
	}
	if len(spec.When) != 1 {
		t.Fatalf("expected 1 when condition, got %d", len(spec.When))
	}
	if len(spec.Until) != 1 {
		t.Fatalf("expected 1 until condition, got %d", len(spec.Until))
	}
}

// A serial entry larger than the hosts left over must not panic. The
// batch slicing advanced begin by the full serial value while clamping
// only end, so the next iteration sliced hosts[begin:end] with begin >
// end. Reachable from any playbook whose serial list overshoots the
// inventory, and it would take the whole server down: the run driver
// plans playbooks on a background goroutine.
func TestGroupHostBySerial_EntryExceedsRemainingHosts(t *testing.T) {
	cases := []struct {
		name   string
		hosts  []string
		serial []any
	}{
		{"overshoot then more", []string{"h1", "h2"}, []any{3, 1}},
		{"overshoot twice", []string{"h1"}, []any{5, 5}},
		{"exact then more", []string{"h1", "h2"}, []any{2, 2}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := GroupHostBySerial(tc.hosts, tc.serial)
			if err != nil {
				t.Fatalf("GroupHostBySerial: %v", err)
			}
			var flat []string
			for _, b := range got {
				flat = append(flat, b...)
			}
			if len(flat) != len(tc.hosts) {
				t.Errorf("batches cover %v, want exactly %v once each", flat, tc.hosts)
			}
		})
	}
}

// A negative serial used to reach the slicing loop and panic on a
// negative index; it is a playbook error and must be reported as one.
func TestGroupHostBySerial_NegativeRejected(t *testing.T) {
	for _, serial := range [][]any{{-1}, {-3, 2}, {2, -1}} {
		if _, err := GroupHostBySerial([]string{"h1", "h2"}, serial); err == nil {
			t.Errorf("GroupHostBySerial(serial=%v) = nil error, want a rejection", serial)
		}
	}
}
