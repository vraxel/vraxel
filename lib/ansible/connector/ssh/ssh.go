package ssh

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"strings"
	"sync"

	"vraxel.io/vraxel/lib/ansible/connector"
	"vraxel.io/vraxel/lib/clients/sshclient"
)

// Compile-time interface checks.
var _ connector.Connector = &SSHConnector{}
var _ connector.GatherFacts = &SSHConnector{}
var _ connector.LiveOutputter = &SSHConnector{}

// SSHConnector implements Connector and GatherFacts using SSH.
// It delegates connection management, command execution, and file transfer
// to the underlying sshclient.Client.
//
// PTY mode (controlled via EnablePty / DisablePty) makes ExecuteCommand
// allocate a remote PTY of the configured size before running the
// command, so dnf / yum / mkfs / parted etc. emit properly column-
// aligned output for that size — exactly what xterm.js on the
// frontend expects to render. PTY mode also lets the caller forward
// dynamic browser-window resizes via Resize and stream live PTY
// bytes via SetLiveOutput.
type SSHConnector struct {
	client     *sshclient.Client
	host       string
	password   string // for sudo
	become     bool
	becomeUser string

	// PTY config + live state. ptyEnabled controls whether
	// ExecuteCommand uses Client.ExecPty vs Client.Exec / ExecWithSudo.
	// resizeCh is created per ExecuteCommand call and torn down when
	// the command returns; Resize() forwards onto the current channel
	// non-blocking so a barrage of frontend FitAddon events doesn't
	// pile up unbounded.
	mu         sync.Mutex
	ptyEnabled bool
	ptyRows    uint16
	ptyCols    uint16
	resizeCh   chan sshclient.WindowSize
	liveOutput io.Writer
}

// NewSSHConnector creates an SSH connector from the given config.
// The connection is not established until Init is called. PTY mode is
// off by default — call EnablePty(rows, cols) to opt in.
func NewSSHConnector(config sshclient.Config, become bool, becomeUser string) *SSHConnector {
	return &SSHConnector{
		client:     sshclient.New(config),
		host:       config.Host,
		password:   config.Password,
		become:     become,
		becomeUser: becomeUser,
	}
}

// EnablePty turns on PTY-allocated command execution with the given
// initial window size. The first ExecuteCommand call after this allocates
// a PTY of (rows, cols); subsequent Resize calls update the active PTY.
// Idempotent — re-calling overwrites the saved size for the next session.
func (s *SSHConnector) EnablePty(rows, cols uint16) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ptyEnabled = true
	s.ptyRows = rows
	s.ptyCols = cols
}

// DisablePty turns off PTY allocation. Subsequent ExecuteCommand calls
// fall back to non-PTY Exec/ExecWithSudo.
func (s *SSHConnector) DisablePty() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ptyEnabled = false
}

// SetLiveOutput attaches a live byte sink for the PTY-allocated command
// stream. Set to nil to disable live streaming. Implements LiveOutputter
// so the playbook executor can wire a tracker / WS log writer in
// without a type assertion at every call site.
func (s *SSHConnector) SetLiveOutput(w io.Writer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.liveOutput = w
}

// Resize forwards a new window size onto the currently-running
// ExecuteCommand's PTY. No-op when no PTY is open (between commands or
// PTY mode off). Drops the message if the channel is full rather than
// blocking — frontend FitAddon can fire a burst of resize events on
// rapid drag, and the latest-wins semantic matches what a real
// terminal sees.
func (s *SSHConnector) Resize(_ context.Context, rows, cols uint16) error {
	s.mu.Lock()
	ch := s.resizeCh
	s.mu.Unlock()
	if ch == nil {
		return nil
	}
	select {
	case ch <- sshclient.WindowSize{Rows: rows, Cols: cols}:
	default:
	}
	return nil
}

// Init establishes the SSH connection.
func (s *SSHConnector) Init(ctx context.Context) error {
	return s.client.Connect(ctx)
}

// Close closes the SSH connection and releases resources.
func (s *SSHConnector) Close(_ context.Context) error {
	return s.client.Close()
}

