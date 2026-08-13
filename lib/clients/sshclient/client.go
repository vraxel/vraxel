package sshclient

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

const (
	defaultPort              = 22
	defaultUser              = "root"
	defaultTimeout           = 30 * time.Second
	defaultKeepAliveInterval = 30 * time.Second
)

var defaultPrivateKey string

func init() {
	if currentUser, err := user.Current(); err == nil {
		defaultPrivateKey = filepath.Join(currentUser.HomeDir, ".ssh", "id_rsa")
	} else {
		defaultPrivateKey = filepath.Join("/root", ".ssh", "id_rsa")
	}
}

// Config holds SSH connection parameters.
type Config struct {
	// Host is the remote host to connect to (required).
	Host string
	// Port is the SSH port. Defaults to 22.
	Port int
	// User is the SSH username. Defaults to "root".
	User string
	// Password for password-based authentication.
	Password string
	// PrivateKey is a file path to the private key.
	PrivateKey string
	// PrivateKeyContent is the raw PEM content of a private key.
	// Takes priority over PrivateKey path.
	PrivateKeyContent string
	// Timeout for the SSH connection. Defaults to 30s.
	Timeout time.Duration
	// KeepAliveInterval is the interval between SSH keep-alive requests.
	// Prevents idle connections from being dropped by firewalls.
	// Defaults to 30s. Set to 0 to disable keep-alive.
	KeepAliveInterval time.Duration
	// ProxyJump is the bastion/jump host config. When set, the client
	// connects to Host through the proxy via SSH tunnel. Supports
	// multi-level chaining (ProxyJump can itself have a ProxyJump).
	// Nil means direct connection.
	ProxyJump *Config
}

// applyDefaults fills in zero-value fields with sensible defaults.
func (c *Config) applyDefaults() {
	if c.Port == 0 {
		c.Port = defaultPort
	}
	if c.User == "" {
		c.User = defaultUser
	}
	if c.Timeout == 0 {
		c.Timeout = defaultTimeout
	}
	if c.KeepAliveInterval == 0 {
		c.KeepAliveInterval = defaultKeepAliveInterval
	}
}

// Client wraps an SSH connection with managed lifecycle.
type Client struct {
	config      Config
	client      *ssh.Client
	proxyClient *Client  // bastion client (auto-managed lifecycle)
	proxyConn   net.Conn // tunnel connection through bastion
	mu          sync.Mutex
	stopKA      chan struct{} // signals keep-alive goroutine to stop
}

// New creates a new Client with the given config. It does not establish
// a connection; call Connect to do so.
func New(config Config) *Client {
	config.applyDefaults()
	if config.ProxyJump != nil {
		config.ProxyJump.applyDefaults()
	}
	return &Client{
		config: config,
	}
}

// Connect establishes the SSH connection using the configured authentication methods.
//
// Authentication priority (matching KubeKey):
//   - Password auth: always included if Password is set
//   - Key auth (exclusive priority):
//     1. PrivateKeyContent — if set, use only this
//     2. PrivateKey path — if set and content not set, use only this
//     3. Default ~/.ssh/id_rsa — only when NO password and no explicit
//     key were given; used if it exists, skipped silently if missing
func (c *Client) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.config.Host == "" {
		return fmt.Errorf("sshclient: host is not set")
	}

	auth, err := c.buildAuthMethods()
	if err != nil {
		return err
	}
	if len(auth) == 0 {
		return fmt.Errorf("sshclient: no authentication method available: provide password, private_key_content, or private_key")
	}

	addr := fmt.Sprintf("%s:%d", c.config.Host, c.config.Port)

	sshConfig := &ssh.ClientConfig{
		User:            c.config.User,
		Auth:            auth,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         c.config.Timeout,
	}

	// Use context deadline if available to set a shorter timeout.
	if deadline, ok := ctx.Deadline(); ok {
		if remaining := time.Until(deadline); remaining < sshConfig.Timeout {
			sshConfig.Timeout = remaining
		}
	}

	if c.config.ProxyJump != nil {
		if err := c.connectViaProxy(ctx, addr, sshConfig); err != nil {
			return err
		}
	} else {
		if err := c.connectDirect(addr, sshConfig); err != nil {
			return err
		}
	}

	// Start SSH keep-alive to prevent idle connections from being dropped
	// by firewalls or intermediate network devices during long-running commands.
	if c.config.KeepAliveInterval > 0 {
		c.stopKA = make(chan struct{})
		go c.keepAlive(c.stopKA, c.config.KeepAliveInterval)
	}

	return nil
}

