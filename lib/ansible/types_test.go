package ansible

import (
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
)

// ============================================================================
// PlayHost UnmarshalYAML tests
// ============================================================================

func TestPlayHost_UnmarshalYAML_Scalar(t *testing.T) {
	input := `hosts: all`
	var play struct {
		Hosts PlayHost `yaml:"hosts"`
	}
	if err := yaml.Unmarshal([]byte(input), &play); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(play.Hosts.Hosts) != 1 {
		t.Fatalf("expected 1 host, got %d", len(play.Hosts.Hosts))
	}
	if play.Hosts.Hosts[0] != "all" {
		t.Fatalf("expected host 'all', got %q", play.Hosts.Hosts[0])
	}
}

func TestPlayHost_UnmarshalYAML_Sequence(t *testing.T) {
	input := `hosts:
  - web
  - db
  - cache`
	var play struct {
		Hosts PlayHost `yaml:"hosts"`
	}
	if err := yaml.Unmarshal([]byte(input), &play); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(play.Hosts.Hosts) != 3 {
		t.Fatalf("expected 3 hosts, got %d", len(play.Hosts.Hosts))
	}
	expected := []string{"web", "db", "cache"}
	for i, h := range play.Hosts.Hosts {
		if h != expected[i] {
			t.Fatalf("host[%d]: expected %q, got %q", i, expected[i], h)
		}
	}
}

func TestPlayHost_UnmarshalYAML_InvalidType(t *testing.T) {
	input := `hosts:
  key: value`
	var play struct {
		Hosts PlayHost `yaml:"hosts"`
	}
	if err := yaml.Unmarshal([]byte(input), &play); err == nil {
		t.Fatal("expected error for mapping type, got nil")
	}
}

// ============================================================================
// PlaySerial UnmarshalYAML tests
// ============================================================================

func TestPlaySerial_UnmarshalYAML_Scalar(t *testing.T) {
	input := `serial: "5"`
	var play struct {
		Serial PlaySerial `yaml:"serial"`
	}
	if err := yaml.Unmarshal([]byte(input), &play); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(play.Serial.Data) != 1 {
		t.Fatalf("expected 1 serial value, got %d", len(play.Serial.Data))
	}
	if play.Serial.Data[0] != "5" {
		t.Fatalf("expected serial '5', got %v", play.Serial.Data[0])
	}
}

func TestPlaySerial_UnmarshalYAML_Sequence(t *testing.T) {
	input := `serial:
  - 1
  - 5
  - 10`
	var play struct {
		Serial PlaySerial `yaml:"serial"`
	}
	if err := yaml.Unmarshal([]byte(input), &play); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(play.Serial.Data) != 3 {
		t.Fatalf("expected 3 serial values, got %d", len(play.Serial.Data))
	}
}

func TestPlaySerial_UnmarshalYAML_InvalidType(t *testing.T) {
	input := `serial:
  key: value`
	var play struct {
		Serial PlaySerial `yaml:"serial"`
	}
	if err := yaml.Unmarshal([]byte(input), &play); err == nil {
		t.Fatal("expected error for mapping type, got nil")
	}
}

// ============================================================================
// When UnmarshalYAML tests
// ============================================================================

func TestWhen_UnmarshalYAML_Scalar(t *testing.T) {
	input := `when: ansible_os_family == "Debian"`
	var cond struct {
		When When `yaml:"when"`
	}
	if err := yaml.Unmarshal([]byte(input), &cond); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cond.When.Data) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(cond.When.Data))
	}
	// Should be wrapped in template syntax.
	expected := `{{ ansible_os_family == "Debian" }}`
	if cond.When.Data[0] != expected {
		t.Fatalf("expected %q, got %q", expected, cond.When.Data[0])
	}
}

func TestWhen_UnmarshalYAML_Scalar_AlreadyTemplate(t *testing.T) {
	input := `when: "{{ my_var }}"`
	var cond struct {
		When When `yaml:"when"`
	}
	if err := yaml.Unmarshal([]byte(input), &cond); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cond.When.Data) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(cond.When.Data))
	}
	// Should NOT be double-wrapped.
	if cond.When.Data[0] != "{{ my_var }}" {
		t.Fatalf("expected '{{ my_var }}', got %q", cond.When.Data[0])
	}
}

