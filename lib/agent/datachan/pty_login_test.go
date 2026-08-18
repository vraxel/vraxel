package datachan

import (
	"os"
	"os/user"
	"slices"
	"strings"
	"testing"
)

// TestLoginEnvDropsServiceVariables pins the half of the environment that
// belongs to the agent's unit rather than to a person's shell. A program
// run in the terminal that speaks sd_notify would otherwise report
// against the agent's own service.
func TestLoginEnvDropsServiceVariables(t *testing.T) {
	t.Setenv("NOTIFY_SOCKET", "/run/systemd/notify")
	t.Setenv("JOURNAL_STREAM", "8:12345")
	t.Setenv("INVOCATION_ID", "deadbeef")
	t.Setenv("LISTEN_FDS", "1")
	// Gate sd_notify's watchdog ping. A program started here that kept
	// petting it would be petting the AGENT's watchdog, and systemd would
	// stop restarting a hung agent.
	t.Setenv("WATCHDOG_USEC", "30000000")
	t.Setenv("WATCHDOG_PID", "1234")

	env := loginEnv(nil)
	for _, banned := range []string{
		"NOTIFY_SOCKET=", "JOURNAL_STREAM=", "INVOCATION_ID=", "LISTEN_FDS=",
		"WATCHDOG_USEC=", "WATCHDOG_PID=",
	} {
		if slices.ContainsFunc(env, func(kv string) bool { return strings.HasPrefix(kv, banned) }) {
			t.Errorf("%s reached the shell; it belongs to the agent's unit", banned)
		}
	}
}

// TestLoginEnvKeepsSystemSettings is the reason this is a denylist. LANG
// and TZ come from the machine's own configuration, and a shell without
// them renders the filenames on that machine wrong -- which an allowlist
// would have had to predict.
func TestLoginEnvKeepsSystemSettings(t *testing.T) {
	t.Setenv("LANG", "zh_CN.UTF-8")
	t.Setenv("TZ", "Asia/Shanghai")

	env := loginEnv(nil)
	for _, want := range []string{"LANG=zh_CN.UTF-8", "TZ=Asia/Shanghai"} {
		if !slices.Contains(env, want) {
			t.Errorf("%s was dropped; the shell loses the machine's own settings", want)
		}
	}
}

// TestLoginEnvSetsIdentity covers what a login supplies and a forked child
// does not. Without HOME a shell and everything under it disagree about
// where dotfiles live.
func TestLoginEnvSetsIdentity(t *testing.T) {
	me, err := user.Current()
	if err != nil {
		t.Skipf("no passwd entry for this uid: %v", err)
	}

	env := loginEnv(me)
	for _, want := range []string{"HOME=" + me.HomeDir, "USER=" + me.Username, "LOGNAME=" + me.Username} {
		if !slices.Contains(env, want) {
			t.Errorf("missing %q", want)
		}
	}
}

// TestLoginEnvForcesTerm keeps TERM this terminal's, not whatever the
// agent was started under -- systemd gives a service no TERM at all, and
// a shell without one refuses to run anything full-screen.
func TestLoginEnvForcesTerm(t *testing.T) {
	t.Setenv("TERM", "dumb")

	env := loginEnv(nil)
	var last string
	for _, kv := range env {
		if strings.HasPrefix(kv, "TERM=") {
			last = kv
		}
	}
	if last != "TERM=xterm-256color" {
		t.Fatalf("effective TERM = %q, want xterm-256color", last)
	}
}

// TestLoginEnvIsSelfContained guards the fallback: no passwd entry means
// no home and no name, and the shell still has to start.
func TestLoginEnvIsSelfContained(t *testing.T) {
	if got := len(loginEnv(nil)); got < len(os.Environ())-len(serviceEnvPrefixes) {
		t.Fatalf("env shrank unexpectedly: %d entries", got)
	}
}

// TestStartDirPrefersTheCallersChoice keeps StreamOpen.Dir authoritative:
// a file browser opening a shell in a directory means that directory.
func TestStartDirPrefersTheCallersChoice(t *testing.T) {
	me := &user.User{HomeDir: "/root"}
	if got := startDir("/var/log", me); got != "/var/log" {
		t.Fatalf("startDir = %q, want the requested /var/log", got)
	}
}

// TestStartDirDefaultsToHome is the reported bug. An empty Dir makes the
// shell inherit the agent's working directory -- the install path its
// unit names -- so a terminal opened in /opt/vraxel rather than in the
// operator's home.
func TestStartDirDefaultsToHome(t *testing.T) {
	if got := startDir("", &user.User{HomeDir: "/root"}); got != "/root" {
		t.Fatalf("startDir = %q, want /root", got)
	}
}

// TestStartDirWithoutAPasswdEntry covers the container case: no home to
// go to, so the shell has to start anyway rather than fail on a bad path.
func TestStartDirWithoutAPasswdEntry(t *testing.T) {
	if got := startDir("", nil); got != "" {
		t.Fatalf("startDir = %q, want empty so exec inherits", got)
	}
}

// TestShellArgv0MarksALogin pins the other half of the report. Without
// the '-' the shell is not a login shell, /etc/profile never runs, and
// PATH is whatever the service manager handed the agent.
func TestShellArgv0MarksALogin(t *testing.T) {
	if got := shellArgv0(nil, "/bin/bash"); got != "-bash" {
		t.Fatalf("argv0 = %q, want -bash", got)
	}
	if got := shellArgv0(nil, "/bin/sh"); got != "-sh" {
		t.Fatalf("argv0 = %q, want -sh", got)
	}
}

// TestShellArgv0LeavesAnExplicitCommandAlone keeps the login treatment
// off callers that named something: they asked for that program, and
// handing it a '-' argv[0] would tell it something untrue about itself.
func TestShellArgv0LeavesAnExplicitCommandAlone(t *testing.T) {
	if got := shellArgv0([]string{"/usr/bin/top", "-b"}, "/usr/bin/top"); got != "/usr/bin/top" {
		t.Fatalf("argv0 = %q, want the command path untouched", got)
	}
}
