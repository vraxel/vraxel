package executor

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"vraxel.io/vraxel/lib/ansible"
	"vraxel.io/vraxel/lib/ansible/connector"
	"vraxel.io/vraxel/lib/ansible/converter"
	"vraxel.io/vraxel/lib/ansible/modules"
	"vraxel.io/vraxel/lib/ansible/variable"
)

// recordItems registers a module under name that records the rendered "v"
// argument of every invocation, and returns the accumulator.
func recordItems(name string) *[]string {
	var mu sync.Mutex
	got := new([]string)
	modules.RegisterModule(name, func(_ context.Context, opts modules.ExecOptions) (string, string, error) {
		mu.Lock()
		defer mu.Unlock()
		*got = append(*got, fmt.Sprintf("%v", opts.Args["v"]))
		return "", "", nil
	})
	return got
}

// loopTask builds a single-host task running module over the given loop.
func loopTask(module string, loop any, kind ansible.LoopKind) ansible.TaskSpec {
	return ansible.TaskSpec{
		Hosts:    []string{"localhost"},
		Module:   ansible.ModuleRef{Name: module, Args: map[string]any{"v": "{{ .item }}"}},
		Loop:     loop,
		LoopKind: kind,
	}
}

// ---------------------------------------------------------------------------
// loop resolution
// ---------------------------------------------------------------------------

func TestLoop_TemplateResolvesToList(t *testing.T) {
	executor, v := setupTestExecutor(t)
	v.Merge(variable.MergeHostRuntimeVars("localhost", map[string]any{
		"mylist": []any{"a", "b", "c"},
	}))

	got := recordItems("_sem_loop_tmpl")
	executor.Exec(context.Background(), loopTask("_sem_loop_tmpl", "{{ .mylist }}", ansible.LoopKindLoop))

	if strings.Join(*got, ",") != "a,b,c" {
		t.Errorf("expected 3 iterations a,b,c; got %v", *got)
	}
}

func TestLoop_TemplateWithPipelineResolvesToList(t *testing.T) {
	executor, v := setupTestExecutor(t)
	v.Merge(variable.MergeHostRuntimeVars("localhost", map[string]any{
		"present": []any{"x", "y"},
	}))

	got := recordItems("_sem_loop_pipe")
	executor.Exec(context.Background(),
		loopTask("_sem_loop_pipe", "{{ .missing | default .present }}", ansible.LoopKindLoop))

	if strings.Join(*got, ",") != "x,y" {
		t.Errorf("expected pipeline to resolve to x,y; got %v", *got)
	}
}

func TestLoop_EmptyListRunsNothingAndSkips(t *testing.T) {
	executor, v := setupTestExecutor(t)
	v.Merge(variable.MergeHostRuntimeVars("localhost", map[string]any{
		"empty": []any{},
	}))

	got := recordItems("_sem_loop_empty")
	results := executor.Exec(context.Background(), loopTask("_sem_loop_empty", "{{ .empty }}", ansible.LoopKindLoop))

	if len(*got) != 0 {
		t.Errorf("expected no iterations for an empty loop, got %v", *got)
	}
	if results[0].Status != ansible.TaskStatusSkipped {
		t.Errorf("expected skipped status for an empty loop, got %q", results[0].Status)
	}
}

func TestLoop_WithItemsFlattensOneLevel(t *testing.T) {
	executor, v := setupTestExecutor(t)
	v.Merge(variable.MergeHostRuntimeVars("localhost", map[string]any{
		"nested": []any{[]any{"a", "b"}, "c"},
	}))

	got := recordItems("_sem_with_items")
	executor.Exec(context.Background(),
		loopTask("_sem_with_items", "{{ .nested }}", ansible.LoopKindItems))

	if strings.Join(*got, ",") != "a,b,c" {
		t.Errorf("expected with_items to flatten to a,b,c; got %v", *got)
	}
}

