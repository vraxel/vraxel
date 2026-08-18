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

	env := loginEnv(nil)
	for _, banned := range []string{"NOTIFY_SOCKET=", "JOURNAL_STREAM=", "INVOCATION_ID=", "LISTEN_FDS="} {
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
