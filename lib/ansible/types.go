package ansible

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// ============================================================================
// Base types
// ============================================================================

// Base defines common fields shared by plays, blocks, and tasks.
type Base struct {
	Name string `yaml:"name,omitempty"`

	// connection/transport
	Connection string `yaml:"connection,omitempty"`
	Port       int    `yaml:"port,omitempty"`
	RemoteUser string `yaml:"remote_user,omitempty"`

	// variables
	Vars Vars `yaml:"vars,omitempty"`

	// flags and misc. settings
	//
	// Environment accepts the shapes Ansible allows: a mapping, a list of
	// mappings, or a template string resolving to either. It stays untyped
	// because the values are rendered per host at execution time.
	Environment    any   `yaml:"environment,omitempty"`
	NoLog          bool  `yaml:"no_log,omitempty"`
	RunOnce        bool  `yaml:"run_once,omitempty"`
	IgnoreErrors   *bool `yaml:"ignore_errors,omitempty"`
	CheckMode      bool  `yaml:"check_mode,omitempty"`
	Diff           bool  `yaml:"diff,omitempty"`
	AnyErrorsFatal bool  `yaml:"any_errors_fatal,omitempty"`
	Throttle       int   `yaml:"throttle,omitempty"`
	Timeout        int   `yaml:"timeout,omitempty"`

	// Debugger invokes a debugger on tasks.
	Debugger string `yaml:"debugger,omitempty"`

	// privilege escalation
	Become       bool   `yaml:"become,omitempty"`
	BecomeMethod string `yaml:"become_method,omitempty"`
	BecomeUser   string `yaml:"become_user,omitempty"`
	BecomeFlags  string `yaml:"become_flags,omitempty"`
	BecomeExe    string `yaml:"become_exe,omitempty"`
}

// Vars is a custom type to hold a list of YAML nodes representing variables.
// This allows for flexible unmarshalling of various YAML structures into Vars.
type Vars struct {
	Nodes []yaml.Node
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for Vars.
// It appends the unmarshalled YAML node to the Vars.Nodes slice.
func (v *Vars) UnmarshalYAML(node *yaml.Node) error {
	v.Nodes = append(v.Nodes, *node)
	return nil
}

// ============================================================================
// Conditional
// ============================================================================

// Conditional holds when conditions for a block or task.
type Conditional struct {
	When When `yaml:"when,omitempty"`
}

// When supports both scalar string and string array YAML values.
type When struct {
	Data []string
}

// UnmarshalYAML parses a when condition from YAML.
// Scalar values are wrapped in template syntax if not already.
func (w *When) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		if IsTmplSyntax(node.Value) {
			w.Data = []string{node.Value}
		} else {
			w.Data = []string{"{{ " + node.Value + " }}"}
		}
	case yaml.SequenceNode:
		if err := node.Decode(&w.Data); err != nil {
			return fmt.Errorf("failed to decode when sequence: %w", err)
		}
		for i, v := range w.Data {
			if !IsTmplSyntax(v) {
				w.Data[i] = ParseTmplSyntax(v)
			}
		}
	default:
		return fmt.Errorf("unsupported type for when, expected string or array of strings")
	}

	return nil
}

// ============================================================================
// Taggable
// ============================================================================

// Special tag constants.
const (
	// AlwaysTag always runs.
	AlwaysTag = "always"
	// NeverTag never runs.
	NeverTag = "never"
	// AllTag represents all tags.
	AllTag = "all"
	// TaggedTag represents items that have tags.
	TaggedTag = "tagged"
)

// Taggable holds tags that determine whether a block/task should execute.
type Taggable struct {
	Tags Tags `yaml:"tags,omitempty"`
}

// Tags supports both scalar string and string array YAML values.
type Tags struct {
	Data []string
}

// UnmarshalYAML parses tags from YAML. Supports a single string or a list of strings.
func (t *Tags) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		t.Data = []string{node.Value}
		return nil
	case yaml.SequenceNode:
		return node.Decode(&t.Data)
	default:
		return fmt.Errorf("unsupported type for tags, expected string or array of strings")
	}
}