// ExecuteCommand executes a command on the remote host via SSH.
// If become is true, the command is wrapped with sudo. If becomeUser is
// set, "sudo -u <user>" is used. The sudo password is supplied via the
// sshclient.ExecWithSudo mechanism when available.
//
// When PTY mode is enabled (EnablePty) AND a live output writer is
// attached (SetLiveOutput), the command runs inside an allocated PTY
// whose initial size matches the saved (rows, cols) and honors live
// Resize / SetLiveOutput updates. PTY merges stdout and stderr — the
// returned stderr is nil — but ansible task plumbing already keys on
// exit code, so single-stream output is fine.
//
// Without a live writer, even if PTY is "enabled", we fall back to the
// plain Exec / ExecWithSudo paths. The PTY+sudo+stdin-prefeed path has
// an intermittent first-output drop where short commands occasionally
// return empty stdout (sshclient.ExecPty receives 0 bytes despite
// rc=0 and err=nil; reproduced repeatedly in add-disks runs from
// 172.24.161.247 — facts-gathering tasks like `uname -m` would land
// stdout=[] and trip the common/facts asserts). Only the cloud-host
// bootstrap path needs PTY for proper xterm.js streaming, and it
// always sets a LiveOutput writer; every other caller (add-disks,
// extend-disk, remove-disk, agent install, etc.) collects output into
// a buffer and never relies on TTY formatting.
func (s *SSHConnector) ExecuteCommand(ctx context.Context, cmd string) ([]byte, []byte, error) {
	s.mu.Lock()
	usePty := s.ptyEnabled
	rows, cols := s.ptyRows, s.ptyCols
	live := s.liveOutput
	s.mu.Unlock()

	if usePty && live != nil {
		actualCmd, stdin := s.buildPtyCommand(cmd)

		// Wire up a per-call resize channel. Buffered so Resize never
		// blocks the WS handler; reader (sshclient.ExecPty) drains it
		// for the lifetime of the command.
		ch := make(chan sshclient.WindowSize, 8)
		s.mu.Lock()
		s.resizeCh = ch
		s.mu.Unlock()
		defer func() {
			s.mu.Lock()
			s.resizeCh = nil
			s.mu.Unlock()
			close(ch)
		}()

		result, err := s.client.ExecPty(ctx, actualCmd, sshclient.PtyOpts{
			InitialSize: sshclient.WindowSize{Rows: rows, Cols: cols},
			ResizeCh:    ch,
			LiveOutput:  live,
			Stdin:       stdin,
			// Disable PTY echo unconditionally: deploy paths never have
			// a human typing on stdin, and disabling echo prevents the
			// pre-fed sudo password (when present) from leaking into
			// LiveOutput. Tools like dnf / mkfs do not depend on stdin
			// echo for their TTY-formatted output.
			EchoOff: true,
		})
		if result != nil {
			return normalizePtyNewlines(result.Stdout), nil, wrapExitCode(err, result.ExitCode)
		}
		return nil, nil, err
	}

	if s.become {
		result, err := s.client.ExecWithSudo(ctx, cmd, s.password)
		if result != nil {
			return result.Stdout, result.Stderr, wrapExitCode(err, result.ExitCode)
		}
		return nil, nil, err
	}

	result, err := s.client.Exec(ctx, cmd)
	if result != nil {
		return result.Stdout, result.Stderr, wrapExitCode(err, result.ExitCode)
	}
	return nil, nil, err
}

// normalizePtyNewlines turns the CRLF a PTY produces back into LF.
//
// A terminal's ONLCR mode rewrites every \n the command emits as \r\n. That
// is a property of the transport, not of the output, but it reaches every
// consumer: a task's registered stdout would end in \r, so `when`, `assert`,
// `failed_when` and `until` comparing it against a plain string all fail --
// and they fail invisibly, because the two values print identically. Only
// the returned bytes are normalized; the live writer still receives the raw
// stream, which is what a terminal renderer needs.
//
// Bare \r (progress bars redrawing a line) is deliberately left alone.
func normalizePtyNewlines(b []byte) []byte {
	if !bytes.Contains(b, []byte("\r\n")) {
		return b
	}
	return bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
}