func TestLoop_WithDictYieldsKeyValueItems(t *testing.T) {
	executor, v := setupTestExecutor(t)
	v.Merge(variable.MergeHostRuntimeVars("localhost", map[string]any{
		"settings": map[string]any{"b": "2", "a": "1"},
	}))

	var mu sync.Mutex
	var got []string
	modules.RegisterModule("_sem_with_dict", func(_ context.Context, opts modules.ExecOptions) (string, string, error) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, fmt.Sprintf("%v", opts.Args["v"]))
		return "", "", nil
	})

	task := ansible.TaskSpec{
		Hosts:    []string{"localhost"},
		Module:   ansible.ModuleRef{Name: "_sem_with_dict", Args: map[string]any{"v": "{{ .item.key }}={{ .item.value }}"}},
		Loop:     "{{ .settings }}",
		LoopKind: ansible.LoopKindDict,
	}
	executor.Exec(context.Background(), task)

	// Keys are sorted, so the order is stable across runs.
	if strings.Join(got, ",") != "a=1,b=2" {
		t.Errorf("expected with_dict to yield a=1,b=2; got %v", got)
	}
}

func TestLoop_LoopVarAndIndexVar(t *testing.T) {
	executor, _ := setupTestExecutor(t)

	var mu sync.Mutex
	var got []string
	modules.RegisterModule("_sem_loop_var", func(_ context.Context, opts modules.ExecOptions) (string, string, error) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, fmt.Sprintf("%v", opts.Args["v"]))
		return "", "", nil
	})

	task := ansible.TaskSpec{
		Hosts:       []string{"localhost"},
		Module:      ansible.ModuleRef{Name: "_sem_loop_var", Args: map[string]any{"v": "{{ .idx }}:{{ .pkg }}"}},
		Loop:        []any{"nginx", "redis"},
		LoopControl: ansible.LoopControl{LoopVar: "pkg", IndexVar: "idx"},
	}
	executor.Exec(context.Background(), task)

	if strings.Join(got, ",") != "0:nginx,1:redis" {
		t.Errorf("expected loop_var/index_var to bind; got %v", got)
	}
}

func TestLoop_PlainListStillIterates(t *testing.T) {
	executor, _ := setupTestExecutor(t)

	got := recordItems("_sem_loop_plain")
	executor.Exec(context.Background(),
		loopTask("_sem_loop_plain", []any{"one", "two"}, ansible.LoopKindLoop))

	if strings.Join(*got, ",") != "one,two" {
		t.Errorf("expected plain list to iterate; got %v", *got)
	}
}

// ---------------------------------------------------------------------------
// changed_when
// ---------------------------------------------------------------------------

func TestChangedWhen_ConditionMarksChanged(t *testing.T) {
	executor, _ := setupTestExecutor(t)

	task := ansible.TaskSpec{
		Hosts:       []string{"localhost"},
		Module:      ansible.ModuleRef{Name: "command", Args: map[string]any{"cmd": "echo mutated"}},
		ChangedWhen: []string{`{{ eq .stdout "mutated" }}`},
	}
	results := executor.Exec(context.Background(), task)

	if !results[0].Changed {
		t.Error("expected Changed=true when changed_when holds")
	}
	if results[0].Status != ansible.TaskStatusChanged {
		t.Errorf("expected status changed, got %q", results[0].Status)
	}
}

func TestChangedWhen_FalseKeepsUnchanged(t *testing.T) {
	executor, _ := setupTestExecutor(t)

	task := ansible.TaskSpec{
		Hosts:       []string{"localhost"},
		Module:      ansible.ModuleRef{Name: "command", Args: map[string]any{"cmd": "echo mutated"}},
		ChangedWhen: []string{"{{ false }}"},
	}
	results := executor.Exec(context.Background(), task)

	if results[0].Changed {
		t.Error("expected Changed=false when changed_when is false")
	}
	if results[0].Status != ansible.TaskStatusOK {
		t.Errorf("expected status ok, got %q", results[0].Status)
	}
}

func TestChangedWhen_AbsentLeavesChangedFalse(t *testing.T) {
	executor, _ := setupTestExecutor(t)

	task := ansible.TaskSpec{
		Hosts:  []string{"localhost"},
		Module: ansible.ModuleRef{Name: "command", Args: map[string]any{"cmd": "echo hi"}},
	}
	results := executor.Exec(context.Background(), task)

	if results[0].Changed {
		t.Error("without changed_when the engine cannot know, so Changed must stay false")
	}
}