// IsEnabled checks if the tags allow execution given onlyTags and skipTags filters.
func (t Tags) IsEnabled(onlyTags []string, skipTags []string) bool {
	shouldRun := true

	if len(onlyTags) > 0 {
		switch {
		case slices.Contains(t.Data, AlwaysTag):
			shouldRun = true
		case slices.Contains(onlyTags, AllTag) && !slices.Contains(t.Data, NeverTag):
			shouldRun = true
		case slices.Contains(onlyTags, TaggedTag) && !slices.Contains(t.Data, NeverTag):
			shouldRun = true
		case !isdisjoint(onlyTags, t.Data):
			shouldRun = true
		default:
			shouldRun = false
		}
	}

	if shouldRun && len(skipTags) > 0 {
		switch {
		case slices.Contains(skipTags, AllTag) &&
			(!slices.Contains(t.Data, AlwaysTag) || !slices.Contains(skipTags, AlwaysTag)):
			shouldRun = false
		case !isdisjoint(skipTags, t.Data):
			shouldRun = false
		case slices.Contains(skipTags, TaggedTag) && len(t.Data) > 0:
			shouldRun = false
		}
	}

	return shouldRun
}

// JoinTag merges tags from another Tags, deduplicating entries.
func (t Tags) JoinTag(other Tags) Tags {
	result := Tags{Data: make([]string, len(t.Data))}
	copy(result.Data, t.Data)
	for _, tag := range other.Data {
		if !slices.Contains(result.Data, tag) {
			result.Data = append(result.Data, tag)
		}
	}
	return result
}

// isdisjoint returns true if a and b have no elements in common.
func isdisjoint(a, b []string) bool {
	for _, s := range a {
		if slices.Contains(b, s) {
			return false
		}
	}
	return true
}

// ============================================================================
// Notifiable
// ============================================================================

// Notifiable holds the notify field for triggering handlers.
type Notifiable struct {
	Notify string `yaml:"notify,omitempty"`
}

// ============================================================================
// Delegable
// ============================================================================

// Delegable holds delegation settings for a task.
type Delegable struct {
	DelegateTo    string `yaml:"delegate_to,omitempty"`
	DelegateFacts bool   `yaml:"delegate_facts,omitempty"`
}

// ============================================================================
// LoopControl
// ============================================================================

// LoopControl defines loop behavior settings.
type LoopControl struct {
	LoopVar          string  `yaml:"loop_var,omitempty"`
	IndexVar         string  `yaml:"index_var,omitempty"`
	Label            string  `yaml:"label,omitempty"`
	Pause            float32 `yaml:"pause,omitempty"`
	Extended         bool    `yaml:"extended,omitempty"`
	ExtendedAllitems bool    `yaml:"extended_allitems,omitempty"`
}

// ============================================================================
// Block types
// ============================================================================

// Block represents an Ansible block, which can be a task, an include_tasks
// directive, or a block/rescue/always structure.
type Block struct {
	BlockBase
	// If it has Block, Task should be empty.
	Task
	// IncludeTasks holds the target of include_tasks or its accepted
	// spelling import_tasks; see Block.UnmarshalYAML.
	IncludeTasks string `yaml:"include_tasks,omitempty"`

	BlockInfo
}

// BlockBase holds the common fields embedded in every block.
type BlockBase struct {
	Base        `yaml:",inline"`
	Conditional `yaml:",inline"`
	Taggable    `yaml:",inline"`
	Notifiable  `yaml:",inline"`
	Delegable   `yaml:",inline"`
}

// BlockInfo holds block/rescue/always sub-blocks.
type BlockInfo struct {
	Block  []Block `yaml:"block,omitempty"`
	Rescue []Block `yaml:"rescue,omitempty"`
	Always []Block `yaml:"always,omitempty"`
}

