package variable

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"vraxel.io/vraxel/lib/ansible"
)

// CombineVariables deeply merges two maps. When both m1 and m2 contain the
// same key and both values are maps, the maps are recursively merged.
// Otherwise m2's value wins. Neither input map is mutated.
func CombineVariables(m1, m2 map[string]any) map[string]any {
	result := make(map[string]any, len(m1)+len(m2))

	for k, v := range m1 {
		result[k] = v
	}

	for k, v2 := range m2 {
		if v1, ok := result[k]; ok {
			result[k] = deepMerge(v1, v2)
		} else {
			result[k] = v2
		}
	}

	return result
}

// deepMerge recursively merges two values. If both are maps, keys are merged
// recursively. Otherwise val2 wins.
func deepMerge(val1, val2 any) any {
	if val1 == nil || val2 == nil {
		return val2
	}

	rv1 := reflect.ValueOf(val1)
	rv2 := reflect.ValueOf(val2)

	if rv1.Kind() == reflect.Map && rv2.Kind() == reflect.Map {
		merged := make(map[string]any)

		// Copy val1 entries.
		for _, k := range rv1.MapKeys() {
			merged[k.String()] = rv1.MapIndex(k).Interface()
		}

		// Merge val2 entries recursively.
		for _, k := range rv2.MapKeys() {
			key := k.String()
			if existing, ok := merged[key]; ok {
				merged[key] = deepMerge(existing, rv2.MapIndex(k).Interface())
			} else {
				merged[key] = rv2.MapIndex(k).Interface()
			}
		}

		return merged
	}

	// Non-map values: val2 overrides val1 (slices are replaced, not appended).
	return val2
}

// ConvertGroup builds a group-name to host-list mapping from an Inventory.
// It always includes:
//   - "all": every host from Inventory.Hosts
//   - "ungrouped": hosts not in any explicit group
//   - Each named group with its hosts resolved recursively (with cycle detection)
func ConvertGroup(inv ansible.Inventory) map[string][]string {
	groups := make(map[string][]string)

	// Build the "all" group from all declared hosts.
	all := make([]string, 0, len(inv.Hosts))
	for hostname := range inv.Hosts {
		all = append(all, hostname)
	}
	groups[keyGroupsAll] = all

	// Track which hosts are in at least one group.
	grouped := make(map[string]struct{})

	// Resolve each named group with its own visited set for cycle detection.
	for groupName := range inv.Groups {
		visited := make(map[string]struct{})
		hosts := resolveGroupHosts(inv, groupName, visited)
		groups[groupName] = hosts
		for _, h := range hosts {
			grouped[h] = struct{}{}
		}
	}

	// Build "ungrouped": hosts not in any named group.
	ungrouped := make([]string, 0)
	for _, hostname := range all {
		if _, ok := grouped[hostname]; !ok {
			ungrouped = append(ungrouped, hostname)
		}
	}
	groups[keyUngrouped] = ungrouped

	return groups
}

// resolveGroupHosts recursively resolves all hosts for a group, including
// hosts from nested sub-groups. The visited set prevents infinite loops.
func resolveGroupHosts(inv ansible.Inventory, groupName string, visited map[string]struct{}) []string {
	if _, cycle := visited[groupName]; cycle {
		return nil
	}
	visited[groupName] = struct{}{}

	group, ok := inv.Groups[groupName]
	if !ok {
		return nil
	}

	var hosts []string

	// Resolve sub-groups first.
	for _, subGroup := range group.Groups {
		subHosts := resolveGroupHosts(inv, subGroup, visited)
		hosts = combineSlice(hosts, subHosts)
	}

	// Add direct hosts.
	hosts = combineSlice(hosts, group.Hosts)

	return hosts
}

// ========================================================================
// Type extraction helpers
// ========================================================================

// StringVar retrieves a string value from a nested map using the given keys.
// Keys are traversed in order to reach nested maps.
// Returns "" if the key path is not found or the value is not a string.
func StringVar(vars map[string]any, keys ...string) string {
	val := nestedLookup(vars, keys...)
	if val == nil {
		return ""
	}
	s, ok := val.(string)
	if !ok {
		return fmt.Sprintf("%v", val)
	}
	return s
}

// IntVar retrieves an integer value from a nested map using the given keys.
// It handles int, int64, float64 (JSON numbers), and string representations.
// Returns 0 if the key path is not found or conversion fails.
func IntVar(vars map[string]any, keys ...string) int {
	val := nestedLookup(vars, keys...)
	if val == nil {
		return 0
	}

	rv := reflect.ValueOf(val)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return int(rv.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return int(rv.Uint())
	case reflect.Float32, reflect.Float64:
		return int(rv.Float())
	case reflect.String:
		n, err := strconv.Atoi(rv.String())
		if err != nil {
			return 0
		}
		return n
	default:
		return 0
	}
}

// BoolVar retrieves a boolean value from a nested map using the given keys.
// It handles bool and string ("true"/"false") representations.
// Returns false if the key path is not found or conversion fails.
func BoolVar(vars map[string]any, keys ...string) bool {
	val := nestedLookup(vars, keys...)
	if val == nil {
		return false
	}

	switch v := val.(type) {
	case bool:
		return v
	case string:
		b, err := strconv.ParseBool(v)
		if err != nil {
			return false
		}
		return b
	default:
		return false
	}
}

// SliceVar retrieves a slice value from a nested map using the given keys.
// Returns nil if the key path is not found or the value is not a slice.
func SliceVar(vars map[string]any, keys ...string) []any {
	val := nestedLookup(vars, keys...)
	if val == nil {
		return nil
	}

	s, ok := val.([]any)
	if !ok {
		return nil
	}
	return s
}

// PrintVar retrieves a value from a nested map using a dot-separated key path.
// For example, PrintVar(vars, "os.hostname") traverses vars["os"]["hostname"].
// Returns nil if any key in the path is not found.
func PrintVar(vars map[string]any, key string) any {
	keys := strings.Split(key, ".")
	return nestedLookup(vars, keys...)
}

// nestedLookup walks a chain of keys through nested maps, returning the final value.
// Returns nil if any key is missing or the intermediate value is not a map.
func nestedLookup(vars map[string]any, keys ...string) any {
	if len(keys) == 0 {
		return nil
	}

	current := any(vars)
	for _, key := range keys {
		m, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		val, exists := m[key]
		if !exists {
			return nil
		}
		current = val
	}
	return current
}