func TestWhen_UnmarshalYAML_Sequence(t *testing.T) {
	input := `when:
  - ansible_os_family == "Debian"
  - "{{ some_var }}"
  - another_condition`
	var cond struct {
		When When `yaml:"when"`
	}
	if err := yaml.Unmarshal([]byte(input), &cond); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cond.When.Data) != 3 {
		t.Fatalf("expected 3 conditions, got %d", len(cond.When.Data))
	}
	// First: not template, should be wrapped.
	if cond.When.Data[0] != `{{ ansible_os_family == "Debian" }}` {
		t.Fatalf("when[0]: expected template-wrapped, got %q", cond.When.Data[0])
	}
	// Second: already template, should remain.
	if cond.When.Data[1] != "{{ some_var }}" {
		t.Fatalf("when[1]: expected '{{ some_var }}', got %q", cond.When.Data[1])
	}
	// Third: not template, should be wrapped.
	if cond.When.Data[2] != "{{ another_condition }}" {
		t.Fatalf("when[2]: expected template-wrapped, got %q", cond.When.Data[2])
	}
}

// ============================================================================
// Tags UnmarshalYAML tests
// ============================================================================

func TestTags_UnmarshalYAML_Scalar(t *testing.T) {
	input := `tags: setup`
	var tag struct {
		Tags Tags `yaml:"tags"`
	}
	if err := yaml.Unmarshal([]byte(input), &tag); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tag.Tags.Data) != 1 || tag.Tags.Data[0] != "setup" {
		t.Fatalf("expected ['setup'], got %v", tag.Tags.Data)
	}
}

func TestTags_UnmarshalYAML_Sequence(t *testing.T) {
	input := `tags:
  - setup
  - install
  - config`
	var tag struct {
		Tags Tags `yaml:"tags"`
	}
	if err := yaml.Unmarshal([]byte(input), &tag); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := []string{"setup", "install", "config"}
	if len(tag.Tags.Data) != 3 {
		t.Fatalf("expected 3 tags, got %d", len(tag.Tags.Data))
	}
	for i, s := range tag.Tags.Data {
		if s != expected[i] {
			t.Fatalf("tag[%d]: expected %q, got %q", i, expected[i], s)
		}
	}
}

func TestTags_IsEnabled(t *testing.T) {
	tests := []struct {
		name     string
		tags     Tags
		only     []string
		skip     []string
		expected bool
	}{
		{
			name:     "no filters",
			tags:     Tags{Data: []string{"setup"}},
			only:     nil,
			skip:     nil,
			expected: true,
		},
		{
			name:     "always tag ignores onlyTags",
			tags:     Tags{Data: []string{"always"}},
			only:     []string{"deploy"},
			skip:     nil,
			expected: true,
		},
		{
			name:     "never tag with all onlyTag",
			tags:     Tags{Data: []string{"never"}},
			only:     []string{"all"},
			skip:     nil,
			expected: false,
		},
		{
			name:     "matching onlyTag",
			tags:     Tags{Data: []string{"setup", "install"}},
			only:     []string{"setup"},
			skip:     nil,
			expected: true,
		},
		{
			name:     "non-matching onlyTag",
			tags:     Tags{Data: []string{"setup"}},
			only:     []string{"deploy"},
			skip:     nil,
			expected: false,
		},
		{
			name:     "matching skipTag",
			tags:     Tags{Data: []string{"setup"}},
			only:     nil,
			skip:     []string{"setup"},
			expected: false,
		},
		{
			name:     "skip all",
			tags:     Tags{Data: []string{"setup"}},
			only:     nil,
			skip:     []string{"all"},
			expected: false,
		},
		{
			name:     "skip all skips always too",
			tags:     Tags{Data: []string{"always"}},
			only:     nil,
			skip:     []string{"all"},
			expected: false,
		},
		{
			name:     "skip all and always explicitly",
			tags:     Tags{Data: []string{"always"}},
			only:     nil,
			skip:     []string{"all", "always"},
			expected: false,
		},
		{
			name:     "tagged onlyTag with tags",
			tags:     Tags{Data: []string{"setup"}},
			only:     []string{"tagged"},
			skip:     nil,
			expected: true,
		},
		{
			name:     "tagged skipTag with tags",
			tags:     Tags{Data: []string{"setup"}},
			only:     nil,
			skip:     []string{"tagged"},
			expected: false,
		},
		{
			name:     "empty tags with no filters",
			tags:     Tags{Data: nil},
			only:     nil,
			skip:     nil,
			expected: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.tags.IsEnabled(tc.only, tc.skip)
			if got != tc.expected {
				t.Fatalf("IsEnabled(%v, %v) = %v, want %v", tc.only, tc.skip, got, tc.expected)
			}
		})
	}
}