// Task holds task-specific fields beyond the base block fields.
type Task struct {
	AsyncVal    int         `yaml:"async,omitempty"`
	ChangedWhen When        `yaml:"changed_when,omitempty"`
	Delay       int         `yaml:"delay,omitempty"`
	FailedWhen  When        `yaml:"failed_when,omitempty"`
	Loop        any         `yaml:"loop,omitempty"`
	WithItems   any         `yaml:"with_items,omitempty"`
	WithDict    any         `yaml:"with_dict,omitempty"`
	LoopControl LoopControl `yaml:"loop_control,omitempty"`
	Poll        int         `yaml:"poll,omitempty"`
	Register    string      `yaml:"register,omitempty"`
	// RegisterType specifies how to register value to variable. Supports: string (default), json, yaml.
	RegisterType string `yaml:"register_type,omitempty"`
	Retries      int    `yaml:"retries,omitempty"`
	Until        When   `yaml:"until,omitempty"`

	// UnknownField stores undefined fields (module names and their args).
	UnknownField map[string]any `yaml:"-"`
}

// UnmarshalYAML implements custom YAML parsing for Block.
// It handles three forms:
//  1. include_tasks: references another task file
//  2. block/rescue/always: a block structure
//  3. task: a module invocation with known and unknown fields
func (b *Block) UnmarshalYAML(node *yaml.Node) error {
	// Decode base info (Base, Conditional, Taggable, Notifiable, Delegable).
	if err := node.Decode(&b.BlockBase); err != nil {
		return fmt.Errorf("failed to decode block base: %w", err)
	}

	for i := 0; i < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		valueNode := node.Content[i+1]

		switch keyNode.Value {
		// import_tasks is accepted as a spelling of include_tasks. Ansible
		// separates them by when the file is read (import at parse time,
		// include at run time); this engine reads it at run time either way,
		// so the difference only shows in how tags and when propagate.
		case "include_tasks", "import_tasks":
			b.IncludeTasks = valueNode.Value
			return nil

		case "block":
			return node.Decode(&b.BlockInfo)
		}
	}

	if err := node.Decode(&b.Task); err != nil {
		return fmt.Errorf("failed to decode task: %w", err)
	}
	b.UnknownField = collectUnknownFields(node,
		append(getFieldNames(reflect.TypeOf(BlockBase{})), getFieldNames(reflect.TypeOf(Task{}))...))

	return nil
}

// collectUnknownFields traverses a YAML mapping node and collects fields
// that are not in the excludeFields list.
func collectUnknownFields(node *yaml.Node, excludeFields []string) map[string]any {
	unknown := make(map[string]any)
	excludeSet := make(map[string]struct{}, len(excludeFields))
	for _, field := range excludeFields {
		excludeSet[field] = struct{}{}
	}

	for i := 0; i < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		valueNode := node.Content[i+1]

		if _, excluded := excludeSet[keyNode.Value]; excluded {
			continue
		}

		var value any
		if err := valueNode.Decode(&value); err == nil {
			unknown[keyNode.Value] = value
		} else {
			unknown[keyNode.Value] = fmt.Sprintf("failed to decode: %v", err)
		}
	}

	return unknown
}

// getFieldNames returns YAML tag names for all fields in a struct type,
// recursively processing inline-tagged embedded fields.
func getFieldNames(t reflect.Type) []string {
	var fields []string
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		yamlTag := field.Tag.Get("yaml")
		if yamlTag != "" {
			if strings.Contains(yamlTag, "inline") {
				inlineFields := getFieldNames(field.Type)
				fields = append(fields, inlineFields...)
				continue
			}
			tagName := strings.Split(yamlTag, ",")[0]
			if tagName != "" && tagName != "-" {
				fields = append(fields, tagName)
			}
		}
	}
	return fields
}

// ============================================================================
// Play types
// ============================================================================

