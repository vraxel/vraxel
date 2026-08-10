package iam

import "strings"

// MatchPermission checks whether a wildcard pattern matches a permission code.
//
// Supported patterns:
//   - "*:*"              → matches everything
//   - "iam:*"            → prefix wildcard, matches any code starting with "iam:"
//   - "iam:namespaces:*" → prefix wildcard, matches "iam:namespaces:list", "iam:namespaces:users:create", etc.
//   - "*:list"           → suffix wildcard, matches any code ending with ":list"
//   - "iam:users:list"   → exact match
func MatchPermission(pattern, code string) bool {
	if pattern == "*:*" {
		return true
	}
	if strings.HasSuffix(pattern, ":*") {
		prefix := pattern[:len(pattern)-2]
		return strings.HasPrefix(code, prefix+":")
	}
	if strings.HasPrefix(pattern, "*:") {
		suffix := pattern[2:]
		return strings.HasSuffix(code, ":"+suffix)
	}
	return pattern == code
}

// ExpandPatterns expands a list of patterns against all known permission codes,
// returning the deduplicated set of matching codes.
func ExpandPatterns(patterns []string, allCodes []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, code := range allCodes {
		for _, pattern := range patterns {
			if MatchPermission(pattern, code) {
				if !seen[code] {
					seen[code] = true
					result = append(result, code)
				}
				break
			}
		}
	}
	return result
}
