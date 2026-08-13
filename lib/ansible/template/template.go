package template

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
)

// Parse renders a template string with the given variables.
// If the input does not contain {{ }} template syntax, it is returned unchanged.
func Parse(vars map[string]any, input string) ([]byte, error) {
	if !IsTmplSyntax(input) {
		return []byte(input), nil
	}

	includedNames := make(map[string]int)
	tl := template.New("gotpl")
	funcMap := buildFuncMap(tl, includedNames)

	_, err := tl.Funcs(funcMap).Parse(input)
	if err != nil {
		return nil, fmt.Errorf("failed to parse template %q: %w", input, err)
	}

	var buf bytes.Buffer
	if err := tl.Execute(&buf, vars); err != nil {
		return nil, fmt.Errorf("failed to execute template %q: %w", input, err)
	}

	return buf.Bytes(), nil
}

// ParseString is a convenience wrapper around Parse that returns a string.
func ParseString(vars map[string]any, input string) (string, error) {
	result, err := Parse(vars, input)
	if err != nil {
		return "", err
	}
	return string(result), nil
}

// ParseBool evaluates one or more template conditions against the given variables.
// ALL conditions must evaluate to "true" (case-insensitive) for the result to be true.
// An empty conditions list returns true.
func ParseBool(vars map[string]any, conditions ...string) (bool, error) {
	for _, cond := range conditions {
		result, err := Parse(vars, cond)
		if err != nil {
			return false, err
		}
		if !bytes.EqualFold(result, []byte("true")) {
			return false, nil
		}
	}
	return true, nil
}

// IsTmplSyntax checks if the string contains {{ }} template syntax.
func IsTmplSyntax(s string) bool {
	return strings.Contains(s, "{{") && strings.Contains(s, "}}")
}

// TrimTmplSyntax removes {{ and }} delimiters from a string and trims whitespace.
func TrimTmplSyntax(s string) string {
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(s, "{{"), "}}"))
}
