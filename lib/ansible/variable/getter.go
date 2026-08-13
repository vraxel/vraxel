package variable

import (
	"fmt"
	"net"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Well-known variable keys.
const (
	keyIPv4          = "ipv4"
	keyIPv6          = "ipv6"
	keyHostname      = "hostname"
	keyInventoryName = "inventory_name"
	keyOS            = "os"
	keyOSHostname    = "hostname"
	keyLocalhost     = "localhost"
	keyGroupsAll     = "all"
	keyUngrouped     = "ungrouped"
	keyGlobalHosts   = "global_hosts"
	keyGroups        = "groups"
)

// ========================================================================
// GetFunc factories
// ========================================================================

// GetAllVariable returns a GetFunc that computes the merged variable map for
// a specific host. Variables are merged in priority order (lowest to highest):
//  1. Role defaults (DefaultVars)
//  2. Inventory.Vars
//  3. Group vars (for each group the host belongs to)
//  4. Host-specific vars from Inventory.Hosts[hostname]
//  5. Remote vars (gather_facts)
//  6. Runtime vars (set_fact, include_vars, block vars, role vars)
//
// The result also includes "global_hosts" and "groups" meta-variables.
func GetAllVariable(hostname string) GetFunc {
	return func(v *Value) any {
		globalHosts := buildGlobalHosts(v)

		hostVars, ok := globalHosts[hostname].(map[string]any)
		if !ok {
			return make(map[string]any)
		}

		// Add global hosts and groups meta-information.
		hostVars[keyGlobalHosts] = globalHosts
		hostVars[keyGroups] = ConvertGroup(v.Inventory)

		return hostVars
	}
}

// buildGlobalHosts builds the merged variable map for every host in the Value.
func buildGlobalHosts(v *Value) map[string]any {
	globalHosts := make(map[string]any, len(v.Hosts))
	for hostname, hv := range v.Hosts {
		merged := make(map[string]any)

		// 1. Role defaults (lowest priority — can be overridden by anything).
		merged = CombineVariables(merged, hv.DefaultVars)

		// 2. Inventory-level vars.
		merged = CombineVariables(merged, v.Inventory.Vars)

		// 3. Group vars: apply vars from every group that contains this host.
		for _, gv := range v.Inventory.Groups {
			if slices.Contains(gv.Hosts, hostname) {
				merged = CombineVariables(merged, gv.Vars)
			}
		}

		// 4. Host-specific vars from inventory.
		if hostSpecific, ok := v.Inventory.Hosts[hostname]; ok {
			merged = CombineVariables(merged, hostSpecific)
		}

		// 5. Remote vars (gather_facts).
		merged = CombineVariables(merged, hv.RemoteVars)

		// 6. Runtime vars (set_fact, include_vars, block vars, role vars) — highest priority.
		merged = CombineVariables(merged, hv.RuntimeVars)

		// Set default variables.
		setDefaultHostVars(hostname, merged)

		globalHosts[hostname] = merged
	}
	return globalHosts
}

// setDefaultHostVars sets default variables for a host:
// - For "localhost": auto-detect ipv4/ipv6.
// - For all hosts: set hostname and inventory_name if not present.
func setDefaultHostVars(hostname string, vars map[string]any) {
	if hostname == keyLocalhost {
		if _, ok := vars[keyIPv4]; !ok {
			vars[keyIPv4] = getLocalIP(keyIPv4)
		}
		if _, ok := vars[keyIPv6]; !ok {
			vars[keyIPv6] = getLocalIP(keyIPv6)
		}
	}

	// Try to derive hostname from OS info.
	if osInfo, ok := vars[keyOS]; ok {
		if osMap, ok := osInfo.(map[string]any); ok {
			if osHostname, ok := osMap[keyOSHostname]; ok {
				vars[keyHostname] = osHostname
			}
		}
	}

	if _, ok := vars[keyInventoryName]; !ok {
		vars[keyInventoryName] = hostname
	}
	if _, ok := vars[keyHostname]; !ok {
		vars[keyHostname] = hostname
	}
}

// getLocalIP returns the first non-loopback IP address of the given type ("ipv4" or "ipv6").
func getLocalIP(ipType string) string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
			if ipType == keyIPv4 && ipNet.IP.To4() != nil && ipNet.IP.IsGlobalUnicast() {
				return ipNet.IP.String()
			}
			if ipType == keyIPv6 && ipNet.IP.To4() == nil && ipNet.IP.IsGlobalUnicast() {
				return ipNet.IP.String()
			}
		}
	}
	return ""
}

// GetHostnames returns a GetFunc that resolves a list of names to actual
// hostnames. Each name can be:
//   - A direct hostname (must exist in Value.Hosts)
//   - A group name (resolves to all hosts in that group)
//   - An indexed group access like "group[0]"
func GetHostnames(names []string) GetFunc {
	if len(names) == 0 {
		return func(v *Value) any {
			return []string{}
		}
	}

	return func(v *Value) any {
		groups := ConvertGroup(v.Inventory)
		var result []string

		for _, name := range names {
			name = strings.TrimSpace(name)
			result = resolveHostname(v, groups, name, result)
		}

		return result
	}
}