// connectViaProxy establishes c.client by tunnelling through the
// configured ProxyJump bastion. On success it sets c.client,
// c.proxyClient, c.proxyConn. Extracted from Connect so the proxy path's
// error-cleanup branching stays out of the top-level method.
func (c *Client) connectViaProxy(ctx context.Context, addr string, sshConfig *ssh.ClientConfig) error {
	proxy := New(*c.config.ProxyJump)
	if err := proxy.Connect(ctx); err != nil {
		return fmt.Errorf("sshclient: failed to connect proxy %s:%d: %w",
			c.config.ProxyJump.Host, c.config.ProxyJump.Port, err)
	}

	tunnelConn, err := proxy.client.Dial("tcp", addr)
	if err != nil {
		proxy.Close()
		return fmt.Errorf("sshclient: failed to tunnel to %s: %w", addr, err)
	}

	ncc, chans, reqs, err := ssh.NewClientConn(tunnelConn, addr, sshConfig)
	if err != nil {
		tunnelConn.Close()
		proxy.Close()
		return fmt.Errorf("sshclient: failed to dial %s via proxy: %w", addr, err)
	}

	c.client = ssh.NewClient(ncc, chans, reqs)
	c.proxyClient = proxy
	c.proxyConn = tunnelConn
	return nil
}

// connectDirect establishes c.client with a plain ssh.Dial (no bastion).
// Extracted from Connect for symmetry with connectViaProxy.
func (c *Client) connectDirect(addr string, sshConfig *ssh.ClientConfig) error {
	sshClient, err := ssh.Dial("tcp", addr, sshConfig)
	if err != nil {
		return fmt.Errorf("sshclient: failed to dial %s: %w", addr, err)
	}
	c.client = sshClient
	return nil
}

// Close closes the underlying SSH connection and any proxy resources.
// It is safe to call multiple times.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Stop keep-alive goroutine.
	if c.stopKA != nil {
		close(c.stopKA)
		c.stopKA = nil
	}

	// Close target SSH connection.
	if c.client != nil {
		c.client.Close()
		c.client = nil
	}

	// Close tunnel connection.
	if c.proxyConn != nil {
		c.proxyConn.Close()
		c.proxyConn = nil
	}

	// Cascade close to bastion client.
	if c.proxyClient != nil {
		c.proxyClient.Close()
		c.proxyClient = nil
	}

	return nil
}

// SSHClient returns the underlying *ssh.Client. Returns nil if not connected.
func (c *Client) SSHClient() *ssh.Client {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.client
}

// buildAuthMethods constructs the list of SSH auth methods based on config.
func (c *Client) buildAuthMethods() ([]ssh.AuthMethod, error) {
	var auth []ssh.AuthMethod

	// Password: independent, always add if provided.
	if c.config.Password != "" {
		auth = append(auth, ssh.Password(c.config.Password))
	}

	// Key auth: exclusive priority.
	switch {
	case c.config.PrivateKeyContent != "":
		// Priority 1: raw key content.
		signer, err := ssh.ParsePrivateKey([]byte(c.config.PrivateKeyContent))
		if err != nil {
			return nil, fmt.Errorf("sshclient: failed to parse private key content: %w", err)
		}
		auth = append(auth, ssh.PublicKeys(signer))

	case c.config.PrivateKey != "":
		// Priority 2: explicit key file path.
		signer, err := loadPrivateKeyFile(c.config.PrivateKey, false)
		if err != nil {
			return nil, err
		}
		if signer != nil {
			auth = append(auth, ssh.PublicKeys(signer))
		}

	case c.config.Password != "":
		// A password is the caller's intended credential; do NOT also
		// graft on this server's default ~/.ssh/id_rsa. Offering an
		// unrelated key alongside the password makes every connection
		// attempt that key too -- a guaranteed extra auth failure on any
		// host that doesn't trust it, which burns a MaxAuthTries slot /
		// pam_faillock tally and is what produced the misleading
		// "[none password publickey]" bootstrap failures (bug #385).

	default:
		// Priority 3: default ~/.ssh/id_rsa, used only when neither a
		// password nor an explicit key was given (skip silently if
		// missing).
		signer, err := loadPrivateKeyFile(defaultPrivateKey, true)
		if err != nil {
			return nil, err
		}
		if signer != nil {
			auth = append(auth, ssh.PublicKeys(signer))
		}
	}

	return auth, nil
}

// loadPrivateKeyFile reads and parses a PEM-encoded private key file.
// If allowMissing is true, a missing file returns (nil, nil) instead of an error.
func loadPrivateKeyFile(path string, allowMissing bool) (ssh.Signer, error) {
	_, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			if allowMissing {
				return nil, nil
			}
			return nil, fmt.Errorf("sshclient: private key file not found: %s", path)
		}
		return nil, fmt.Errorf("sshclient: failed to stat private key file %s: %w", path, err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("sshclient: failed to read private key %q: %w", path, err)
	}

	signer, err := ssh.ParsePrivateKey(data)
	if err != nil {
		return nil, fmt.Errorf("sshclient: failed to parse private key %q: %w", path, err)
	}

	return signer, nil
}

// keepAlive sends periodic SSH keep-alive requests to prevent idle
// connections from being dropped by firewalls or NAT devices.
// It runs until the stop channel is closed.
func (c *Client) keepAlive(stop <-chan struct{}, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			c.mu.Lock()
			client := c.client
			c.mu.Unlock()
			if client != nil {
				// SendRequest with wantReply=true; ignore errors — Close will
				// handle cleanup if the connection is dead.
				_, _, _ = client.SendRequest("keepalive@openssh.com", true, nil)
			}
		}
	}
}
