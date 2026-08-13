package template

import (
	"fmt"
	"math"
	"net"
	"os"
	"strconv"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"gopkg.in/yaml.v3"
)

const recursionMaxNums = 1000

// buildFuncMap creates a template.FuncMap starting with Sprig's TxtFuncMap
// and adding custom functions on top.
func buildFuncMap(t *template.Template, includedNames map[string]int) template.FuncMap {
	fm := sprig.TxtFuncMap()
	// Remove potentially dangerous functions
	delete(fm, "env")
	delete(fm, "expandenv")

	// Add custom functions
	fm["toYaml"] = toYAML
	fm["fromYaml"] = fromYAML
	fm["ipInCIDR"] = ipInCIDR
	fm["ipFamily"] = ipFamily
	fm["pow"] = pow
	fm["subtractList"] = subtractList
	fm["fileExists"] = fileExists
	fm["unquote"] = unquote
	fm["include"] = includeFn(t, includedNames)
	fm["tpl"] = tplFn(t, includedNames, false)

	return fm
}

// includeFn returns a function that executes a named template with recursion protection.
func includeFn(t *template.Template, includedNames map[string]int) func(string, any) (string, error) {
	return func(name string, data any) (string, error) {
		var buf strings.Builder
		if v, ok := includedNames[name]; ok {
			if v > recursionMaxNums {
				return "", fmt.Errorf(
					"rendering template has a nested reference name: %s: unable to execute template",
					name)
			}
			includedNames[name]++
		} else {
			includedNames[name] = 1
		}
		err := t.ExecuteTemplate(&buf, name, data)
		includedNames[name]--
		return buf.String(), err
	}
}

// tplFn returns a function that evaluates a string as a template, supporting
// recursive template evaluation with depth limiting.
// When strict is true, missing keys produce an error; when false, they evaluate to zero values.
func tplFn(parent *template.Template, includedNames map[string]int, strict bool) func(string, any) (string, error) {
	return func(tpl string, vals any) (string, error) {
		t, err := parent.Clone()
		if err != nil {
			return "", fmt.Errorf("cannot clone template: %w", err)
		}

		// Re-inject the missingkey option, see text/template issue https://github.com/golang/go/issues/43022
		// We have to go by strict from our engine configuration, as the option fields are private in Template.
		if strict {
			t.Option("missingkey=error")
		} else {
			t.Option("missingkey=zero")
		}

		// Re-inject include and tpl so that define blocks inside tpl can be
		// included, and nested tpl calls work.
		t.Funcs(template.FuncMap{
			"include": includeFn(t, includedNames),
			"tpl":     tplFn(t, includedNames, strict),
		})

		t, err = t.New(parent.Name()).Parse(tpl)
		if err != nil {
			return "", fmt.Errorf("cannot parse template %q: %w", tpl, err)
		}

		var buf strings.Builder
		if err := t.Execute(&buf, vals); err != nil {
			return "", fmt.Errorf("error during tpl function execution for %q: %w", tpl, err)
		}

		return strings.ReplaceAll(buf.String(), "<no value>", ""), nil
	}
}

// toYAML marshals a value to a YAML string. Returns empty string on error.
func toYAML(v any) string {
	data, err := yaml.Marshal(v)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// fromYAML unmarshals a YAML string into an interface{}.
func fromYAML(v string) (any, error) {
	var output any
	err := yaml.Unmarshal([]byte(v), &output)
	return output, err
}

// ipInCIDR takes a comma-separated list of CIDR/IP/range strings and returns
// all individual IP addresses contained within them.
func ipInCIDR(cidr string) ([]string, error) {
	ips := make([]string, 0)
	for _, s := range strings.Split(cidr, ",") {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		ips = append(ips, parseIP(s)...)
	}
	return ips, nil
}

// parseIP parses a CIDR, IP range (e.g. "10.0.0.1-10.0.0.5"), or single IP
// into a slice of IP address strings.
func parseIP(ip string) []string {
	ip = strings.TrimRight(ip, "/")
	if strings.Contains(ip, "/") {
		return parseIPCIDR(ip)
	}
	if strings.Contains(ip, "-") {
		return parseIPRange(ip)
	}
	return []string{ip}
}

// parseIPCIDR enumerates a CIDR string into its contained IP address
// strings. Lifted verbatim from parseIP's CIDR branch.
func parseIPCIDR(ip string) []string {
	var result []string
	_, ipnet, err := net.ParseCIDR(ip)
	if err != nil || ipnet == nil {
		return result
	}
	// For /32 or /128 return just the IP
	ones, bits := ipnet.Mask.Size()
	if ones == bits {
		result = append(result, strings.Split(ip, "/")[0])
		return result
	}
	// Enumerate IPs in the CIDR
	for addr := ipnet.IP.Mask(ipnet.Mask); ipnet.Contains(addr); incrementIP(addr) {
		result = append(result, addr.String())
	}
	// Remove network and broadcast for IPv4 networks with more than 2 IPs
	if len(result) > 2 && result[0] != "" {
		netIP := net.ParseIP(result[0])
		if netIP != nil && netIP.To4() != nil {
			result = result[1 : len(result)-1]
		}
	}
	return result
}

// parseIPRange enumerates an inclusive IP range "start-end" into its
// contained IP address strings. Lifted verbatim from parseIP's range branch.
func parseIPRange(ip string) []string {
	var result []string
	parts := strings.SplitN(ip, "-", 2)
	if len(parts) == 2 {
		startIP := net.ParseIP(strings.TrimSpace(parts[0]))
		endIP := net.ParseIP(strings.TrimSpace(parts[1]))
		if startIP != nil && endIP != nil {
			for cur := cloneIP(startIP); !cur.Equal(endIP); incrementIP(cur) {
				result = append(result, cur.String())
			}
			result = append(result, endIP.String())
		}
	}
	return result
}

// cloneIP returns a copy of the given IP.
func cloneIP(ip net.IP) net.IP {
	dup := make(net.IP, len(ip))
	copy(dup, ip)
	return dup
}

// incrementIP increments an IP address in place.
func incrementIP(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}

// ipFamily returns "IPv4" or "IPv6" for the given IP address or CIDR string.
func ipFamily(addrOrCIDR string) (string, error) {
	ip := net.ParseIP(addrOrCIDR)
	if ip == nil {
		// Try parsing as CIDR
		ipFromCIDR, _, err := net.ParseCIDR(addrOrCIDR)
		if err != nil {
			return "Invalid", fmt.Errorf("%s is not ip or cidr", addrOrCIDR)
		}
		ip = ipFromCIDR
	}
	if ip.To4() != nil {
		return "IPv4", nil
	}
	return "IPv6", nil
}

// pow returns base raised to the power of exp.
func pow(base, exp float64) (float64, error) {
	return math.Pow(base, exp), nil
}

// subtractList returns elements from a that are not in b (set difference).
func subtractList(a, b []any) ([]any, error) {
	set := make(map[any]struct{}, len(b))
	for _, v := range b {
		set[v] = struct{}{}
	}
	result := make([]any, 0, len(a))
	for _, v := range a {
		if _, exists := set[v]; !exists {
			result = append(result, v)
		}
	}
	return result, nil
}

// fileExists returns true if the file at the given path exists.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// unquote removes surrounding quotes from a string value.
// Returns empty string for nil input or non-string input.
func unquote(input any) string {
	if input == nil {
		return ""
	}
	inputStr, ok := input.(string)
	if !ok {
		return ""
	}
	output, err := strconv.Unquote(inputStr)
	if err != nil {
		return inputStr
	}
	return output
}
