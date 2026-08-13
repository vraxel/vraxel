package connector

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
)

// Connector is the interface for connecting to a host.
// It abstracts the operations required to interact with different types of hosts
// (e.g., local, SSH). Implementations provide mechanisms for initialization,
// cleanup, file transfer, and command execution.
type Connector interface {
	// Init initializes the connection.
	Init(ctx context.Context) error
	// Close closes the connection and releases any resources.
	Close(ctx context.Context) error
	// ExecuteCommand executes a command on the host.
	// Returns stdout, stderr, and error (if any).
	ExecuteCommand(ctx context.Context, cmd string) (stdout, stderr []byte, err error)
	// PutFile copies content from src (as bytes) to dst (path on host) with the specified file mode.
	PutFile(ctx context.Context, src []byte, dst string, mode fs.FileMode) error
	// FetchFile copies a file from src (path on host) to dst (writer).
	FetchFile(ctx context.Context, src string, dst io.Writer) error
	// Resize forwards a window-size change to the active PTY (if the
	// connector allocated one). No-op for connectors that don't speak
	// PTY (LocalConnector, SSH connector with PTY disabled).
	Resize(ctx context.Context, rows, cols uint16) error
}

// LiveOutputter is implemented by connectors that can stream the
// command's PTY bytes to an io.Writer as they arrive. The Connector
// caller assigns a writer once before each ExecuteCommand call so
// the WS log viewer renders character-by-character instead of waiting
// for end-of-task. A nil writer disables streaming and only the
// captured bytes returned by ExecuteCommand are surfaced.
type LiveOutputter interface {
	SetLiveOutput(io.Writer)
}

// GatherFacts defines an interface for retrieving host information.
type GatherFacts interface {
	// HostInfo returns a map of host facts gathered from the system.
	HostInfo(ctx context.Context) (map[string]any, error)
}

// ExitError wraps a command error with the process exit code.
// Use errors.As to extract it from the error chain.
type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string { return e.Err.Error() }
func (e *ExitError) Unwrap() error { return e.Err }

// NewConnector creates a connector for a host that this process can
// reach without a network protocol: the local machine.
//
// SSH lives in the sibling package connector/ssh, and that split is the
// point. the agent runs one play against the box it is installed on and
// never dials anything, so linking a full SSH client and SFTP library
// into every agent would put 0.79 MB of unreachable code on ten thousand
// customer machines -- and contradict the design's central claim that
// the SSH path disappears (design §2.2).
//
// Callers that DO need SSH pass ssh.NewConnector as the executor's
// connector factory (executor.WithConnectorFactory). Asking this factory
// for an SSH connection is a wiring mistake, so it says exactly that
// rather than failing somewhere further along.
//
// Recognised variables:
//
//	connection, become_user, password
func NewConnector(host string, vars map[string]any) (Connector, error) {
	connType, _ := vars["connection"].(string)
	password, _ := vars["password"].(string)

	switch connType {
	case "local":
		return NewLocalConnector(password), nil
	case "ssh", "":
		// An implicit connection to a local address is local; anything
		// else needs the SSH factory this build does not have.
		if connType == "" && IsLocalHost(host) {
			return NewLocalConnector(password), nil
		}
		return nil, fmt.Errorf("host %s needs an SSH connector, which this executor was not given: "+
			"pass executor.WithConnectorFactory(ssh.NewConnector)", host)
	default:
		return nil, fmt.Errorf("unsupported connection type: %s", connType)
	}
}

// IsLocalHost reports whether host refers to the machine this process
// runs on. It matches
// "localhost", well-known loopback addresses, the OS hostname, and any
// IP address assigned to a local network interface.
func IsLocalHost(host string) bool {
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return true
	}

	// Check against the OS hostname.
	if hostname, err := os.Hostname(); err == nil && host == hostname {
		return true
	}

	// Check against local network interface addresses.
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return false
	}
	for _, addr := range addrs {
		var ip net.IP
		switch v := addr.(type) {
		case *net.IPNet:
			ip = v.IP
		case *net.IPAddr:
			ip = v.IP
		default:
			continue
		}
		if ip.String() == host {
			return true
		}
	}

	return false
}
