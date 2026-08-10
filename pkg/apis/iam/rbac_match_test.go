package iam

import (
	"sort"
	"testing"
)

func TestMatchPermission(t *testing.T) {
	tests := []struct {
		pattern string
		code    string
		want    bool
	}{
		// *:* matches everything
		{"*:*", "iam:users:list", true},
		{"*:*", "infra:hosts:create", true},
		{"*:*", "app:services:delete", true},

		// Prefix wildcard
		{"iam:*", "iam:users:list", true},
		{"iam:*", "iam:namespaces:users:create", true},
		{"iam:*", "app:services:list", false},
		{"iam:namespaces:*", "iam:namespaces:list", true},
		{"iam:namespaces:*", "iam:namespaces:users:list", true},
		{"iam:namespaces:*", "iam:users:list", false},

		// Suffix wildcard
		{"*:list", "iam:users:list", true},
		{"*:list", "app:services:list", true},
		{"*:list", "iam:users:get", false},
		{"*:list", "iam:namespaces:users:list", true},
		{"*:get", "iam:users:get", true},
		{"*:get", "iam:users:list", false},

		// Exact match
		{"iam:users:list", "iam:users:list", true},
		{"iam:users:list", "iam:users:get", false},
		{"iam:users:list", "infra:users:list", false},

		// Edge cases
		{"*:*", "*:*", true},
	}

	for _, tt := range tests {
		got := MatchPermission(tt.pattern, tt.code)
		if got != tt.want {
			t.Errorf("MatchPermission(%q, %q) = %v, want %v", tt.pattern, tt.code, got, tt.want)
		}
	}
}

func TestExpandPatterns(t *testing.T) {
	allCodes := []string{
		"iam:users:list", "iam:users:get", "iam:users:create",
		"iam:namespaces:list", "iam:namespaces:get", "iam:namespaces:update",
		"iam:namespaces:users:list",
		"app:services:list", "app:services:get", "app:services:create",
		"infra:hosts:list", "infra:hosts:get",
	}

	tests := []struct {
		name     string
		patterns []string
		want     []string
	}{
		{
			name:     "wildcard all",
			patterns: []string{"*:*"},
			want:     allCodes,
		},
		{
			name:     "module prefix",
			patterns: []string{"iam:*"},
			want: []string{
				"iam:users:list", "iam:users:get", "iam:users:create",
				"iam:namespaces:list", "iam:namespaces:get", "iam:namespaces:update",
				"iam:namespaces:users:list",
			},
		},
		{
			name:     "suffix wildcard list+get",
			patterns: []string{"*:list", "*:get"},
			want: []string{
				"iam:users:list", "iam:users:get",
				"iam:namespaces:list", "iam:namespaces:get",
				"iam:namespaces:users:list",
				"app:services:list", "app:services:get",
				"infra:hosts:list", "infra:hosts:get",
			},
		},
		{
			name:     "exact match",
			patterns: []string{"iam:users:list"},
			want:     []string{"iam:users:list"},
		},
		{
			name:     "sub-resource prefix",
			patterns: []string{"iam:namespaces:*"},
			want: []string{
				"iam:namespaces:list", "iam:namespaces:get", "iam:namespaces:update",
				"iam:namespaces:users:list",
			},
		},
		{
			name:     "no match",
			patterns: []string{"unknown:*"},
			want:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExpandPatterns(tt.patterns, allCodes)
			sort.Strings(got)
			sort.Strings(tt.want)
			if len(got) != len(tt.want) {
				t.Fatalf("ExpandPatterns(%v) returned %d items, want %d\ngot:  %v\nwant: %v", tt.patterns, len(got), len(tt.want), got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("ExpandPatterns(%v)[%d] = %q, want %q", tt.patterns, i, got[i], tt.want[i])
				}
			}
		})
	}
}
