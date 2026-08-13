package hostinfo

import "testing"

func TestParseMemTotalBytes(t *testing.T) {
	const sample = `MemTotal:       16307840 kB
MemFree:          123456 kB
MemAvailable:    8000000 kB
`
	got, err := parseMemTotalBytes([]byte(sample))
	if err != nil {
		t.Fatalf("parseMemTotalBytes: %v", err)
	}
	const want = int64(16307840) * 1024
	if got != want {
		t.Errorf("total = %d, want %d", got, want)
	}
}

func TestParseMemTotalBytes_MissingErrors(t *testing.T) {
	const sample = "MemFree:          123456 kB\n"
	if _, err := parseMemTotalBytes([]byte(sample)); err == nil {
		t.Fatal("expected error when MemTotal is absent, got nil")
	}
}