func TestTags_JoinTag(t *testing.T) {
	a := Tags{Data: []string{"setup", "install"}}
	b := Tags{Data: []string{"install", "deploy"}}
	result := a.JoinTag(b)
	if len(result.Data) != 3 {
		t.Fatalf("expected 3 tags after join, got %d: %v", len(result.Data), result.Data)
	}
	expected := []string{"setup", "install", "deploy"}
	for i, s := range result.Data {
		if s != expected[i] {
			t.Fatalf("tag[%d]: expected %q, got %q", i, expected[i], s)
		}
	}
}

func TestTags_JoinTag_NoOverlap(t *testing.T) {
	a := Tags{Data: []string{"a"}}
	b := Tags{Data: []string{"b"}}
	result := a.JoinTag(b)
	if len(result.Data) != 2 {
		t.Fatalf("expected 2 tags, got %d", len(result.Data))
	}
}

func TestTags_JoinTag_DoesNotMutateOriginal(t *testing.T) {
	a := Tags{Data: []string{"a"}}
	b := Tags{Data: []string{"b"}}
	_ = a.JoinTag(b)
	if len(a.Data) != 1 {
		t.Fatalf("original tags should not be modified, got %v", a.Data)
	}
}

// ============================================================================
// Role UnmarshalYAML tests
// ============================================================================

func TestRole_UnmarshalYAML_Scalar(t *testing.T) {
	input := `roles:
  - myrole`
	var play struct {
		Roles []Role `yaml:"roles"`
	}
	if err := yaml.Unmarshal([]byte(input), &play); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(play.Roles) != 1 {
		t.Fatalf("expected 1 role, got %d", len(play.Roles))
	}
	if play.Roles[0].Role != "myrole" {
		t.Fatalf("expected role name 'myrole', got %q", play.Roles[0].Role)
	}
}

func TestRole_UnmarshalYAML_Mapping(t *testing.T) {
	input := `roles:
  - role: nginx
    name: install nginx
    become: true
    when: ansible_os_family == "Debian"`
	var play struct {
		Roles []Role `yaml:"roles"`
	}
	if err := yaml.Unmarshal([]byte(input), &play); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(play.Roles) != 1 {
		t.Fatalf("expected 1 role, got %d", len(play.Roles))
	}
	r := play.Roles[0]
	if r.Role != "nginx" {
		t.Fatalf("expected role 'nginx', got %q", r.Role)
	}
	if r.Name != "install nginx" {
		t.Fatalf("expected name 'install nginx', got %q", r.Name)
	}
	if !r.Become {
		t.Fatal("expected become=true")
	}
	if len(r.When.Data) != 1 {
		t.Fatalf("expected 1 when condition, got %d", len(r.When.Data))
	}
}

func TestRole_UnmarshalYAML_Mixed(t *testing.T) {
	input := `roles:
  - simple_role
  - role: complex_role
    become: true`
	var play struct {
		Roles []Role `yaml:"roles"`
	}
	if err := yaml.Unmarshal([]byte(input), &play); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(play.Roles) != 2 {
		t.Fatalf("expected 2 roles, got %d", len(play.Roles))
	}
	if play.Roles[0].Role != "simple_role" {
		t.Fatalf("expected first role 'simple_role', got %q", play.Roles[0].Role)
	}
	if play.Roles[1].Role != "complex_role" {
		t.Fatalf("expected second role 'complex_role', got %q", play.Roles[1].Role)
	}
	if !play.Roles[1].Become {
		t.Fatal("expected second role become=true")
	}
}

