package converter

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"vraxel.io/vraxel/lib/ansible"
)

// ParsePlaybook parses YAML bytes into a Playbook struct.
func ParsePlaybook(data []byte) (*ansible.Playbook, error) {
	var plays []ansible.Play
	if err := yaml.Unmarshal(data, &plays); err != nil {
		return nil, fmt.Errorf("parse playbook: %w", err)
	}
	pb := &ansible.Playbook{Play: plays}
	if err := pb.Validate(); err != nil {
		return nil, err
	}
	return pb, nil
}

// BlockToTaskSpec converts a Block to a TaskSpec, identifying the module from UnknownField.
// The moduleFinder function checks if a name is a registered module.
func BlockToTaskSpec(block ansible.Block, hosts []string, role string, moduleFinder func(string) bool) ansible.TaskSpec {
	spec := ansible.TaskSpec{
		Name:         block.Name,
		Hosts:        hosts,
		When:         block.When.Data,
		FailedWhen:   block.FailedWhen.Data,
		ChangedWhen:  block.ChangedWhen.Data,
		Environment:  block.Environment,
		LoopControl:  block.Task.LoopControl,
		Register:     block.Register,
		RegisterType: block.RegisterType,
		Retries:      block.Retries,
		Delay:        block.Delay,
		Until:        block.Until.Data,
		Become:       block.Become,
		BecomeUser:   block.BecomeUser,
		DelegateTo:   block.DelegateTo,
		Async:        block.AsyncVal,
		Poll:         block.Poll,
		Notify:       block.Notify,
		IgnoreErrors: block.IgnoreErrors,
	}

	// loop / with_items / with_dict share TaskSpec.Loop but keep their own
	// item semantics, recorded in LoopKind. Only one may win; loop first.
	switch {
	case block.Loop != nil:
		spec.Loop, spec.LoopKind = block.Loop, ansible.LoopKindLoop
	case block.WithItems != nil:
		spec.Loop, spec.LoopKind = block.WithItems, ansible.LoopKindItems
	case block.WithDict != nil:
		spec.Loop, spec.LoopKind = block.WithDict, ansible.LoopKindDict
	}

	// Identify module from UnknownField
	for name, args := range block.UnknownField {
		if moduleFinder(name) {
			argsMap := make(map[string]any)
			switch v := args.(type) {
			case map[string]any:
				argsMap = v
			case string:
				argsMap[name] = v
			default:
				argsMap[name] = v
			}
			spec.Module = ansible.ModuleRef{Name: name, Args: argsMap}
			break
		}
	}

	return spec
}

// GroupHostBySerial splits hosts into batches based on serial specification.
// Serial items can be integers or percentage strings (e.g., "50%").
// The last serial value repeats for any remaining hosts.
func GroupHostBySerial(hosts []string, serial []any) ([][]string, error) {
	if len(serial) == 0 || len(hosts) == 0 {
		return [][]string{hosts}, nil
	}

	// Convert serial values to integer batch sizes.
	sis, count, err := groupHostBySerialSizes(hosts, serial)
	if err != nil {
		return nil, err
	}

	// Repeat the last serial value to cover remaining hosts.
	if len(hosts) > count {
		lastVal := sis[len(sis)-1]
		for i := 0.0; i < float64(len(hosts)-count)/float64(lastVal); i++ {
			sis = append(sis, lastVal)
		}
	}

	// Slice hosts into batches. Both bounds are clamped, not just end: a
	// serial entry larger than the hosts remaining pushes begin past the
	// slice on the NEXT iteration, and hosts[begin:end] with begin > end
	// panics -- e.g. 2 hosts with serial [3, 1]. Over-long entries simply
	// yield empty trailing batches, which the caller already tolerates.
	result := make([][]string, len(sis))
	var begin, end int
	for i, si := range sis {
		end = min(end+si, len(hosts))
		begin = min(begin, end)
		result[i] = hosts[begin:end]
		begin += si
	}

	return result, nil
}

// groupHostBySerialSizes converts serial values to integer batch sizes,
// returning the sizes slice and their running total. Serial items can be
// integers or percentage strings (e.g., "50%").
func groupHostBySerialSizes(hosts []string, serial []any) ([]int, int, error) {
	sis := make([]int, len(serial))
	var count int
	for i, a := range serial {
		size, err := groupHostBySerialSize(a, len(hosts))
		if err != nil {
			return nil, 0, err
		}
		sis[i] = size
		// Non-positive, not just zero: a negative serial produced a
		// negative slice index further down and panicked, taking the
		// server's playbook-planning goroutine with it.
		if sis[i] <= 0 {
			return nil, 0, fmt.Errorf("serial %v must be positive", a)
		}
		count += sis[i]
	}
	return sis, count, nil
}

// groupHostBySerialSize resolves a single serial value to an integer batch
// size. An int is used as-is; a percentage string (e.g., "50%") is ceil'd
// against hostCount; a plain numeric string is parsed as an int.
func groupHostBySerialSize(a any, hostCount int) (int, error) {
	switch val := a.(type) {
	case int:
		return val, nil
	case string:
		if strings.HasSuffix(val, "%") {
			b, err := strconv.ParseFloat(val[:len(val)-1], 64)
			if err != nil {
				return 0, fmt.Errorf("convert serial %q to float: %w", val, err)
			}
			return int(math.Ceil(float64(hostCount) * b / 100.0)), nil
		}
		b, err := strconv.Atoi(val)
		if err != nil {
			return 0, fmt.Errorf("convert serial %q to int: %w", val, err)
		}
		return b, nil
	default:
		return 0, fmt.Errorf("unknown serial type: only int or percentage string supported")
	}
}

// ConvertVarsNodes converts Vars yaml.Nodes to a merged map[string]any.
// Later nodes override earlier ones for duplicate keys.
func ConvertVarsNodes(nodes []yaml.Node) (map[string]any, error) {
	result := make(map[string]any)
	for _, node := range nodes {
		var m map[string]any
		if err := node.Decode(&m); err != nil {
			return nil, fmt.Errorf("decode vars: %w", err)
		}
		for k, v := range m {
			result[k] = v
		}
	}
	return result, nil
}