func TestChangedWhen_RegisteredAsVariable(t *testing.T) {
	executor, v := setupTestExecutor(t)

	task := ansible.TaskSpec{
		Hosts:       []string{"localhost"},
		Module:      ansible.ModuleRef{Name: "command", Args: map[string]any{"cmd": "echo hi"}},
		ChangedWhen: []string{"{{ true }}"},
		Register:    "r",
	}
	executor.Exec(context.Background(), task)

	vars := v.Get(variable.GetAllVariable("localhost")).(map[string]any)
	if got := vars["r"].(map[string]any)["changed"]; got != true {
		t.Errorf("expected registered changed=true, got %v", got)
	}
}

// ---------------------------------------------------------------------------
// environment
// ---------------------------------------------------------------------------

func TestEnvironment_ExportedToCommand(t *testing.T) {
	executor, _ := setupTestExecutor(t)

	task := ansible.TaskSpec{
		Hosts:       []string{"localhost"},
		Module:      ansible.ModuleRef{Name: "command", Args: map[string]any{"cmd": "echo $SEM_VAR"}},
		Environment: map[string]any{"SEM_VAR": "from-env"},
		Register:    "r",
	}
	executor.Exec(context.Background(), task)

	if got := executor.hostVars("localhost")["r"].(map[string]any)["stdout"]; got != "from-env" {
		t.Errorf("expected environment to reach the command, got %v", got)
	}
}

func TestEnvironment_ValuesAreTemplated(t *testing.T) {
	executor, v := setupTestExecutor(t)
	v.Merge(variable.MergeHostRuntimeVars("localhost", map[string]any{"proxy": "http://p:8080"}))

	task := ansible.TaskSpec{
		Hosts:       []string{"localhost"},
		Module:      ansible.ModuleRef{Name: "command", Args: map[string]any{"cmd": "echo $http_proxy"}},
		Environment: map[string]any{"http_proxy": "{{ .proxy }}"},
		Register:    "r",
	}
	executor.Exec(context.Background(), task)

	if got := executor.hostVars("localhost")["r"].(map[string]any)["stdout"]; got != "http://p:8080" {
		t.Errorf("expected templated env value, got %v", got)
	}
}

func TestEnvironment_WholeMapFromTemplate(t *testing.T) {
	executor, v := setupTestExecutor(t)
	v.Merge(variable.MergeHostRuntimeVars("localhost", map[string]any{
		"proxy_env": map[string]any{"SEM_FROM_MAP": "yes"},
	}))

	task := ansible.TaskSpec{
		Hosts:       []string{"localhost"},
		Module:      ansible.ModuleRef{Name: "command", Args: map[string]any{"cmd": "echo $SEM_FROM_MAP"}},
		Environment: "{{ .proxy_env }}",
		Register:    "r",
	}
	executor.Exec(context.Background(), task)

	if got := executor.hostVars("localhost")["r"].(map[string]any)["stdout"]; got != "yes" {
		t.Errorf("expected env map resolved from a template, got %v", got)
	}
}

// ---------------------------------------------------------------------------
// delegate_to
// ---------------------------------------------------------------------------

// markerConnector reports which host it belongs to, so a test can tell which
// connection a task actually ran over.
type markerConnector struct{ name string }

func (m *markerConnector) Init(context.Context) error  { return nil }
func (m *markerConnector) Close(context.Context) error { return nil }
func (m *markerConnector) ExecuteCommand(_ context.Context, _ string) ([]byte, []byte, error) {
	return []byte(m.name), nil, nil
}
func (m *markerConnector) PutFile(context.Context, []byte, string, fs.FileMode) error { return nil }
func (m *markerConnector) FetchFile(context.Context, string, io.Writer) error         { return nil }
func (m *markerConnector) Resize(context.Context, uint16, uint16) error               { return nil }

// setupMarkerExecutor builds an executor whose hosts each answer with their
// own name, plus an optional resolver for hosts outside the registry.
func setupMarkerExecutor(t *testing.T, resolve ConnectorResolver, hosts ...string) (*TaskExecutor, variable.Variable) {
	t.Helper()

	hostMap := make(map[string]map[string]any, len(hosts))
	conns := newConnectorRegistry()
	for _, h := range hosts {
		hostMap[h] = map[string]any{"connection": "local"}
		conns.Put(h, &markerConnector{name: h})
	}

	v := variable.New(ansible.Inventory{Hosts: hostMap})
	var logBuf bytes.Buffer
	return NewTaskExecutor(v, nil, conns, &logBuf, resolve), v
}