// resolveHostname folds a single name into result by trying, in order:
// direct hostname, group name, then indexed group access ("group[0]").
// Extracted from GetHostnames' closure so its loop body stays flat.
func resolveHostname(v *Value, groups map[string][]string, name string, result []string) []string {
	// Direct hostname match.
	if _, exists := v.Hosts[name]; exists {
		result = combineSlice(result, []string{name})
	}

	// Group name match.
	if groupHosts, ok := groups[name]; ok {
		result = combineSlice(result, groupHosts)
	}

	// Indexed group access: "group[0]".
	result = resolveHostnameIndexed(groups, name, result)
	return result
}

// resolveHostnameIndexed handles the "group[0]" indexed-access form,
// folding the addressed host into result when the index is in range.
// Extracted from resolveHostname to keep its nesting shallow.
func resolveHostnameIndexed(groups map[string][]string, name string, result []string) []string {
	regexForIndex := regexp.MustCompile(`^(.*?)\[(\d+)]$`)
	match := regexForIndex.FindStringSubmatch(name)
	if match == nil {
		return result
	}
	index, err := strconv.Atoi(match[2])
	if err != nil {
		return result
	}
	if groupHosts, ok := groups[match[1]]; ok && index < len(groupHosts) {
		result = combineSlice(result, []string{groupHosts[index]})
	}
	return result
}

// GetResultVariable returns a GetFunc that retrieves the global Result map.
func GetResultVariable() GetFunc {
	return func(v *Value) any {
		return v.Result
	}
}

// GetHostMaxLength returns a GetFunc that computes the maximum hostname length.
func GetHostMaxLength() GetFunc {
	return func(v *Value) any {
		maxLen := 0
		for hostname := range v.Hosts {
			if len(hostname) > maxLen {
				maxLen = len(hostname)
			}
		}
		return maxLen
	}
}

// ========================================================================
// MergeFunc factories
// ========================================================================

// MergeRuntimeVariable returns a MergeFunc that parses YAML nodes into maps
// and merges them into the RuntimeVars of the specified hosts.
func MergeRuntimeVariable(nodes []yaml.Node, hosts ...string) MergeFunc {
	if len(nodes) == 0 {
		return func(v *Value) {}
	}

	return func(v *Value) {
		for _, hostname := range hosts {
			hv, ok := v.Hosts[hostname]
			if !ok {
				continue
			}
			mergeRuntimeVariableNodes(hv, nodes)
		}
	}
}

// mergeRuntimeVariableNodes parses each non-zero YAML node and folds it
// into hv.RuntimeVars. Extracted from MergeRuntimeVariable's closure so
// the per-host loop body stays flat.
func mergeRuntimeVariableNodes(hv *HostVars, nodes []yaml.Node) {
	for _, node := range nodes {
		if node.IsZero() {
			continue
		}
		data, err := parseYAMLNode(node)
		if err != nil {
			continue
		}
		hv.RuntimeVars = CombineVariables(hv.RuntimeVars, data)
	}
}

// MergeRemoteVariable returns a MergeFunc that sets the RemoteVars for a host.
func MergeRemoteVariable(host string, vars map[string]any) MergeFunc {
	return func(v *Value) {
		hv, ok := v.Hosts[host]
		if !ok {
			return
		}
		hv.RemoteVars = vars
	}
}

// MergeHostDefaultVars returns a MergeFunc that merges a plain map into the
// DefaultVars of the specified host. DefaultVars have the lowest priority
// and are used for role defaults/main.yml.
func MergeHostDefaultVars(host string, vars map[string]any) MergeFunc {
	if len(vars) == 0 {
		return func(v *Value) {}
	}

	return func(v *Value) {
		hv, ok := v.Hosts[host]
		if !ok {
			return
		}
		hv.DefaultVars = CombineVariables(hv.DefaultVars, vars)
	}
}

// MergeHostRuntimeVars returns a MergeFunc that merges a plain map into the
// RuntimeVars of the specified host. Unlike MergeRuntimeVariable which parses
// YAML nodes, this accepts a ready-to-use map[string]any directly.
func MergeHostRuntimeVars(host string, vars map[string]any) MergeFunc {
	if len(vars) == 0 {
		return func(v *Value) {}
	}

	return func(v *Value) {
		hv, ok := v.Hosts[host]
		if !ok {
			return
		}
		hv.RuntimeVars = CombineVariables(hv.RuntimeVars, vars)
	}
}

// MergeResultVariable returns a MergeFunc that merges data into the global Result map.
func MergeResultVariable(result map[string]any) MergeFunc {
	return func(v *Value) {
		v.Result = CombineVariables(v.Result, result)
	}
}

// ========================================================================
// YAML node parsing (simplified, no template engine)
// ========================================================================

// parseYAMLNode decodes a yaml.Node into a map[string]any.
// It handles document nodes by unwrapping them first.
func parseYAMLNode(node yaml.Node) (map[string]any, error) {
	// If it's a document node, decode from the first content node.
	target := &node
	if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		target = node.Content[0]
	}

	var result map[string]any
	if err := target.Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode YAML node: %w", err)
	}
	return result, nil
}

// combineSlice merges two string slices, skipping duplicates from s2.
func combineSlice(s1, s2 []string) []string {
	seen := make(map[string]struct{}, len(s1))
	for _, v := range s1 {
		seen[v] = struct{}{}
	}

	result := make([]string, len(s1))
	copy(result, s1)

	for _, v := range s2 {
		if _, exists := seen[v]; !exists {
			seen[v] = struct{}{}
			result = append(result, v)
		}
	}
	return result
}
