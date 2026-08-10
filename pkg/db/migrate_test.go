package db

import "testing"

func TestParseVersion(t *testing.T) {
	tests := []struct {
		filename string
		want     int64
		wantErr  bool
	}{
		{"20260101000000_initial.up.sql", 20260101000000, false},
		{"20260313100000_drop_host_assignments.up.sql", 20260313100000, false},
		{"100_short.up.sql", 100, false},
		{"bad.up.sql", 0, true},
	}
	for _, tt := range tests {
		got, err := parseVersion(tt.filename)
		if (err != nil) != tt.wantErr {
			t.Errorf("parseVersion(%q) error = %v, wantErr %v", tt.filename, err, tt.wantErr)
			continue
		}
		if got != tt.want {
			t.Errorf("parseVersion(%q) = %d, want %d", tt.filename, got, tt.want)
		}
	}
}
