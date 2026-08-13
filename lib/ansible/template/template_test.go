package template

import (
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// Parse
// ---------------------------------------------------------------------------

func TestParse_NoTemplate(t *testing.T) {
	input := "hello world"
	result, err := Parse(nil, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(result) != input {
		t.Fatalf("expected %q, got %q", input, string(result))
	}
}

func TestParse_SimpleVariable(t *testing.T) {
	vars := map[string]any{"name": "world"}
	result, err := Parse(vars, "hello {{ .name }}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "hello world"
	if string(result) != expected {
		t.Fatalf("expected %q, got %q", expected, string(result))
	}
}

func TestParse_NestedVariable(t *testing.T) {
	vars := map[string]any{
		"server": map[string]any{
			"host": "10.0.0.1",
		},
	}
	result, err := Parse(vars, "{{ .server.host }}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "10.0.0.1"
	if string(result) != expected {
		t.Fatalf("expected %q, got %q", expected, string(result))
	}
}

func TestParse_MultipleVariables(t *testing.T) {
	vars := map[string]any{
		"host": "10.0.0.1",
		"port": 8080,
	}
	result, err := Parse(vars, "{{ .host }}:{{ .port }}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "10.0.0.1:8080"
	if string(result) != expected {
		t.Fatalf("expected %q, got %q", expected, string(result))
	}
}

func TestParse_InvalidTemplate(t *testing.T) {
	// Use a template that contains {{ }} so IsTmplSyntax returns true,
	// but has invalid Go template syntax inside.
	_, err := Parse(nil, "{{ .foo | invalidFunc }}")
	if err == nil {
		t.Fatal("expected error for invalid template syntax")
	}
}

// ---------------------------------------------------------------------------
// ParseString
// ---------------------------------------------------------------------------

func TestParseString(t *testing.T) {
	vars := map[string]any{"greeting": "hi"}
	result, err := ParseString(vars, "{{ .greeting }} there")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "hi there"
	if result != expected {
		t.Fatalf("expected %q, got %q", expected, result)
	}
}

func TestParseString_NoTemplate(t *testing.T) {
	result, err := ParseString(nil, "plain text")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "plain text" {
		t.Fatalf("expected %q, got %q", "plain text", result)
	}
}

// ---------------------------------------------------------------------------
// ParseBool
// ---------------------------------------------------------------------------

func TestParseBool_True(t *testing.T) {
	vars := map[string]any{"enabled": "true"}
	ok, err := ParseBool(vars, `{{ eq .enabled "true" }}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected true, got false")
	}
}

func TestParseBool_False(t *testing.T) {
	vars := map[string]any{"enabled": "false"}
	ok, err := ParseBool(vars, `{{ eq .enabled "true" }}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected false, got true")
	}
}

func TestParseBool_MultipleConds_AllTrue(t *testing.T) {
	vars := map[string]any{
		"a": "yes",
		"b": "yes",
	}
	ok, err := ParseBool(vars,
		`{{ eq .a "yes" }}`,
		`{{ eq .b "yes" }}`,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected true, got false")
	}
}

func TestParseBool_MultipleConds_OneFalse(t *testing.T) {
	vars := map[string]any{
		"a": "yes",
		"b": "no",
	}
	ok, err := ParseBool(vars,
		`{{ eq .a "yes" }}`,
		`{{ eq .b "yes" }}`,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected false, got true")
	}
}

func TestParseBool_Empty(t *testing.T) {
	ok, err := ParseBool(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected true for empty conditions, got false")
	}
}

func TestParseBool_PlainTrue(t *testing.T) {
	// Non-template string "true" should pass through unchanged and match.
	ok, err := ParseBool(nil, "true")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected true, got false")
	}
}

// ---------------------------------------------------------------------------
// IsTmplSyntax
// ---------------------------------------------------------------------------

func TestIsTmplSyntax(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"{{ .foo }}", true},
		{"hello {{ world }}", true},
		{"no template here", false},
		{"only {{ opening", false},
		{"only }} closing", false},
		{"", false},
		{"{{.x}}", true},
	}
	for _, tc := range tests {
		if got := IsTmplSyntax(tc.input); got != tc.expected {
			t.Errorf("IsTmplSyntax(%q) = %v, want %v", tc.input, got, tc.expected)
		}
	}
}

// ---------------------------------------------------------------------------
// TrimTmplSyntax
// ---------------------------------------------------------------------------

func TestTrimTmplSyntax(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"{{ .foo }}", ".foo"},
		{"{{bar}}", "bar"},
		{"{{ spaced }}", "spaced"},
	}
	for _, tc := range tests {
		if got := TrimTmplSyntax(tc.input); got != tc.expected {
			t.Errorf("TrimTmplSyntax(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

// ---------------------------------------------------------------------------
// Custom functions
// ---------------------------------------------------------------------------

func TestToYaml(t *testing.T) {
	vars := map[string]any{
		"config": map[string]any{
			"key1": "value1",
			"key2": "value2",
		},
	}
	result, err := ParseString(vars, `{{ toYaml .config }}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == "" {
		t.Fatal("expected non-empty YAML output")
	}
	// Should contain both keys
	if !containsAll(result, "key1: value1", "key2: value2") {
		t.Fatalf("unexpected YAML output: %s", result)
	}
}

func TestToYaml_Nil(t *testing.T) {
	result := toYAML(nil)
	if result != "null" {
		t.Fatalf("expected %q for nil, got %q", "null", result)
	}
}

func TestFromYaml(t *testing.T) {
	input := "key: value"
	result, err := fromYAML(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", result)
	}
	if m["key"] != "value" {
		t.Fatalf("expected key=value, got key=%v", m["key"])
	}
}

func TestFromYaml_Invalid(t *testing.T) {
	_, err := fromYAML(":\n  :\n    - ][")
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestSubtractList(t *testing.T) {
	a := []any{1, 2, 3, 4, 5}
	b := []any{2, 4}
	result, err := subtractList(a, b)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := []any{1, 3, 5}
	if len(result) != len(expected) {
		t.Fatalf("expected %v, got %v", expected, result)
	}
	for i, v := range expected {
		if result[i] != v {
			t.Fatalf("expected result[%d]=%v, got %v", i, v, result[i])
		}
	}
}

func TestSubtractList_Empty(t *testing.T) {
	result, err := subtractList([]any{1, 2}, []any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(result))
	}
}

func TestSubtractList_AllRemoved(t *testing.T) {
	result, err := subtractList([]any{1, 2}, []any{1, 2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("expected 0 elements, got %d", len(result))
	}
}

func TestIpFamily_IPv4(t *testing.T) {
	family, err := ipFamily("192.168.1.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if family != "IPv4" {
		t.Fatalf("expected IPv4, got %s", family)
	}
}

func TestIpFamily_IPv6(t *testing.T) {
	family, err := ipFamily("::1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if family != "IPv6" {
		t.Fatalf("expected IPv6, got %s", family)
	}
}

func TestIpFamily_CIDR(t *testing.T) {
	family, err := ipFamily("10.0.0.0/24")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if family != "IPv4" {
		t.Fatalf("expected IPv4, got %s", family)
	}
}

func TestIpFamily_Invalid(t *testing.T) {
	_, err := ipFamily("not-an-ip")
	if err == nil {
		t.Fatal("expected error for invalid input")
	}
}

func TestPow(t *testing.T) {
	result, err := pow(2, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != 1024 {
		t.Fatalf("expected 1024, got %f", result)
	}
}

func TestFileExists(t *testing.T) {
	// Current test file should exist
	if !fileExists("template_test.go") {
		t.Fatal("expected template_test.go to exist")
	}
	if fileExists("nonexistent_file_xyz.go") {
		t.Fatal("expected nonexistent file to not exist")
	}
}

func TestFileExists_InTemplate(t *testing.T) {
	// Create a temp file to test with
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "testfile.txt")
	if err := os.WriteFile(tmpFile, []byte("hello"), 0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	vars := map[string]any{"path": tmpFile}
	result, err := ParseString(vars, `{{ if fileExists .path }}yes{{ else }}no{{ end }}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "yes" {
		t.Fatalf("expected yes, got %q", result)
	}
}

func TestUnquote(t *testing.T) {
	tests := []struct {
		input    any
		expected string
	}{
		{`"hello"`, "hello"},
		{"noquotes", "noquotes"},
		{nil, ""},
		{123, ""},
		{`"escaped \"quote\""`, `escaped "quote"`},
	}
	for _, tc := range tests {
		got := unquote(tc.input)
		if got != tc.expected {
			t.Errorf("unquote(%v) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestIpInCIDR_SingleIP(t *testing.T) {
	ips, err := ipInCIDR("10.0.0.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ips) != 1 || ips[0] != "10.0.0.1" {
		t.Fatalf("expected [10.0.0.1], got %v", ips)
	}
}

func TestIpInCIDR_Multiple(t *testing.T) {
	ips, err := ipInCIDR("10.0.0.1, 10.0.0.2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ips) != 2 {
		t.Fatalf("expected 2 IPs, got %d: %v", len(ips), ips)
	}
}

// ---------------------------------------------------------------------------
// Sprig functions
// ---------------------------------------------------------------------------

func TestSprigFunctions_Upper(t *testing.T) {
	vars := map[string]any{"name": "hello"}
	result, err := ParseString(vars, "{{ upper .name }}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "HELLO" {
		t.Fatalf("expected HELLO, got %q", result)
	}
}

func TestSprigFunctions_Default(t *testing.T) {
	vars := map[string]any{}
	result, err := ParseString(vars, `{{ default "fallback" .missing }}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "fallback" {
		t.Fatalf("expected fallback, got %q", result)
	}
}

func TestSprigFunctions_Join(t *testing.T) {
	vars := map[string]any{"items": []string{"a", "b", "c"}}
	result, err := ParseString(vars, `{{ join "," .items }}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "a,b,c" {
		t.Fatalf("expected a,b,c, got %q", result)
	}
}

func TestSprigFunctions_Ternary(t *testing.T) {
	vars := map[string]any{"flag": true}
	result, err := ParseString(vars, `{{ ternary "yes" "no" .flag }}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "yes" {
		t.Fatalf("expected yes, got %q", result)
	}
}

// ---------------------------------------------------------------------------
// tpl function (recursive template evaluation)
// ---------------------------------------------------------------------------

func TestTplFunction(t *testing.T) {
	vars := map[string]any{
		"greeting": "hello",
		"tmpl":     "{{ .greeting }} world",
	}
	result, err := ParseString(vars, `{{ tpl .tmpl . }}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "hello world"
	if result != expected {
		t.Fatalf("expected %q, got %q", expected, result)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func containsAll(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if !stringContains(s, sub) {
			return false
		}
	}
	return true
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
