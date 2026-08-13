// Package upgrade replaces the agent's own binary in place.
//
// Self-upgrade is what makes every other agent-side change affordable:
// at ten thousand hosts, a capability that requires an operator to
// re-run an install script is a capability that never ships (design
// §2.5, which is why this moved from P2 into M2).
package upgrade

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
)

const (
	// downloadTimeout bounds fetching one binary.
	downloadTimeout = 10 * time.Minute
	// verifyTimeout bounds the staged binary's self-identification.
	verifyTimeout = 30 * time.Second
	// restartPoll is how often a pending restart re-checks for idleness.
	restartPoll = 15 * time.Second
	// maxBoots is how many times a freshly installed binary may start
	// without reaching the server before it is rolled back.
	maxBoots = 2
)

// Logger is the minimal logging surface this package needs.
type Logger interface {
	Infof(format string, args ...any)
	Warnf(format string, args ...any)
}

// Manager performs upgrades and the boot-time rollback check.
type Manager struct {
	// BinaryPath is the agent executable to replace.
	BinaryPath string
	// StateDir holds the upgrade sentinel.
	StateDir string
	// ServerURL is where the binary is fetched from. The upgrade frame
	// never supplies a URL.
	ServerURL string
	// Version is the version this process is running.
	Version string
	// Busy reports whether work is in flight. A restart waits for it to
	// go false: killing a running playbook to install a new agent would
	// trade a fixed cost (a slightly stale agent) for a variable one (a
	// half-configured host).
	Busy func() bool
	// Exit ends the process so the supervisor restarts it into the new
	// binary. Defaults to os.Exit(0).
	Exit func()
	// Client fetches the binary. Defaults to http.DefaultClient.
	Client *http.Client
	// RestartPoll overrides how often a pending restart re-checks for
	// idleness. Zero uses restartPoll.
	RestartPoll time.Duration
	// VersionOf reads the version out of the staged binary's -version
	// output. It is a hook because the output format belongs to the host
	// program: vr-agent prints its full build string while the server
	// only ever names the short one. Defaults to the trimmed output.
	VersionOf func(output string) string
	Log       Logger
}

// sentinel records an upgrade in flight. It exists from the moment the
// new binary is staged until the new process proves it can reach the
// server.
type sentinel struct {
	FromVersion string `json:"fromVersion"`
	ToVersion   string `json:"toVersion"`
	Boots       int    `json:"boots"`
	StartedUnix int64  `json:"startedUnix"`
}

func (m *Manager) sentinelPath() string { return filepath.Join(m.StateDir, "upgrade.json") }
func (m *Manager) backupPath() string   { return m.BinaryPath + ".old" }

// Apply downloads, verifies and stages the new binary, then restarts
// once the agent is idle. It returns after staging; the restart happens
// on its own goroutine, because the caller is the control-channel frame
// loop and must not block for the length of a running job.
func (m *Manager) Apply(ctx context.Context, up agenttypes.AgentUpgrade) error {
	if up.Version == "" || up.SHA256 == "" {
		return fmt.Errorf("upgrade frame is missing version or digest")
	}
	// Without a path there is nothing to replace. Refusing here keeps the
	// promise the startup warning already made, instead of failing later
	// inside stage() with a rename of "" that reads like a bug.
	if m.BinaryPath == "" {
		return fmt.Errorf("self-upgrade is disabled: the agent could not resolve its own path")
	}
	if up.Version == m.Version {
		m.Log.Infof("upgrade: already running %s", up.Version)
		return nil
	}

	staged := m.BinaryPath + ".new"
	if err := m.download(ctx, staged, up.SHA256); err != nil {
		os.Remove(staged)
		return err
	}
	// A binary that cannot even state its own version is corrupt or built
	// for the wrong machine. Finding that out here costs one exec; finding
	// it out after the swap costs a crash-looping host.
	if err := m.verifyRuns(ctx, staged, up.Version); err != nil {
		os.Remove(staged)
		return err
	}

	if err := m.stage(staged, up.Version); err != nil {
		os.Remove(staged)
		return err
	}
	m.Log.Infof("upgrade: %s staged, restarting once idle", up.Version)
	go m.restartWhenIdle(ctx)
	return nil
}