// ============================================================================
// Block UnmarshalYAML tests
// ============================================================================

func TestBlock_UnmarshalYAML_KnownFields(t *testing.T) {
	input := `- name: install package
  register: result
  when: ansible_os_family == "Debian"
  shell: apt-get install -y nginx`
	var blocks []Block
	if err := yaml.Unmarshal([]byte(input), &blocks); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	b := blocks[0]
	if b.Name != "install package" {
		t.Fatalf("expected name 'install package', got %q", b.Name)
	}
	if b.Register != "result" {
		t.Fatalf("expected register 'result', got %q", b.Register)
	}
	if len(b.When.Data) != 1 {
		t.Fatalf("expected 1 when condition, got %d", len(b.When.Data))
	}
}

func TestBlock_UnmarshalYAML_UnknownModuleField(t *testing.T) {
	input := `- name: run shell command
  shell: echo hello
  args:
    chdir: /tmp`
	var blocks []Block
	if err := yaml.Unmarshal([]byte(input), &blocks); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	b := blocks[0]

	// "shell" should be in UnknownField since it's not a BlockBase or Task field.
	if b.UnknownField == nil {
		t.Fatal("expected UnknownField to be non-nil")
	}
	if v, ok := b.UnknownField["shell"]; !ok {
		t.Fatal("expected 'shell' in UnknownField")
	} else if v != "echo hello" {
		t.Fatalf("expected shell value 'echo hello', got %v", v)
	}
	// "args" is also unknown.
	if _, ok := b.UnknownField["args"]; !ok {
		t.Fatal("expected 'args' in UnknownField")
	}
}

func TestBlock_UnmarshalYAML_BlockRescueAlways(t *testing.T) {
	input := `- name: error handling block
  block:
    - name: try this
      shell: /bin/true
  rescue:
    - name: handle error
      shell: echo "error occurred"
  always:
    - name: cleanup
      shell: rm -f /tmp/lockfile`
	var blocks []Block
	if err := yaml.Unmarshal([]byte(input), &blocks); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	b := blocks[0]
	if b.Name != "error handling block" {
		t.Fatalf("expected name 'error handling block', got %q", b.Name)
	}
	if len(b.BlockInfo.Block) != 1 {
		t.Fatalf("expected 1 block task, got %d", len(b.BlockInfo.Block))
	}
	if len(b.BlockInfo.Rescue) != 1 {
		t.Fatalf("expected 1 rescue task, got %d", len(b.BlockInfo.Rescue))
	}
	if len(b.BlockInfo.Always) != 1 {
		t.Fatalf("expected 1 always task, got %d", len(b.BlockInfo.Always))
	}
	if b.BlockInfo.Block[0].Name != "try this" {
		t.Fatalf("expected block task name 'try this', got %q", b.BlockInfo.Block[0].Name)
	}
}

func TestBlock_UnmarshalYAML_IncludeTasks(t *testing.T) {
	input := `- name: include other tasks
  include_tasks: other.yml`
	var blocks []Block
	if err := yaml.Unmarshal([]byte(input), &blocks); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	if blocks[0].IncludeTasks != "other.yml" {
		t.Fatalf("expected include_tasks 'other.yml', got %q", blocks[0].IncludeTasks)
	}
}

func TestBlock_UnmarshalYAML_MultipleModuleFields(t *testing.T) {
	input := `- name: copy file
  copy:
    src: /tmp/foo
    dest: /tmp/bar
  become: true
  tags: deploy`
	var blocks []Block
	if err := yaml.Unmarshal([]byte(input), &blocks); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	b := blocks[0]
	if !b.Become {
		t.Fatal("expected become=true")
	}
	if len(b.Tags.Data) != 1 || b.Tags.Data[0] != "deploy" {
		t.Fatalf("expected tags ['deploy'], got %v", b.Tags.Data)
	}
	// "copy" should be unknown field.
	if _, ok := b.UnknownField["copy"]; !ok {
		t.Fatal("expected 'copy' in UnknownField")
	}
}