// buildPtyCommand returns (commandLine, stdin) for the PTY path:
//   - no become: cmd as-is, no stdin.
//   - become without password: `sudo [-u user] sh -c '<cmd>'`, no stdin.
//   - become with password: `sudo -p ” [-u user] -S sh -c '<cmd>'`,
//     stdin pre-fed with password+\n.  -p ” suppresses sudo's prompt
//     so nothing extra reaches LiveOutput; -S takes the password from
//     stdin instead of /dev/tty. PTY echo is disabled by the caller
//     (EchoOff) so the password bytes don't loop back.
//
// CRITICAL: cmd MUST be wrapped in single quotes (mirroring
// ExecWithSudo). Double quotes (e.g. via Go's %q) let the OUTER
// SSH login shell expand $VARs in cmd before sudo ever runs — so
// `. /etc/os-release && echo $VERSION_ID` becomes
// `. /etc/os-release && echo  ` because the outer shell has no
// VERSION_ID. This silently broke fact-gathering on PTY-default ON.
func (s *SSHConnector) buildPtyCommand(cmd string) (string, io.Reader) {
	if !s.become {
		return cmd, nil
	}
	escaped := strings.ReplaceAll(cmd, "'", `'\''`)
	userArg := ""
	if s.becomeUser != "" && s.becomeUser != "root" {
		userArg = fmt.Sprintf(" -u %s", s.becomeUser)
	}
	if s.password == "" {
		return fmt.Sprintf("sudo%s sh -c '%s'", userArg, escaped), nil
	}
	return fmt.Sprintf("sudo -p '' -S%s sh -c '%s'", userArg, escaped),
		strings.NewReader(s.password + "\n")
}

// wrapExitCode wraps an error with ExitError when the exit code is non-zero.
func wrapExitCode(err error, code int) error {
	if err != nil && code != 0 {
		return &connector.ExitError{Code: code, Err: err}
	}
	return err
}

// PutFile uploads content to a remote path via SFTP.
func (s *SSHConnector) PutFile(_ context.Context, src []byte, dst string, mode fs.FileMode) error {
	return s.client.PutFile(src, dst, mode)
}

// FetchFile downloads a remote file via SFTP.
func (s *SSHConnector) FetchFile(_ context.Context, src string, dst io.Writer) error {
	return s.client.FetchFile(src, dst)
}

// HostInfo gathers remote system information by executing commands and
// reading files over the SSH connection. It collects OS release info,
// kernel version, hostname, architecture, CPU info, and memory info.
func (s *SSHConnector) HostInfo(ctx context.Context) (map[string]any, error) {
	// OS information
	osVars := make(map[string]any)

	var osRelease bytes.Buffer
	if err := s.FetchFile(ctx, "/etc/os-release", &osRelease); err != nil {
		return nil, fmt.Errorf("failed to read /etc/os-release: %w", err)
	}
	osVars["os_release"] = connector.ParseKeyValues(osRelease.Bytes(), "=")

	kernel, stderr, err := s.ExecuteCommand(ctx, "uname -r")
	if err != nil {
		return nil, fmt.Errorf("failed to get kernel version: %w (stderr: %s)", err, string(stderr))
	}
	osVars["kernel_version"] = string(bytes.TrimSpace(kernel))

	hostname, stderr, err := s.ExecuteCommand(ctx, "hostname")
	if err != nil {
		return nil, fmt.Errorf("failed to get hostname: %w (stderr: %s)", err, string(stderr))
	}
	osVars["hostname"] = string(bytes.TrimSpace(hostname))

	arch, stderr, err := s.ExecuteCommand(ctx, "arch")
	if err != nil {
		return nil, fmt.Errorf("failed to get architecture: %w (stderr: %s)", err, string(stderr))
	}
	osVars["architecture"] = string(bytes.TrimSpace(arch))

	// Process information
	procVars := make(map[string]any)

	var cpuInfo bytes.Buffer
	if err := s.FetchFile(ctx, "/proc/cpuinfo", &cpuInfo); err != nil {
		return nil, fmt.Errorf("failed to read /proc/cpuinfo: %w", err)
	}
	procVars["cpu"] = connector.ParseLines(cpuInfo.Bytes(), ":")

	var memInfo bytes.Buffer
	if err := s.FetchFile(ctx, "/proc/meminfo", &memInfo); err != nil {
		return nil, fmt.Errorf("failed to read /proc/meminfo: %w", err)
	}
	procVars["memory"] = connector.ParseKeyValues(memInfo.Bytes(), ":")

	return map[string]any{
		"os":      osVars,
		"process": procVars,
	}, nil
}
