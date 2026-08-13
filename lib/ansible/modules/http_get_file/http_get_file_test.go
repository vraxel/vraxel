package http_get_file

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"vraxel.io/vraxel/lib/ansible/modules/internal"
)

func serve(t *testing.T, body string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/artifact.bin"
}

func run(t *testing.T, args map[string]any) error {
	t.Helper()
	_, _, err := ModuleHTTPGetFile(context.Background(), internal.ExecOptions{Args: args})
	return err
}

func TestDownloadVerifiesDigest(t *testing.T) {
	const body = "artifact-contents"
	sum := sha256.Sum256([]byte(body))
	dest := filepath.Join(t.TempDir(), "artifact.bin")

	if err := run(t, map[string]any{
		"url":    serve(t, body),
		"dest":   dest,
		"sha256": hex.EncodeToString(sum[:]),
	}); err != nil {
		t.Fatalf("download with a matching digest failed: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil || string(got) != body {
		t.Fatalf("file content = %q, err = %v", got, err)
	}
}

// TestDigestMismatchLeavesNoFile is the point of the check: a tampered
// or truncated download must not become the destination file.
func TestDigestMismatchLeavesNoFile(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "artifact.bin")

	err := run(t, map[string]any{
		"url":    serve(t, "tampered"),
		"dest":   dest,
		"sha256": hex.EncodeToString(make([]byte, 32)),
	})
	if err == nil {
		t.Fatal("a mismatching digest was accepted")
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatal("the destination file was written despite the mismatch")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("temp file left behind: %v", entries)
	}
}

// TestDigestOptional keeps the existing callers working: every current
// use of this module passes no checksum.
func TestDigestOptional(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "artifact.bin")
	if err := run(t, map[string]any{"url": serve(t, "no-digest"), "dest": dest}); err != nil {
		t.Fatalf("download without a digest failed: %v", err)
	}
	if got, _ := os.ReadFile(dest); string(got) != "no-digest" {
		t.Fatalf("file content = %q", got)
	}
}