// ============================================================================
// Vars UnmarshalYAML tests
// ============================================================================

func TestVars_UnmarshalYAML_Merge(t *testing.T) {
	input := `vars:
  foo: bar
  num: 42`
	var base struct {
		Vars Vars `yaml:"vars"`
	}
	if err := yaml.Unmarshal([]byte(input), &base); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(base.Vars.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(base.Vars.Nodes))
	}
	// The node should be a mapping.
	if base.Vars.Nodes[0].Kind != yaml.MappingNode {
		t.Fatalf("expected MappingNode, got %v", base.Vars.Nodes[0].Kind)
	}
}

func TestVars_UnmarshalYAML_MultipleMerge(t *testing.T) {
	// Simulate merging vars from multiple YAML documents by calling UnmarshalYAML twice.
	v := &Vars{}

	node1 := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
		{Kind: yaml.ScalarNode, Value: "a"},
		{Kind: yaml.ScalarNode, Value: "1"},
	}}
	if err := v.UnmarshalYAML(node1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	node2 := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
		{Kind: yaml.ScalarNode, Value: "b"},
		{Kind: yaml.ScalarNode, Value: "2"},
	}}
	if err := v.UnmarshalYAML(node2); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(v.Nodes) != 2 {
		t.Fatalf("expected 2 nodes after multiple merges, got %d", len(v.Nodes))
	}
}

// ============================================================================
// Playbook Validate tests
// ============================================================================

func TestPlaybook_Validate_RemovesImportPlaybook(t *testing.T) {
	pb := &Playbook{
		Play: []Play{
			{ImportPlaybook: "other.yml"},
			{PlayHost: PlayHost{Hosts: []string{"all"}}},
		},
	}
	if err := pb.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pb.Play) != 1 {
		t.Fatalf("expected 1 play after validation, got %d", len(pb.Play))
	}
	if len(pb.Play[0].PlayHost.Hosts) != 1 || pb.Play[0].PlayHost.Hosts[0] != "all" {
		t.Fatalf("expected remaining play with hosts=['all']")
	}
}

func TestPlaybook_Validate_EmptyHosts(t *testing.T) {
	pb := &Playbook{
		Play: []Play{
			{PlayHost: PlayHost{Hosts: nil}},
		},
	}
	if err := pb.Validate(); err == nil {
		t.Fatal("expected error for play with no hosts, got nil")
	}
}

func TestPlaybook_Validate_ValidPlay(t *testing.T) {
	pb := &Playbook{
		Play: []Play{
			{PlayHost: PlayHost{Hosts: []string{"webservers"}}},
		},
	}
	if err := pb.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pb.Play) != 1 {
		t.Fatalf("expected 1 play, got %d", len(pb.Play))
	}
}

func TestPlaybook_Validate_AllImports(t *testing.T) {
	pb := &Playbook{
		Play: []Play{
			{ImportPlaybook: "a.yml"},
			{ImportPlaybook: "b.yml"},
		},
	}
	if err := pb.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pb.Play) != 0 {
		t.Fatalf("expected 0 plays after removing all imports, got %d", len(pb.Play))
	}
}

// ============================================================================
// Template syntax helper tests
// ============================================================================

func TestIsTmplSyntax(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"{{ foo }}", true},
		{"{{foo}}", true},
		{"hello {{ world }}", true},
		{"no template here", false},
		{"{{ missing end", false},
		{"missing start }}", false},
		{"", false},
	}
	for _, tc := range tests {
		got := IsTmplSyntax(tc.input)
		if got != tc.expected {
			t.Errorf("IsTmplSyntax(%q) = %v, want %v", tc.input, got, tc.expected)
		}
	}
}