// download fetches the binary for this platform and checks its digest
// before it ever reaches its final name.
func (m *Manager) download(ctx context.Context, dest, wantSHA string) error {
	url := strings.TrimRight(m.ServerURL, "/") + agenttypes.BinaryPath(runtime.GOOS, runtime.GOARCH)
	dctx, cancel := context.WithTimeout(ctx, downloadTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(dctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := m.client().Do(req)
	if err != nil {
		return fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: http %d", url, resp.StatusCode)
	}

	f, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	h := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(f, h), resp.Body)
	closeErr := f.Close()
	if copyErr != nil {
		return fmt.Errorf("write %s: %w", dest, copyErr)
	}
	if closeErr != nil {
		return closeErr
	}
	if got := hex.EncodeToString(h.Sum(nil)); !strings.EqualFold(got, wantSHA) {
		return fmt.Errorf("binary digest %s does not match the announced %s", got, wantSHA)
	}
	return nil
}

// verifyRuns executes the staged binary with -version and requires it to
// report the version the server announced. Catching a mismatch here is
// what stops an upgrade loop: install a binary that is not what was
// announced and the agent would restart, report the old version, be told
// to upgrade again, forever.
func (m *Manager) verifyRuns(ctx context.Context, path, want string) error {
	vctx, cancel := context.WithTimeout(ctx, verifyTimeout)
	defer cancel()
	out, err := exec.CommandContext(vctx, path, "-version").CombinedOutput()
	if err != nil {
		return fmt.Errorf("staged binary does not run: %v (%s)", err, strings.TrimSpace(string(out)))
	}
	if got := m.versionOf(string(out)); got != want {
		return fmt.Errorf("staged binary reports %q, expected %s", got, want)
	}
	return nil
}

func (m *Manager) versionOf(out string) string {
	if m.VersionOf != nil {
		return m.VersionOf(out)
	}
	return strings.TrimSpace(out)
}

// stage swaps the binaries and writes the sentinel.
//
// The running executable is renamed rather than overwritten: a rename is
// atomic and leaves the running process's open file intact, while writing
// over it would fail with ETXTBSY on Linux.
func (m *Manager) stage(staged, toVersion string) error {
	if err := os.Rename(m.BinaryPath, m.backupPath()); err != nil {
		return fmt.Errorf("back up the current binary: %w", err)
	}
	if err := os.Rename(staged, m.BinaryPath); err != nil {
		// Put the old one back rather than leaving the host with no agent.
		_ = os.Rename(m.backupPath(), m.BinaryPath)
		return fmt.Errorf("install the new binary: %w", err)
	}
	return m.writeSentinel(sentinel{
		FromVersion: m.Version,
		ToVersion:   toVersion,
		StartedUnix: time.Now().Unix(),
	})
}

// restartWhenIdle waits for the agent to finish its work, then exits so
// the supervisor starts the new binary.
func (m *Manager) restartWhenIdle(ctx context.Context) {
	poll := m.RestartPoll
	if poll <= 0 {
		poll = restartPoll
	}
	for m.busy() {
		select {
		case <-ctx.Done():
			return
		case <-time.After(poll):
		}
	}
	m.Log.Infof("upgrade: restarting into the new binary")
	m.exit()
}

// RollbackIfUnhealthy runs at startup. It reports whether it restored
// the previous binary, in which case the caller must exit so the
// supervisor starts it.
//
// The rule is boot count, not a timer: an agent that cannot reach the
// server has no way to ask for help, so the only evidence available is
// "this new binary has now started maxBoots times without ever
// confirming". ConfirmHealthy clears the sentinel on the first successful
// connection, so a working upgrade never reaches the second boot.
func (m *Manager) RollbackIfUnhealthy() (bool, error) {
	s, err := m.readSentinel()
	if err != nil || s == nil {
		return false, err
	}
	s.Boots++
	if s.Boots < maxBoots {
		return false, m.writeSentinel(*s)
	}
	if _, err := os.Stat(m.backupPath()); err != nil {
		// Nothing to roll back to; drop the sentinel so the count does
		// not grow forever.
		m.Log.Warnf("upgrade: %s never confirmed and there is no backup to restore", s.ToVersion)
		return false, m.clearSentinel()
	}
	m.Log.Warnf("upgrade: %s failed to confirm after %d boots, rolling back to %s",
		s.ToVersion, s.Boots, s.FromVersion)
	if err := os.Rename(m.backupPath(), m.BinaryPath); err != nil {
		return false, fmt.Errorf("restore the previous binary: %w", err)
	}
	return true, m.clearSentinel()
}

// ConfirmHealthy is called once the agent has reached the server on the
// new binary. It ends the upgrade: the sentinel goes away, so a later
// restart is an ordinary one, and the backup is removed.
func (m *Manager) ConfirmHealthy() {
	s, err := m.readSentinel()
	if err != nil || s == nil {
		return
	}
	m.Log.Infof("upgrade: %s confirmed", s.ToVersion)
	_ = m.clearSentinel()
	_ = os.Remove(m.backupPath())
}

// Pending reports whether a staged upgrade is waiting for the agent to
// go idle, so the control channel can report pending_restart.
func (m *Manager) Pending() bool {
	s, err := m.readSentinel()
	return err == nil && s != nil && m.busy()
}

func (m *Manager) readSentinel() (*sentinel, error) {
	data, err := os.ReadFile(m.sentinelPath())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var s sentinel
	if err := json.Unmarshal(data, &s); err != nil {
		// A corrupt sentinel must not wedge startup: drop it.
		_ = m.clearSentinel()
		return nil, nil
	}
	return &s, nil
}

func (m *Manager) writeSentinel(s sentinel) error {
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(m.StateDir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(m.sentinelPath(), data, 0o600)
}

func (m *Manager) clearSentinel() error {
	err := os.Remove(m.sentinelPath())
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func (m *Manager) busy() bool {
	return m.Busy != nil && m.Busy()
}

func (m *Manager) exit() {
	if m.Exit != nil {
		m.Exit()
		return
	}
	os.Exit(0)
}

func (m *Manager) client() *http.Client {
	if m.Client != nil {
		return m.Client
	}
	return http.DefaultClient
}
