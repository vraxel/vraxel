package hostinfo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Real `sshd -T` output, trimmed to the lines this collector reads. The
// list options print ONE LINE PER ENTRY -- "AllowGroups sudo adm" comes
// back as two allowgroups lines -- which is the whole reason this is
// tested against captured output rather than something invented.
const sshdTOutput = `port 22
port 2222
addressfamily any
listenaddress [::]:22
maxauthtries 6
permitrootlogin yes
pubkeyauthentication yes
passwordauthentication yes
kbdinteractiveauthentication no
permitemptypasswords no
allowusers alice
allowusers bob
allowgroups sudo
allowgroups adm
denyusers mallory
ciphers chacha20-poly1305@openssh.com,aes128-ctr
`

func TestParseSSHDConfig(t *testing.T) {
	c := parseSSHDConfig([]byte(sshdTOutput))
	if c == nil {
		t.Fatal("parsed nothing")
	}
	// Every entry, not the last one. An access list reported as one
	// account where the machine admits several is a wrong answer about
	// who can log in.
	if strings.Join(c.AllowUsers, ",") != "alice,bob" {
		t.Errorf("allowUsers = %v", c.AllowUsers)
	}
	if strings.Join(c.AllowGroups, ",") != "sudo,adm" {
		t.Errorf("allowGroups = %v", c.AllowGroups)
	}
	if len(c.Ports) != 2 || c.Ports[0] != 22 || c.Ports[1] != 2222 {
		t.Errorf("ports = %v", c.Ports)
	}
	// Four values, not a boolean: prohibit-password is the common
	// hardened setting and is neither "yes" nor "no".
	if c.PermitRootLogin != "yes" {
		t.Errorf("permitRootLogin = %q", c.PermitRootLogin)
	}
	if !c.PasswordAuth || c.KbdInteractiveAuth || !c.PubkeyAuth || c.PermitEmptyPasswords {
		t.Errorf("auth flags = %+v", c)
	}
	if c.MaxAuthTries != 6 {
		t.Errorf("maxAuthTries = %d", c.MaxAuthTries)
	}
	// Nothing at all means sshd did not answer, which is not the same as
	// a daemon that answered with defaults.
	if parseSSHDConfig(nil) != nil {
		t.Error("empty output parsed as a configuration")
	}
}

func TestCountMatchBlocks(t *testing.T) {
	// Case-insensitive keyword, a commented-out block that does not
	// count, and a bare "Match" with no criteria that is not a block.
	got := countMatchBlocks([]byte(
		"Port 22\nMatch User deploy\n  PasswordAuthentication yes\n" +
			"#Match Address 10.0.0.0/8\nmatch group wheel\n  X11Forwarding yes\nMatch\n"))
	if got != 2 {
		t.Errorf("counted %d match blocks, want 2", got)
	}
}

func TestIncludeGlobs(t *testing.T) {
	got := includeGlobs([]byte("Include /etc/ssh/sshd_config.d/*.conf\n# Include /nope\nPort 22\n"))
	if len(got) != 1 || got[0] != "/etc/ssh/sshd_config.d/*.conf" {
		t.Errorf("globs = %v", got)
	}
}

// The exec guards, each covering a way an unattended collector would
// otherwise fail silently.
func TestRunToolGuards(t *testing.T) {
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("no /bin/sh")
	}
	if got := runTool("/bin/sh", "-c", "echo hello"); strings.TrimSpace(string(got)) != "hello" {
		t.Errorf("stdout = %q", got)
	}
	// A tool that refused to answer has not given a partial answer worth
	// reporting.
	if got := runTool("/bin/sh", "-c", "echo partial; exit 3"); got != nil {
		t.Errorf("non-zero exit returned %q, want nothing", got)
	}
	// Stderr must not reach the parser: a warning line interleaved into
	// "key value" output would become a setting.
	if got := runTool("/bin/sh", "-c", "echo warn >&2; echo real"); strings.TrimSpace(string(got)) != "real" {
		t.Errorf("stderr leaked into stdout: %q", got)
	}
	// Truncated output is refused rather than parsed as if whole.
	if got := runTool("/bin/sh", "-c", "yes x | head -c 4000000"); got != nil {
		t.Errorf("oversized output returned %d bytes, want nothing", len(got))
	}
	// A missing binary is not an error to propagate.
	if got := runTool("/nonexistent/tool"); got != nil {
		t.Errorf("missing binary returned %q", got)
	}
}

// The one that matters most: exec.CommandContext kills the direct child
// only, and a grandchild holding the pipe open blocks the read forever.
// WaitDelay is what stops it, and a collector without it wedges the loop
// that reports the machine.
func TestRunToolDoesNotHangOnAGrandchild(t *testing.T) {
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("no /bin/sh")
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		// The child exits at once; the backgrounded grandchild keeps
		// stdout open far longer than any deadline here.
		runTool("/bin/sh", "-c", "sleep 300 & exit 0")
	}()
	select {
	case <-done:
	case <-time.After(toolTimeout + toolKillDelay + 5*time.Second):
		t.Fatal("runTool hung on a grandchild holding the pipe")
	}
}

func TestLookToolTakesTheFirstThatExists(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "sshd")
	if err := os.WriteFile(real, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := lookTool(filepath.Join(dir, "nope"), real); got != real {
		t.Errorf("lookTool = %q, want %q", got, real)
	}
	// A directory is not a tool.
	if got := lookTool(dir); got != "" {
		t.Errorf("lookTool returned a directory: %q", got)
	}
	if got := lookTool(); got != "" {
		t.Errorf("lookTool with no candidates = %q", got)
	}
}
