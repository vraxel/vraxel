package internal

import (
	"context"
	"io"
	"io/fs"
	"strings"
	"testing"
)

func TestShellQuote(t *testing.T) {
	cases := map[string]string{
		"/etc/app":          "'/etc/app'",
		"/tmp/a b":          "'/tmp/a b'",
		"/tmp/x'; rm -rf /": `'/tmp/x'\''; rm -rf /'`,
	}
	for in, want := range cases {
		if got := ShellQuote(in); got != want {
			t.Errorf("ShellQuote(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEnvPrefix(t *testing.T) {
	if got := EnvPrefix(nil); got != "" {
		t.Errorf("EnvPrefix(nil) = %q, want empty", got)
	}
	// Sorted keys keep the emitted command byte-identical across runs.
	got := EnvPrefix(map[string]string{"B": "2", "A": "a b"})
	if got != "export A='a b'; export B='2'; " {
		t.Errorf("EnvPrefix = %q", got)
	}
	// Values are shell-escaped, so a quote cannot break out of the export.
	if got := EnvPrefix(map[string]string{"X": "a'; rm -rf /"}); !strings.Contains(got, `'a'\''; rm -rf /'`) {
		t.Errorf("EnvPrefix did not escape the value: %q", got)
	}
}

// captureConnector records the last shell command and put destination.
type captureConnector struct {
	lastCmd string
	lastPut string
}

func (c *captureConnector) Init(context.Context) error  { return nil }
func (c *captureConnector) Close(context.Context) error { return nil }
func (c *captureConnector) ExecuteCommand(_ context.Context, cmd string) ([]byte, []byte, error) {
	c.lastCmd = cmd
	return nil, nil, nil
}
func (c *captureConnector) PutFile(_ context.Context, _ []byte, dst string, _ fs.FileMode) error {
	c.lastPut = dst
	return nil
}
func (c *captureConnector) FetchFile(context.Context, string, io.Writer) error { return nil }
func (c *captureConnector) Resize(context.Context, uint16, uint16) error       { return nil }

func TestWriteFile_DirectWithoutBecome(t *testing.T) {
	conn := &captureConnector{}
	opts := ExecOptions{Connector: conn}
	if err := WriteFile(context.Background(), opts, []byte("data"), "/etc/app.conf", 0644); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conn.lastPut != "/etc/app.conf" {
		t.Errorf("PutFile dest = %q, want /etc/app.conf", conn.lastPut)
	}
	if conn.lastCmd != "" {
		t.Errorf("no shell command expected without become, got %q", conn.lastCmd)
	}
}

func TestWriteFile_BecomeEscapesDest(t *testing.T) {
	conn := &captureConnector{}
	opts := ExecOptions{Connector: conn, Become: true}
	dest := "/etc/a b/x'; rm -rf /"
	if err := WriteFile(context.Background(), opts, []byte("data"), dest, 0600); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	quoted := ShellQuote(dest)
	if !strings.Contains(conn.lastCmd, "mv /tmp/.ansible_tmp_") {
		t.Errorf("expected temp mv command, got %q", conn.lastCmd)
	}
	if !strings.Contains(conn.lastCmd, quoted) {
		t.Errorf("dest not shell-escaped in %q (want %q)", conn.lastCmd, quoted)
	}
	// The raw unescaped path must never appear verbatim.
	if strings.Contains(conn.lastCmd, "mv /tmp/.ansible_tmp_0 "+dest) {
		t.Errorf("dest interpolated unescaped: %q", conn.lastCmd)
	}
}