func TestDelegateTo_RunsOnDelegateButRegistersOnOriginal(t *testing.T) {
	executor, v := setupMarkerExecutor(t, nil, "web1", "db1")

	task := ansible.TaskSpec{
		Hosts:      []string{"web1"},
		Module:     ansible.ModuleRef{Name: "command", Args: map[string]any{"cmd": "whoami"}},
		DelegateTo: "db1",
		Register:   "r",
	}
	results := executor.Exec(context.Background(), task)

	if results[0].Host != "web1" {
		t.Errorf("result host = %q, want the original host web1", results[0].Host)
	}
	// The command ran over db1's connection...
	if got := extractString(results[0].Output, "stdout"); got != "db1" {
		t.Errorf("expected the task to run over db1's connection, got %q", got)
	}
	// ...but the registered variable belongs to web1.
	web1 := v.Get(variable.GetAllVariable("web1")).(map[string]any)
	if _, ok := web1["r"]; !ok {
		t.Error("expected the registered variable on the original host web1")
	}
	db1 := v.Get(variable.GetAllVariable("db1")).(map[string]any)
	if _, ok := db1["r"]; ok {
		t.Error("registered variable must not leak onto the delegate host")
	}
}

func TestDelegateTo_IsTemplated(t *testing.T) {
	executor, v := setupMarkerExecutor(t, nil, "web1", "db1")
	v.Merge(variable.MergeHostRuntimeVars("web1", map[string]any{"primary": "db1"}))

	task := ansible.TaskSpec{
		Hosts:      []string{"web1"},
		Module:     ansible.ModuleRef{Name: "command", Args: map[string]any{"cmd": "whoami"}},
		DelegateTo: "{{ .primary }}",
	}
	results := executor.Exec(context.Background(), task)

	if got := extractString(results[0].Output, "stdout"); got != "db1" {
		t.Errorf("expected templated delegate_to to resolve to db1, got %q", got)
	}
}

func TestDelegateTo_HostOutsideThePlayIsResolved(t *testing.T) {
	var created []string
	resolve := func(_ context.Context, host string) (connector.Connector, error) {
		created = append(created, host)
		return &markerConnector{name: "made:" + host}, nil
	}
	executor, _ := setupMarkerExecutor(t, resolve, "web1")

	task := ansible.TaskSpec{
		Hosts:      []string{"web1"},
		Module:     ansible.ModuleRef{Name: "command", Args: map[string]any{"cmd": "whoami"}},
		DelegateTo: "bastion",
	}
	results := executor.Exec(context.Background(), task)

	if got := extractString(results[0].Output, "stdout"); got != "made:bastion" {
		t.Errorf("expected the resolver's connector, got %q", got)
	}
	if len(created) != 1 || created[0] != "bastion" {
		t.Errorf("expected the resolver to be asked for bastion, got %v", created)
	}
}

func TestDelegateTo_UnknownHostWithoutResolverFails(t *testing.T) {
	executor, _ := setupMarkerExecutor(t, nil, "web1")

	task := ansible.TaskSpec{
		Hosts:      []string{"web1"},
		Module:     ansible.ModuleRef{Name: "command", Args: map[string]any{"cmd": "whoami"}},
		DelegateTo: "nowhere",
	}
	results := executor.Exec(context.Background(), task)

	if results[0].Status != ansible.TaskStatusFailed {
		t.Fatalf("expected failure for an unreachable delegate, got %q", results[0].Status)
	}
	if !strings.Contains(results[0].Error, "nowhere") {
		t.Errorf("expected the error to name the delegate, got %q", results[0].Error)
	}
}

func TestDelegateTo_PlaybookClosesDelegateConnector(t *testing.T) {
	// A delegate outside the play still has to be torn down with it.
	inv := ansible.Inventory{
		Hosts: map[string]map[string]any{
			"localhost": {"connection": "local"},
			"other":     {"connection": "local"},
		},
	}
	exec, _ := setupPlaybookExecutor(t, inv, nil)

	playbook := &ansible.Playbook{
		Play: []ansible.Play{{
			Base:     ansible.Base{Name: "delegating play"},
			PlayHost: ansible.PlayHost{Hosts: []string{"localhost"}},
			Tasks: []ansible.Block{{
				BlockBase: ansible.BlockBase{
					Base:      ansible.Base{Name: "delegate somewhere else"},
					Delegable: ansible.Delegable{DelegateTo: "other"},
				},
				Task: ansible.Task{UnknownField: map[string]any{
					"command": map[string]any{"cmd": "echo delegated"},
				}},
			}},
		}},
	}

	result, err := exec.Execute(context.Background(), playbook)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !result.Success {
		t.Fatalf("playbook failed: %s", result.Error)
	}
	if left := exec.connectors.Snapshot(); len(left) != 0 {
		t.Errorf("expected every connector closed and dropped, %d left", len(left))
	}
}

