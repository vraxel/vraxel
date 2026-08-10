package list

import (
	"testing"
	"time"
)

func strPtr(s string) *string { return &s }
func boolPtr(b bool) *bool    { return &b }
func i32Ptr(n int32) *int32   { return &n }
func i64Ptr(n int64) *int64   { return &n }

func TestFilterStr(t *testing.T) {
	tests := []struct {
		name     string
		filters  map[string]any
		key      string
		expected *string
	}{
		{"string value", map[string]any{"certType": "ca"}, "certType", strPtr("ca")},
		{"missing key", map[string]any{"certType": "ca"}, "missing", nil},
		{"non-string value", map[string]any{"count": 42}, "count", nil},
		{"empty map", map[string]any{}, "certType", nil},
		{"empty string skipped", map[string]any{"certType": ""}, "certType", nil},
		{"search escapes LIKE", map[string]any{"search": "50%_off"}, "search", strPtr(`50\%\_off`)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FilterStr(tt.filters, tt.key)
			if tt.expected == nil {
				if got != nil {
					t.Errorf("expected nil, got %q", *got)
				}
				return
			}
			if got == nil {
				t.Errorf("expected %q, got nil", *tt.expected)
			} else if *got != *tt.expected {
				t.Errorf("expected %q, got %q", *tt.expected, *got)
			}
		})
	}
}

func TestFilterInt32(t *testing.T) {
	tests := []struct {
		name     string
		filters  map[string]any
		expected *int32
	}{
		{"int32 direct", map[string]any{"n": int32(42)}, i32Ptr(42)},
		{"int64 narrowed", map[string]any{"n": int64(42)}, i32Ptr(42)},
		{"string parsed", map[string]any{"n": "42"}, i32Ptr(42)},
		{"string unparseable", map[string]any{"n": "abc"}, nil},
		{"missing", map[string]any{}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FilterInt32(tt.filters, "n")
			if tt.expected == nil && got != nil {
				t.Errorf("expected nil, got %d", *got)
			}
			if tt.expected != nil && (got == nil || *got != *tt.expected) {
				t.Errorf("expected %d, got %v", *tt.expected, got)
			}
		})
	}
}

func TestFilterInt64(t *testing.T) {
	tests := []struct {
		name     string
		filters  map[string]any
		expected *int64
	}{
		{"int64 direct", map[string]any{"n": int64(42)}, i64Ptr(42)},
		{"string parsed", map[string]any{"n": "42"}, i64Ptr(42)},
		{"string unparseable", map[string]any{"n": "abc"}, nil},
		{"int (not supported)", map[string]any{"n": 42}, nil},
		{"missing", map[string]any{}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FilterInt64(tt.filters, "n")
			if tt.expected == nil && got != nil {
				t.Errorf("expected nil, got %d", *got)
			}
			if tt.expected != nil && (got == nil || *got != *tt.expected) {
				t.Errorf("expected %d, got %v", *tt.expected, got)
			}
		})
	}
}

func TestFilterInt64Slice(t *testing.T) {
	if got := FilterInt64Slice(map[string]any{}, "ids"); got != nil {
		t.Errorf("missing key: expected nil, got %v", got)
	}
	if got := FilterInt64Slice(map[string]any{"ids": []int64{1, 2, 3}}, "ids"); len(got) != 3 {
		t.Errorf("expected len 3, got %v", got)
	}
	if got := FilterInt64Slice(map[string]any{"ids": "abc"}, "ids"); got != nil {
		t.Errorf("wrong type: expected nil, got %v", got)
	}
}

func TestFilterBool(t *testing.T) {
	tests := []struct {
		name     string
		filters  map[string]any
		expected *bool
	}{
		{"bool true", map[string]any{"x": true}, boolPtr(true)},
		{"bool false", map[string]any{"x": false}, boolPtr(false)},
		{"string true", map[string]any{"x": "true"}, boolPtr(true)},
		{"string false", map[string]any{"x": "false"}, boolPtr(false)},
		{"string 1", map[string]any{"x": "1"}, boolPtr(true)},
		{"string 0", map[string]any{"x": "0"}, boolPtr(false)},
		{"string T", map[string]any{"x": "T"}, boolPtr(true)},
		{"string invalid", map[string]any{"x": "yes"}, nil},
		{"missing", map[string]any{}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FilterBool(tt.filters, "x")
			if tt.expected == nil && got != nil {
				t.Errorf("expected nil, got %v", *got)
			}
			if tt.expected != nil && (got == nil || *got != *tt.expected) {
				t.Errorf("expected %v, got %v", *tt.expected, got)
			}
		})
	}
}

func TestFilterTime(t *testing.T) {
	if got := FilterTime(map[string]any{}, "t"); got != nil {
		t.Errorf("missing: expected nil, got %v", got)
	}
	ts := "2025-01-02T15:04:05Z"
	got := FilterTime(map[string]any{"t": ts}, "t")
	if got == nil {
		t.Fatalf("expected parsed time, got nil")
	}
	want, _ := time.Parse(time.RFC3339, ts)
	if !got.Equal(want) {
		t.Errorf("expected %v, got %v", want, *got)
	}
	if got := FilterTime(map[string]any{"t": "not-a-date"}, "t"); got != nil {
		t.Errorf("invalid: expected nil, got %v", *got)
	}
}