func TestParseTmplSyntax(t *testing.T) {
	result := ParseTmplSyntax("my_var")
	expected := "{{ my_var }}"
	if result != expected {
		t.Fatalf("expected %q, got %q", expected, result)
	}
}

func TestTrimTmplSyntax(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"{{ foo }}", "foo"},
		{"{{bar}}", "bar"},
		{"{{ spaced }}", "spaced"},
	}
	for _, tc := range tests {
		got := TrimTmplSyntax(tc.input)
		if got != tc.expected {
			t.Errorf("TrimTmplSyntax(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

// ============================================================================
// Inventory tests
// ============================================================================

func TestGetHostsFromGroup(t *testing.T) {
	inv := &Inventory{
		Hosts: map[string]map[string]any{
			"node1": {"ansible_host": "10.0.0.1"},
			"node2": {"ansible_host": "10.0.0.2"},
			"node3": {"ansible_host": "10.0.0.3"},
		},
		Groups: map[string]InventoryGroup{
			"web": {
				Hosts: []string{"node1", "node2"},
			},
			"db": {
				Hosts: []string{"node2", "node3"},
			},
			"all": {
				Groups: []string{"web", "db"},
			},
		},
	}

	unavailableHosts := make(map[string]struct{})
	unavailableGroups := make(map[string]struct{})
	hosts := GetHostsFromGroup(inv, "all", unavailableHosts, unavailableGroups)

	// Should have all 3 unique hosts.
	if len(hosts) != 3 {
		t.Fatalf("expected 3 hosts, got %d: %v", len(hosts), hosts)
	}
}

func TestGetHostsFromGroup_NonexistentGroup(t *testing.T) {
	inv := &Inventory{
		Hosts:  map[string]map[string]any{},
		Groups: map[string]InventoryGroup{},
	}
	hosts := GetHostsFromGroup(inv, "missing", make(map[string]struct{}), make(map[string]struct{}))
	if len(hosts) != 0 {
		t.Fatalf("expected 0 hosts for missing group, got %d", len(hosts))
	}
}

func TestGetHostsFromGroup_CircularGroups(t *testing.T) {
	inv := &Inventory{
		Hosts: map[string]map[string]any{
			"node1": {},
		},
		Groups: map[string]InventoryGroup{
			"a": {
				Groups: []string{"b"},
				Hosts:  []string{"node1"},
			},
			"b": {
				Groups: []string{"a"},
			},
		},
	}
	unavailableHosts := make(map[string]struct{})
	unavailableGroups := make(map[string]struct{})
	hosts := GetHostsFromGroup(inv, "a", unavailableHosts, unavailableGroups)
	// Should not loop infinitely and should return node1.
	if len(hosts) != 1 || hosts[0] != "node1" {
		t.Fatalf("expected [node1], got %v", hosts)
	}
}

func TestGetHostsFromGroup_InvalidHost(t *testing.T) {
	inv := &Inventory{
		Hosts: map[string]map[string]any{
			"node1": {},
		},
		Groups: map[string]InventoryGroup{
			"web": {
				Hosts: []string{"node1", "node_nonexistent"},
			},
		},
	}
	hosts := GetHostsFromGroup(inv, "web", make(map[string]struct{}), make(map[string]struct{}))
	// Only valid hosts should be returned.
	if len(hosts) != 1 || hosts[0] != "node1" {
		t.Fatalf("expected [node1], got %v", hosts)
	}
}

// ============================================================================
// Full playbook YAML parse test
// ============================================================================

func TestFullPlaybookParse(t *testing.T) {
	input := `- hosts: webservers
  name: Deploy web app
  become: true
  gather_facts: true
  vars:
    app_version: "1.0.0"
  roles:
    - common
    - role: nginx
      when: install_nginx
  pre_tasks:
    - name: update apt cache
      shell: apt-get update
      when: ansible_os_family == "Debian"
  tasks:
    - name: copy config
      copy:
        src: app.conf
        dest: /etc/app.conf
      tags:
        - config
        - deploy
    - name: restart service
      shell: systemctl restart app
      notify: verify service
  handlers:
    - name: verify service
      shell: systemctl status app
  serial: 5
  strategy: free`

	var plays []Play
	if err := yaml.Unmarshal([]byte(input), &plays); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plays) != 1 {
		t.Fatalf("expected 1 play, got %d", len(plays))
	}

	p := plays[0]
	if p.Name != "Deploy web app" {
		t.Fatalf("expected name 'Deploy web app', got %q", p.Name)
	}
	if !p.Become {
		t.Fatal("expected become=true")
	}
	if !p.GatherFacts {
		t.Fatal("expected gather_facts=true")
	}
	if len(p.PlayHost.Hosts) != 1 || p.PlayHost.Hosts[0] != "webservers" {
		t.Fatalf("expected hosts ['webservers'], got %v", p.PlayHost.Hosts)
	}
	if len(p.Roles) != 2 {
		t.Fatalf("expected 2 roles, got %d", len(p.Roles))
	}
	if p.Roles[0].Role != "common" {
		t.Fatalf("expected first role 'common', got %q", p.Roles[0].Role)
	}
	if p.Roles[1].Role != "nginx" {
		t.Fatalf("expected second role 'nginx', got %q", p.Roles[1].Role)
	}
	if len(p.PreTasks) != 1 {
		t.Fatalf("expected 1 pre_task, got %d", len(p.PreTasks))
	}
	if len(p.Tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(p.Tasks))
	}
	if len(p.Handlers) != 1 {
		t.Fatalf("expected 1 handler, got %d", len(p.Handlers))
	}
	if p.Strategy != "free" {
		t.Fatalf("expected strategy 'free', got %q", p.Strategy)
	}
	if len(p.Serial.Data) != 1 || p.Serial.Data[0] != "5" {
		t.Fatalf("expected serial ['5'], got %v", p.Serial.Data)
	}

	// Check task with tags.
	copyTask := p.Tasks[0]
	if len(copyTask.Tags.Data) != 2 {
		t.Fatalf("expected 2 tags on copy task, got %d", len(copyTask.Tags.Data))
	}
	if _, ok := copyTask.UnknownField["copy"]; !ok {
		t.Fatal("expected 'copy' in UnknownField of copy task")
	}

	// Check task with notify.
	restartTask := p.Tasks[1]
	if restartTask.Notify != "verify service" {
		t.Fatalf("expected notify 'verify service', got %q", restartTask.Notify)
	}
}

// ============================================================================
// Execution types tests
// ============================================================================

func TestNewPlaybookStats(t *testing.T) {
	stats := NewPlaybookStats()
	if stats.OK == nil || stats.Changed == nil || stats.Unreachable == nil ||
		stats.Failed == nil || stats.Skipped == nil {
		t.Fatal("expected all maps to be initialized")
	}
	// Should be empty but not nil.
	stats.OK["host1"] = 5
	if stats.OK["host1"] != 5 {
		t.Fatal("expected map to be writable")
	}
}

func TestTaskStatus_Constants(t *testing.T) {
	// Verify the constants exist and have expected values.
	statuses := []TaskStatus{
		TaskStatusPending,
		TaskStatusRunning,
		TaskStatusOK,
		TaskStatusChanged,
		TaskStatusFailed,
		TaskStatusSkipped,
		TaskStatusUnreachable,
	}
	expected := []string{"pending", "running", "ok", "changed", "failed", "skipped", "unreachable"}
	for i, s := range statuses {
		if string(s) != expected[i] {
			t.Fatalf("expected %q, got %q", expected[i], string(s))
		}
	}
}

// ============================================================================
// getFieldNames tests
// ============================================================================

func TestGetFieldNames(t *testing.T) {
	names := getFieldNames(reflect.TypeOf(BlockBase{}))
	// Should include fields from Base (name, connection, port, etc.),
	// Conditional (when), Taggable (tags), Notifiable (notify), Delegable (delegate_to, delegate_facts).
	expectedFields := []string{"name", "connection", "port", "remote_user", "when", "tags", "notify", "delegate_to"}
	for _, ef := range expectedFields {
		found := false
		for _, n := range names {
			if n == ef {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected field %q in BlockBase field names, not found in %v", ef, names)
		}
	}
}

func TestGetFieldNames_Task(t *testing.T) {
	names := getFieldNames(reflect.TypeOf(Task{}))
	expectedFields := []string{"async", "changed_when", "delay", "failed_when", "loop",
		"loop_control", "poll", "register", "register_type", "retries", "until"}
	for _, ef := range expectedFields {
		found := false
		for _, n := range names {
			if n == ef {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected field %q in Task field names, not found in %v", ef, names)
		}
	}
}

// ============================================================================
// Edge case: Block with loop
// ============================================================================

func TestBlock_UnmarshalYAML_WithLoop(t *testing.T) {
	input := `- name: install packages
  shell: "apt-get install -y {{ item }}"
  loop:
    - nginx
    - curl
    - vim
  loop_control:
    loop_var: item
    pause: 1.5
  register: install_result
  retries: 3
  delay: 5
  until: install_result is succeeded`
	var blocks []Block
	if err := yaml.Unmarshal([]byte(input), &blocks); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	b := blocks[0]
	if b.Register != "install_result" {
		t.Fatalf("expected register 'install_result', got %q", b.Register)
	}
	if b.Retries != 3 {
		t.Fatalf("expected retries 3, got %d", b.Retries)
	}
	if b.Delay != 5 {
		t.Fatalf("expected delay 5, got %d", b.Delay)
	}
	if b.Task.LoopControl.LoopVar != "item" {
		t.Fatalf("expected loop_var 'item', got %q", b.Task.LoopControl.LoopVar)
	}
	if b.Task.LoopControl.Pause != 1.5 {
		t.Fatalf("expected pause 1.5, got %f", b.Task.LoopControl.Pause)
	}
	// "shell" should be in unknown fields.
	if _, ok := b.UnknownField["shell"]; !ok {
		t.Fatal("expected 'shell' in UnknownField")
	}
}

// ============================================================================
// Block with become and delegate
// ============================================================================

func TestBlock_UnmarshalYAML_BecomeAndDelegate(t *testing.T) {
	input := `- name: delegate task
  shell: hostname
  become: true
  become_user: admin
  delegate_to: localhost`
	var blocks []Block
	if err := yaml.Unmarshal([]byte(input), &blocks); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	b := blocks[0]
	if !b.Become {
		t.Fatal("expected become=true")
	}
	if b.BecomeUser != "admin" {
		t.Fatalf("expected become_user 'admin', got %q", b.BecomeUser)
	}
	if b.DelegateTo != "localhost" {
		t.Fatalf("expected delegate_to 'localhost', got %q", b.DelegateTo)
	}
}

// ============================================================================
// Inventory YAML parsing
// ============================================================================

func TestInventory_YAMLParse(t *testing.T) {
	input := `hosts:
  node1:
    ansible_host: "10.0.0.1"
    ansible_port: 22
  node2:
    ansible_host: "10.0.0.2"
vars:
  global_var: hello
groups:
  web:
    hosts:
      - node1
    vars:
      http_port: 80
  db:
    hosts:
      - node2
  all:
    groups:
      - web
      - db`

	var inv Inventory
	if err := yaml.Unmarshal([]byte(input), &inv); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(inv.Hosts) != 2 {
		t.Fatalf("expected 2 hosts, got %d", len(inv.Hosts))
	}
	if inv.Hosts["node1"]["ansible_host"] != "10.0.0.1" {
		t.Fatalf("expected node1 ansible_host '10.0.0.1', got %v", inv.Hosts["node1"]["ansible_host"])
	}
	if inv.Vars["global_var"] != "hello" {
		t.Fatalf("expected global_var 'hello', got %v", inv.Vars["global_var"])
	}
	if len(inv.Groups) != 3 {
		t.Fatalf("expected 3 groups, got %d", len(inv.Groups))
	}
	if len(inv.Groups["all"].Groups) != 2 {
		t.Fatalf("expected 2 sub-groups in 'all', got %d", len(inv.Groups["all"].Groups))
	}
}