// ---------------------------------------------------------------------------
// Load-time rejection reaches files the top-level playbook never shows
// ---------------------------------------------------------------------------

// runPlaybookWithFiles executes a one-task play against an in-memory source.
func runPlaybookWithFiles(t *testing.T, files map[string][]byte, task ansible.Block) error {
	t.Helper()

	exec, _ := setupPlaybookExecutor(t, localhostInventory(), files)
	playbook := &ansible.Playbook{
		Play: []ansible.Play{{
			Base:     ansible.Base{Name: "p"},
			PlayHost: ansible.PlayHost{Hosts: []string{"localhost"}},
			Tasks:    []ansible.Block{task},
		}},
	}
	result, err := exec.Execute(context.Background(), playbook)
	if err != nil {
		return err
	}
	if !result.Success {
		return fmt.Errorf("%s", result.Error)
	}
	return nil
}

func TestUnsupported_RejectedInsideIncludeTasks(t *testing.T) {
	files := map[string][]byte{
		"tasks/extra.yml": []byte(`
- name: globbing task
  command:
    cmd: "echo {{ .item }}"
  with_fileglob: "/etc/*.conf"
`),
	}
	err := runPlaybookWithFiles(t, files, ansible.Block{IncludeTasks: "tasks/extra.yml"})

	if err == nil {
		t.Fatal("expected the included file's with_fileglob to be rejected")
	}
	if !strings.Contains(err.Error(), "with_fileglob") {
		t.Errorf("error should name the directive, got %q", err.Error())
	}
}