// Play represents a single Ansible play.
type Play struct {
	ImportPlaybook string `yaml:"import_playbook,omitempty"`

	Base     `yaml:",inline"`
	Taggable `yaml:",inline"`

	PlayHost PlayHost `yaml:"hosts,omitempty"`

	// Facts
	GatherFacts bool `yaml:"gather_facts,omitempty"`

	// Variable Attributes
	VarsFiles []string `yaml:"vars_files,omitempty"`

	// Role Attributes
	Roles []Role `yaml:"roles,omitempty"`

	// Block (Task) Lists Attributes
	Handlers  []Block `yaml:"handlers,omitempty"`
	PreTasks  []Block `yaml:"pre_tasks,omitempty"`
	PostTasks []Block `yaml:"post_tasks,omitempty"`
	Tasks     []Block `yaml:"tasks,omitempty"`

	// Flag/Setting Attributes
	ForceHandlers     bool       `yaml:"force_handlers,omitempty"`
	MaxFailPercentage float32    `yaml:"max_fail_percentage,omitempty"`
	Serial            PlaySerial `yaml:"serial,omitempty"`
	Strategy          string     `yaml:"strategy,omitempty"`
	Order             string     `yaml:"order,omitempty"`
}

// PlayHost supports both a single host string and a list of hosts in YAML.
type PlayHost struct {
	Hosts []string
}

// UnmarshalYAML parses hosts from YAML. Accepts a scalar or a sequence.
func (p *PlayHost) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		p.Hosts = []string{node.Value}
		return nil
	case yaml.SequenceNode:
		return node.Decode(&p.Hosts)
	default:
		return fmt.Errorf("unsupported type for hosts, expected string or string array")
	}
}

// PlaySerial supports both a single value and a sequence in YAML.
type PlaySerial struct {
	Data []any
}

// UnmarshalYAML parses serial from YAML. Accepts a scalar or a sequence.
func (s *PlaySerial) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		s.Data = []any{node.Value}
		return nil
	case yaml.SequenceNode:
		return node.Decode(&s.Data)
	default:
		return fmt.Errorf("unsupported type for serial, expected value or array")
	}
}

// ============================================================================
// Playbook types
// ============================================================================

// Playbook wraps a list of plays parsed from YAML.
type Playbook struct {
	Play []Play
}

// Validate validates the playbook structure. It removes plays that are
// import_playbook references (which should have been resolved already)
// and checks that remaining plays have hosts defined.
func (p *Playbook) Validate() error {
	for _, play := range p.Play {
		// import_playbook used to be dropped here, which turned "run these
		// other plays too" into "run nothing" without a word.
		if play.ImportPlaybook != "" {
			return unsupported(describe("play", play.Name), "import_playbook",
				unsupportedDirectives["import_playbook"])
		}

		if len(play.PlayHost.Hosts) == 0 {
			return fmt.Errorf("playbook's hosts must not be empty")
		}
		if err := ValidatePlay(play); err != nil {
			return err
		}
		for _, blocks := range [][]Block{play.PreTasks, play.Tasks, play.PostTasks} {
			if err := ValidateBlocks(blocks); err != nil {
				return err
			}
		}
	}
	return nil
}

// IsTmplSyntax checks if the string contains template syntax ({{ }}).
func IsTmplSyntax(s string) bool {
	return strings.Contains(s, "{{") && strings.Contains(s, "}}")
}

// ParseTmplSyntax wraps a string with template syntax delimiters.
func ParseTmplSyntax(s string) string {
	return "{{ " + s + " }}"
}

// TrimTmplSyntax removes template syntax delimiters from a string.
func TrimTmplSyntax(s string) string {
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(s, "{{"), "}}"))
}

// ============================================================================
// Role types
// ============================================================================

// Role represents an Ansible role reference. It supports both a simple
// scalar form (just a role name) and a full mapping with all role fields.
type Role struct {
	RoleInfo
}

// RoleInfo holds the full definition of a role.
type RoleInfo struct {
	Base        `yaml:",inline"`
	Conditional `yaml:",inline"`
	Taggable    `yaml:",inline"`

	// Role is the role name reference in a playbook.
	Role string `yaml:"role,omitempty"`

	// Dependencies are other roles this role depends on.
	Dependencies []Role `yaml:"dependencies,omitempty"`

	// Blocks are the task blocks within the role.
	Blocks []Block
}