func TestImportTasks_LoadsLikeIncludeTasks(t *testing.T) {
	files := map[string][]byte{
		"tasks/imported.yml": []byte(`
- name: imported task
  command:
    cmd: echo imported
  register: imported_out
`),
	}

	pb, err := converter.ParsePlaybook([]byte(`
- name: importing
  hosts:
    - localhost
  gather_facts: false
  tasks:
    - import_tasks: tasks/imported.yml
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	exec, _ := setupPlaybookExecutor(t, localhostInventory(), files)
	result, err := exec.Execute(context.Background(), pb)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !result.Success {
		t.Fatalf("playbook failed: %s", result.Error)
	}

	vars := exec.variable.Get(variable.GetAllVariable("localhost")).(map[string]any)
	reg, ok := vars["imported_out"].(map[string]any)
	if !ok {
		t.Fatal("expected the imported task to have run and registered")
	}
	if reg["stdout"] != "imported" {
		t.Errorf("imported task stdout = %v, want %q", reg["stdout"], "imported")
	}
}

func TestUnsupported_RejectedInsideRole(t *testing.T) {
	files := map[string][]byte{
		"roles/svc/tasks/main.yml": []byte(`
- name: notify task
  command:
    cmd: echo hi
  notify: restart svc
`),
	}

	exec, _ := setupPlaybookExecutor(t, localhostInventory(), files)
	playbook := &ansible.Playbook{
		Play: []ansible.Play{{
			Base:     ansible.Base{Name: "p"},
			PlayHost: ansible.PlayHost{Hosts: []string{"localhost"}},
			Roles:    []ansible.Role{{RoleInfo: ansible.RoleInfo{Role: "svc"}}},
		}},
	}

	result, err := exec.Execute(context.Background(), playbook)
	failed := err != nil || !result.Success
	if !failed {
		t.Fatal("expected the role's notify to be rejected")
	}

	msg := result.Error
	if err != nil {
		msg = err.Error()
	}
	if !strings.Contains(msg, "notify") {
		t.Errorf("error should name the directive, got %q", msg)
	}
}

// ---------------------------------------------------------------------------
// Torture playbook: the whole chain, YAML -> converter -> planner -> executor
// ---------------------------------------------------------------------------

const torturePlaybook = `
- name: torture
  hosts:
    - localhost
  gather_facts: false
  vars:
    pkgs:
      - alpha
      - beta
    nested:
      - [a, b]
      - [c]
    settings:
      k2: v2
      k1: v1
  tasks:
    - name: loop over a templated list
      command:
        cmd: "echo {{ .item }}"
      loop: "{{ .pkgs }}"
      register: loop_out

    - name: with_items flattens one level
      command:
        cmd: "echo {{ .item }}"
      with_items: "{{ .nested }}"
      register: items_out

    - name: with_dict yields key/value
      command:
        cmd: "echo {{ .item.key }}={{ .item.value }}"
      with_dict: "{{ .settings }}"
      register: dict_out

    - name: environment reaches the shell
      command:
        cmd: "echo $TORTURE_VAR"
      environment:
        TORTURE_VAR: "torture-env"
      register: env_out

    - name: changed_when false stays unchanged
      command:
        cmd: "echo noop"
      changed_when: false
      register: nochange_out

    - name: changed_when expression marks changed
      command:
        cmd: "echo mutated"
      changed_when: '{{ eq .stdout "mutated" }}'
      register: change_out

    - name: loop_var renames the item
      command:
        cmd: "echo {{ .svc }}"
      loop:
        - svc-a
      loop_control:
        loop_var: svc
      register: loopvar_out
`

// TestStatDrivesWhen checks the pattern the README documents: stat writes
// JSON, register_type turns it into a value, and when indexes into it. The
// three land in different layers, so only an end-to-end run proves they meet.
func TestStatDrivesWhen(t *testing.T) {
	dir := t.TempDir()
	present := filepath.Join(dir, "present.conf")
	if err := os.WriteFile(present, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "missing.conf")

	pb, err := converter.ParsePlaybook(fmt.Appendf(nil, `
- name: stat drives when
  hosts:
    - localhost
  gather_facts: false
  tasks:
    - stat:
        path: %s
      register: found
      register_type: json
    - stat:
        path: %s
      register: absent
      register_type: json
    - set_fact:
        saw_present: "yes"
      when: '{{ .found.stdout.exists }}'
    - set_fact:
        saw_absent: "yes"
      when: '{{ .absent.stdout.exists }}'
`, present, missing))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	exec, _ := setupPlaybookExecutor(t, localhostInventory(), nil)
	result, err := exec.Execute(context.Background(), pb)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !result.Success {
		t.Fatalf("playbook failed: %s", result.Error)
	}

	vars := exec.variable.Get(variable.GetAllVariable("localhost")).(map[string]any)
	if vars["saw_present"] != "yes" {
		t.Error("when should have fired for the existing path")
	}
	if _, ok := vars["saw_absent"]; ok {
		t.Error("when must not fire for the missing path")
	}
}

func TestTorturePlaybook(t *testing.T) {
	pb, err := converter.ParsePlaybook([]byte(torturePlaybook))
	if err != nil {
		t.Fatalf("parse torture playbook: %v", err)
	}

	exec, _ := setupPlaybookExecutor(t, localhostInventory(), nil)
	result, err := exec.Execute(context.Background(), pb)
	if err != nil {
		t.Fatalf("execute torture playbook: %v", err)
	}
	if !result.Success {
		t.Fatalf("torture playbook failed: %s", result.Error)
	}

	vars := exec.variable.Get(variable.GetAllVariable("localhost")).(map[string]any)
	stdoutOf := func(reg string) string {
		r, ok := vars[reg].(map[string]any)
		if !ok {
			t.Fatalf("register %q missing, got %T", reg, vars[reg])
		}
		return fmt.Sprintf("%v", r["stdout"])
	}
	changedOf := func(reg string) any {
		return vars[reg].(map[string]any)["changed"]
	}

	// register keeps the last loop iteration, which pins the item order.
	for reg, want := range map[string]string{
		"loop_out":    "beta",
		"items_out":   "c",
		"dict_out":    "k2=v2",
		"env_out":     "torture-env",
		"loopvar_out": "svc-a",
	} {
		if got := stdoutOf(reg); got != want {
			t.Errorf("%s: expected stdout %q, got %q", reg, want, got)
		}
	}

	if changedOf("nochange_out") != false {
		t.Errorf("nochange_out: expected changed=false, got %v", changedOf("nochange_out"))
	}
	if changedOf("change_out") != true {
		t.Errorf("change_out: expected changed=true, got %v", changedOf("change_out"))
	}
}