// UnmarshalYAML parses a role from YAML.
// A scalar value is treated as the role name.
// A mapping is decoded into the full RoleInfo.
func (r *Role) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		r.Role = node.Value
	case yaml.MappingNode:
		return node.Decode(&r.RoleInfo)
	}
	return nil
}

// ============================================================================
// Inventory types (replacing Kubernetes CRD)
// ============================================================================

// Inventory stores hosts and variables for playbook execution.
type Inventory struct {
	// Hosts maps hostname to host-specific variables.
	Hosts map[string]map[string]any `json:"hosts" yaml:"hosts"`
	// Vars are inventory-level variables. Priority: host vars > group vars > inventory vars.
	Vars map[string]any `json:"vars,omitempty" yaml:"vars,omitempty"`
	// Groups maps group names to their definitions.
	Groups map[string]InventoryGroup `json:"groups,omitempty" yaml:"groups,omitempty"`
}

// InventoryGroup defines a group of hosts with optional sub-groups and variables.
type InventoryGroup struct {
	Groups []string       `json:"groups,omitempty" yaml:"groups,omitempty"`
	Hosts  []string       `json:"hosts" yaml:"hosts"`
	Vars   map[string]any `json:"vars,omitempty" yaml:"vars,omitempty"`
}

// GetHostsFromGroup flattens a specific inventory group with de-duplication,
// recursively resolving sub-groups.
func GetHostsFromGroup(inv *Inventory, groupName string, unavailableHosts, unavailableGroups map[string]struct{}) []string {
	var hosts = make([]string, 0)
	if v, ok := inv.Groups[groupName]; ok {
		unavailableGroups[groupName] = struct{}{}
		for _, cg := range v.Groups {
			if _, exist := unavailableGroups[cg]; !exist {
				unavailableGroups[cg] = struct{}{}
				hosts = append(hosts, GetHostsFromGroup(inv, cg, unavailableHosts, unavailableGroups)...)
			}
		}

		hosts = append(hosts, getHostsFromGroupValidHosts(inv, v.Hosts, unavailableHosts)...)
	}
	return hosts
}

// getHostsFromGroupValidHosts collects the direct, de-duplicated, defined
// hosts of a group. Extracted from GetHostsFromGroup to cut its nesting.
func getHostsFromGroupValidHosts(inv *Inventory, groupHosts []string, unavailableHosts map[string]struct{}) []string {
	validHosts := make([]string, 0)
	for _, hostname := range groupHosts {
		if _, ok := inv.Hosts[hostname]; ok {
			if _, exist := unavailableHosts[hostname]; !exist {
				unavailableHosts[hostname] = struct{}{}
				validHosts = append(validHosts, hostname)
			}
		}
	}
	return validHosts
}

// ============================================================================
// Execution types
// ============================================================================

// TaskStatus represents the status of a task execution.
type TaskStatus string

const (
	TaskStatusPending     TaskStatus = "pending"
	TaskStatusRunning     TaskStatus = "running"
	TaskStatusOK          TaskStatus = "ok"
	TaskStatusChanged     TaskStatus = "changed"
	TaskStatusFailed      TaskStatus = "failed"
	TaskStatusSkipped     TaskStatus = "skipped"
	TaskStatusUnreachable TaskStatus = "unreachable"
)

// ModuleRef identifies a module and its arguments.
type ModuleRef struct {
	Name string `json:"name" yaml:"name"`
	Args any    `json:"args,omitempty" yaml:"args,omitempty"`
}

// LoopKind records which directive produced TaskSpec.Loop. The directives
// share one Loop field but not one item semantics, so the executor needs to
// know which one was written: with_items flattens one level, with_dict turns
// a mapping into key/value items, and plain loop iterates as-is.
type LoopKind string

const (
	// LoopKindLoop is the plain "loop:" directive (iterate the list as-is).
	LoopKindLoop LoopKind = ""
	// LoopKindItems is "with_items:" (iterate the list flattened one level).
	LoopKindItems LoopKind = "items"
	// LoopKindDict is "with_dict:" (iterate a mapping as {key, value} items).
	LoopKindDict LoopKind = "dict"
)

// TaskSpec is the internal specification of a task to be executed.
type TaskSpec struct {
	Name         string      `json:"name,omitempty" yaml:"name,omitempty"`
	Module       ModuleRef   `json:"module" yaml:"module"`
	Hosts        []string    `json:"hosts,omitempty" yaml:"hosts,omitempty"`
	When         []string    `json:"when,omitempty" yaml:"when,omitempty"`
	FailedWhen   []string    `json:"failed_when,omitempty" yaml:"failed_when,omitempty"`
	Loop         any         `json:"loop,omitempty" yaml:"loop,omitempty"`
	LoopKind     LoopKind    `json:"loop_kind,omitempty" yaml:"loop_kind,omitempty"`
	LoopControl  LoopControl `json:"loop_control,omitempty" yaml:"loop_control,omitempty"`
	ChangedWhen  []string    `json:"changed_when,omitempty" yaml:"changed_when,omitempty"`
	Environment  any         `json:"environment,omitempty" yaml:"environment,omitempty"`
	Register     string      `json:"register,omitempty" yaml:"register,omitempty"`
	RegisterType string      `json:"register_type,omitempty" yaml:"register_type,omitempty"`
	Retries      int         `json:"retries,omitempty" yaml:"retries,omitempty"`
	Delay        int         `json:"delay,omitempty" yaml:"delay,omitempty"`
	Until        []string    `json:"until,omitempty" yaml:"until,omitempty"`
	Become       bool        `json:"become,omitempty" yaml:"become,omitempty"`
	BecomeUser   string      `json:"become_user,omitempty" yaml:"become_user,omitempty"`
	DelegateTo   string      `json:"delegate_to,omitempty" yaml:"delegate_to,omitempty"`
	Async        int         `json:"async,omitempty" yaml:"async,omitempty"`
	Poll         int         `json:"poll,omitempty" yaml:"poll,omitempty"`
	Notify       string      `json:"notify,omitempty" yaml:"notify,omitempty"`
	IgnoreErrors *bool       `json:"ignore_errors,omitempty" yaml:"ignore_errors,omitempty"`
}

// TaskResult holds the result of executing a single task on a single host.
type TaskResult struct {
	Host    string         `json:"host"`
	Status  TaskStatus     `json:"status"`
	Changed bool           `json:"changed"`
	Output  map[string]any `json:"output,omitempty"`
	Error   string         `json:"error,omitempty"`
}

// LoopResult holds the results for a single loop iteration.
type LoopResult struct {
	Item   any        `json:"item"`
	Result TaskResult `json:"result"`
}

// PlaybookStats tracks aggregate execution counts across hosts.
type PlaybookStats struct {
	OK          map[string]int `json:"ok"`
	Changed     map[string]int `json:"changed"`
	Unreachable map[string]int `json:"unreachable"`
	Failed      map[string]int `json:"failed"`
	Skipped     map[string]int `json:"skipped"`
}

// NewPlaybookStats creates a new PlaybookStats with initialized maps.
func NewPlaybookStats() *PlaybookStats {
	return &PlaybookStats{
		OK:          make(map[string]int),
		Changed:     make(map[string]int),
		Unreachable: make(map[string]int),
		Failed:      make(map[string]int),
		Skipped:     make(map[string]int),
	}
}

// PlaybookResult holds the overall result of a playbook execution.
type PlaybookResult struct {
	StartTime time.Time      `json:"start_time"`
	EndTime   time.Time      `json:"end_time"`
	Stats     *PlaybookStats `json:"stats"`
	Success   bool           `json:"success"`
	Error     string         `json:"error,omitempty"`
}
