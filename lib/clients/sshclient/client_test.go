package sshclient

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// generateTestKeyPEM generates an Ed25519 private key and returns the
// PEM-encoded bytes suitable for SSH authentication.
func generateTestKeyPEM(t *testing.T) []byte {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate ed25519 key: %v", err)
	}
	block, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		t.Fatalf("failed to marshal private key: %v", err)
	}
	return pem.EncodeToMemory(block)
}

func TestConfigDefaults(t *testing.T) {
	cfg := Config{Host: "example.com"}
	cfg.applyDefaults()

	if cfg.Port != 22 {
		t.Errorf("expected default port 22, got %d", cfg.Port)
	}
	if cfg.User != "root" {
		t.Errorf("expected default user %q, got %q", "root", cfg.User)
	}
	if cfg.Timeout != 30*time.Second {
		t.Errorf("expected default timeout 30s, got %v", cfg.Timeout)
	}
}

func TestConfigPreservesExplicitValues(t *testing.T) {
	cfg := Config{
		Host:    "example.com",
		Port:    2222,
		User:    "admin",
		Timeout: 10 * time.Second,
	}
	cfg.applyDefaults()

	if cfg.Port != 2222 {
		t.Errorf("expected port 2222, got %d", cfg.Port)
	}
	if cfg.User != "admin" {
		t.Errorf("expected user %q, got %q", "admin", cfg.User)
	}
	if cfg.Timeout != 10*time.Second {
		t.Errorf("expected timeout 10s, got %v", cfg.Timeout)
	}
}

func TestNewAppliesDefaults(t *testing.T) {
	c := New(Config{Host: "example.com"})
	if c.config.Port != 22 {
		t.Errorf("New should apply default port, got %d", c.config.Port)
	}
	if c.config.User != "root" {
		t.Errorf("New should apply default user, got %q", c.config.User)
	}
	if c.config.Timeout != 30*time.Second {
		t.Errorf("New should apply default timeout, got %v", c.config.Timeout)
	}
}

func TestNewDoesNotConnect(t *testing.T) {
	c := New(Config{Host: "example.com"})
	if c.SSHClient() != nil {
		t.Error("New should not establish a connection")
	}
}

func TestBuildAuthMethods_PasswordOnly(t *testing.T) {
	// With default fallback, if ~/.ssh/id_rsa doesn't exist, it's skipped silently.
	// So password-only works when the default key file is missing.
	c := New(Config{
		Host:     "example.com",
		Password: "secret",
	})

	auth, err := c.buildAuthMethods()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(auth) == 0 {
		t.Fatal("expected at least one auth method")
	}
}

func TestBuildAuthMethods_PasswordSkipsDefaultKey(t *testing.T) {
	// A present default ~/.ssh/id_rsa must NOT be offered alongside a
	// password: that makes every password connection also attempt an
	// unrelated key -- a guaranteed extra auth failure. It is the
	// misleading "publickey" in the "[none password publickey]" error and
	// adds needless pam_faillock pressure (seen while diagnosing #385).
	keyPEM := generateTestKeyPEM(t)
	keyPath := filepath.Join(t.TempDir(), "id_rsa")
	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		t.Fatalf("failed to write default key file: %v", err)
	}
	origDefault := defaultPrivateKey
	defaultPrivateKey = keyPath
	defer func() { defaultPrivateKey = origDefault }()

	c := New(Config{Host: "example.com", Password: "secret"})
	auth, err := c.buildAuthMethods()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(auth) != 1 {
		t.Fatalf("password-only must yield exactly 1 auth method (password, no default key), got %d", len(auth))
	}
}

func TestBuildAuthMethods_DefaultKeyUsedWhenNoPassword(t *testing.T) {
	// No password and no explicit key: the default key IS still used.
	keyPEM := generateTestKeyPEM(t)
	keyPath := filepath.Join(t.TempDir(), "id_rsa")
	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		t.Fatalf("failed to write default key file: %v", err)
	}
	origDefault := defaultPrivateKey
	defaultPrivateKey = keyPath
	defer func() { defaultPrivateKey = origDefault }()

	c := New(Config{Host: "example.com"})
	auth, err := c.buildAuthMethods()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(auth) != 1 {
		t.Fatalf("expected default key to be used (1 auth method), got %d", len(auth))
	}
}

func TestBuildAuthMethods_PrivateKeyContent(t *testing.T) {
	keyPEM := generateTestKeyPEM(t)

	c := New(Config{
		Host:              "example.com",
		PrivateKeyContent: string(keyPEM),
	})

	auth, err := c.buildAuthMethods()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(auth) == 0 {
		t.Fatal("expected at least one auth method from key content")
	}
}

func TestBuildAuthMethods_PrivateKeyFile(t *testing.T) {
	keyPEM := generateTestKeyPEM(t)

	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test_key")
	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		t.Fatalf("failed to write key file: %v", err)
	}

	c := New(Config{
		Host:       "example.com",
		PrivateKey: keyPath,
	})

	auth, err := c.buildAuthMethods()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(auth) == 0 {
		t.Fatal("expected at least one auth method from key file")
	}
}

func TestBuildAuthMethods_PrivateKeyContentPriority(t *testing.T) {
	keyPEM := generateTestKeyPEM(t)

	// Set both content and a non-existent path. Content should take priority
	// and the path should never be accessed.
	c := New(Config{
		Host:              "example.com",
		PrivateKeyContent: string(keyPEM),
		PrivateKey:        "/nonexistent/should/not/be/read",
	})

	auth, err := c.buildAuthMethods()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(auth) == 0 {
		t.Fatal("expected auth method from key content")
	}
}

func TestBuildAuthMethods_ExplicitKeyFileNotFound(t *testing.T) {
	c := New(Config{
		Host:       "example.com",
		PrivateKey: "/nonexistent/path/to/key",
	})

	_, err := c.buildAuthMethods()
	if err == nil {
		t.Fatal("expected error for missing explicit key file")
	}
}

func TestBuildAuthMethods_NoAuthAvailable(t *testing.T) {
	// Override defaultPrivateKey to point to a non-existent path.
	origDefault := defaultPrivateKey
	defaultPrivateKey = "/nonexistent/default/key"
	defer func() { defaultPrivateKey = origDefault }()

	c := New(Config{
		Host: "example.com",
		// No password, no key content, no key path.
	})

	auth, err := c.buildAuthMethods()
	if err != nil {
		t.Fatalf("buildAuthMethods should not error for missing default key, got: %v", err)
	}
	if len(auth) != 0 {
		t.Fatalf("expected zero auth methods, got %d", len(auth))
	}
}

func TestConnect_EmptyHost(t *testing.T) {
	c := New(Config{})
	err := c.Connect(t.Context())
	if err == nil {
		t.Fatal("expected error for empty host")
	}
}

func TestConnect_NoAuth(t *testing.T) {
	origDefault := defaultPrivateKey
	defaultPrivateKey = "/nonexistent/default/key"
	defer func() { defaultPrivateKey = origDefault }()

	c := New(Config{Host: "example.com"})
	err := c.Connect(t.Context())
	if err == nil {
		t.Fatal("expected error when no auth method available")
	}
}

func TestCloseWithoutConnect(t *testing.T) {
	c := New(Config{Host: "example.com"})
	if err := c.Close(); err != nil {
		t.Fatalf("Close on unconnected client should not error, got: %v", err)
	}
}

func TestBuildAuthMethods_InvalidKeyContent(t *testing.T) {
	c := New(Config{
		Host:              "example.com",
		PrivateKeyContent: "not-a-valid-pem-key",
	})

	_, err := c.buildAuthMethods()
	if err == nil {
		t.Fatal("expected error for invalid key content")
	}
}

func TestBuildAuthMethods_PasswordAndKeyContent(t *testing.T) {
	keyPEM := generateTestKeyPEM(t)

	c := New(Config{
		Host:              "example.com",
		Password:          "secret",
		PrivateKeyContent: string(keyPEM),
	})

	auth, err := c.buildAuthMethods()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should have both password and public key methods.
	if len(auth) != 2 {
		t.Fatalf("expected 2 auth methods (password + key), got %d", len(auth))
	}
}

func TestNewWithProxyJump(t *testing.T) {
	c := New(Config{
		Host:     "target.example.com",
		Password: "target-pass",
		ProxyJump: &Config{
			Host:     "bastion.example.com",
			Password: "bastion-pass",
		},
	})
	if c.config.ProxyJump == nil {
		t.Fatal("expected ProxyJump config to be set")
	}
	if c.config.ProxyJump.Host != "bastion.example.com" {
		t.Errorf("expected bastion host, got %q", c.config.ProxyJump.Host)
	}
	// ProxyJump config should also have defaults applied
	if c.config.ProxyJump.Port != 22 {
		t.Errorf("expected default port 22 on ProxyJump, got %d", c.config.ProxyJump.Port)
	}
	if c.config.ProxyJump.User != "root" {
		t.Errorf("expected default user on ProxyJump, got %q", c.config.ProxyJump.User)
	}
}

func TestBuildAuthMethods_InvalidKeyFile(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "bad_key")
	if err := os.WriteFile(keyPath, []byte("not-a-valid-key"), 0600); err != nil {
		t.Fatalf("failed to write key file: %v", err)
	}

	c := New(Config{
		Host:       "example.com",
		PrivateKey: keyPath,
	})

	_, err := c.buildAuthMethods()
	if err == nil {
		t.Fatal("expected error for invalid key file content")
	}
}

// startTestSSHServer starts a minimal SSH server that accepts connections
// with password auth. It returns the listener address and a cleanup function.
// If allowTunnel is true, the server handles "direct-tcpip" channel requests
// by dialing the requested address (acting as a bastion).
func startTestSSHServer(t *testing.T, password string, allowTunnel bool) (addr string, cleanup func()) {
	t.Helper()

	_, hostPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate host key: %v", err)
	}
	hostSigner, err := ssh.NewSignerFromKey(hostPriv)
	if err != nil {
		t.Fatalf("failed to create host signer: %v", err)
	}

	serverConfig := &ssh.ServerConfig{
		PasswordCallback: func(conn ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			if string(pass) == password {
				return nil, nil
			}
			return nil, fmt.Errorf("password rejected")
		},
	}
	serverConfig.AddHostKey(hostSigner)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go handleTestSSHConn(conn, serverConfig, allowTunnel)
		}
	}()

	return ln.Addr().String(), func() { ln.Close() }
}

func handleTestSSHConn(conn net.Conn, config *ssh.ServerConfig, allowTunnel bool) {
	srvConn, chans, reqs, err := ssh.NewServerConn(conn, config)
	if err != nil {
		conn.Close()
		return
	}
	defer srvConn.Close()

	go ssh.DiscardRequests(reqs)

	for newChan := range chans {
		switch newChan.ChannelType() {
		case "session":
			ch, _, err := newChan.Accept()
			if err != nil {
				continue
			}
			ch.Close()

		case "direct-tcpip":
			if !allowTunnel {
				newChan.Reject(ssh.Prohibited, "tunneling not allowed")
				continue
			}

			var payload struct {
				DestHost string
				DestPort uint32
				SrcHost  string
				SrcPort  uint32
			}
			if err := ssh.Unmarshal(newChan.ExtraData(), &payload); err != nil {
				newChan.Reject(ssh.ConnectionFailed, "bad payload")
				continue
			}

			targetAddr := net.JoinHostPort(payload.DestHost, fmt.Sprintf("%d", payload.DestPort))
			targetConn, err := net.Dial("tcp", targetAddr)
			if err != nil {
				newChan.Reject(ssh.ConnectionFailed, err.Error())
				continue
			}

			ch, _, err := newChan.Accept()
			if err != nil {
				targetConn.Close()
				continue
			}

			go func() {
				defer ch.Close()
				defer targetConn.Close()
				go func() { _, _ = io.Copy(ch, targetConn) }()
				_, _ = io.Copy(targetConn, ch)
			}()

		default:
			newChan.Reject(ssh.UnknownChannelType, "unknown channel type")
		}
	}
}

func TestClose_ProxyJumpCascade(t *testing.T) {
	targetAddr, targetCleanup := startTestSSHServer(t, "target-pass", false)
	defer targetCleanup()

	bastionAddr, bastionCleanup := startTestSSHServer(t, "bastion-pass", true)
	defer bastionCleanup()

	bastionHost, bastionPortStr, _ := net.SplitHostPort(bastionAddr)
	bastionPort := 0
	fmt.Sscanf(bastionPortStr, "%d", &bastionPort)

	targetHost, targetPortStr, _ := net.SplitHostPort(targetAddr)
	targetPort := 0
	fmt.Sscanf(targetPortStr, "%d", &targetPort)

	c := New(Config{
		Host:     targetHost,
		Port:     targetPort,
		Password: "target-pass",
		ProxyJump: &Config{
			Host:     bastionHost,
			Port:     bastionPort,
			Password: "bastion-pass",
		},
	})

	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	if err := c.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Verify all resources are nil after close.
	if c.SSHClient() != nil {
		t.Error("expected client to be nil after Close")
	}
	if c.proxyClient != nil {
		t.Error("expected proxyClient to be nil after Close")
	}
	if c.proxyConn != nil {
		t.Error("expected proxyConn to be nil after Close")
	}

	// Close again should be safe (idempotent).
	if err := c.Close(); err != nil {
		t.Fatalf("second Close should not error: %v", err)
	}
}

func TestConnect_ProxyJumpBastionFails(t *testing.T) {
	c := New(Config{
		Host:     "127.0.0.1",
		Port:     1,
		Password: "target-pass",
		ProxyJump: &Config{
			Host:     "127.0.0.1",
			Port:     1, // nothing listening
			Password: "bastion-pass",
		},
	})

	err := c.Connect(context.Background())
	if err == nil {
		c.Close()
		t.Fatal("expected error when bastion is unreachable")
	}
	if !strings.Contains(err.Error(), "proxy") {
		t.Errorf("expected proxy-related error, got: %v", err)
	}
}

func TestConnect_ProxyJumpBadTargetPassword(t *testing.T) {
	targetAddr, targetCleanup := startTestSSHServer(t, "correct-pass", false)
	defer targetCleanup()

	bastionAddr, bastionCleanup := startTestSSHServer(t, "bastion-pass", true)
	defer bastionCleanup()

	bastionHost, bastionPortStr, _ := net.SplitHostPort(bastionAddr)
	bastionPort := 0
	fmt.Sscanf(bastionPortStr, "%d", &bastionPort)

	targetHost, targetPortStr, _ := net.SplitHostPort(targetAddr)
	targetPort := 0
	fmt.Sscanf(targetPortStr, "%d", &targetPort)

	c := New(Config{
		Host:     targetHost,
		Port:     targetPort,
		Password: "wrong-pass",
		ProxyJump: &Config{
			Host:     bastionHost,
			Port:     bastionPort,
			Password: "bastion-pass",
		},
	})

	err := c.Connect(context.Background())
	if err == nil {
		c.Close()
		t.Fatal("expected error when target auth fails")
	}
}

func TestConnect_ProxyJump(t *testing.T) {
	targetAddr, targetCleanup := startTestSSHServer(t, "target-pass", false)
	defer targetCleanup()

	bastionAddr, bastionCleanup := startTestSSHServer(t, "bastion-pass", true)
	defer bastionCleanup()

	bastionHost, bastionPortStr, _ := net.SplitHostPort(bastionAddr)
	bastionPort := 0
	fmt.Sscanf(bastionPortStr, "%d", &bastionPort)

	targetHost, targetPortStr, _ := net.SplitHostPort(targetAddr)
	targetPort := 0
	fmt.Sscanf(targetPortStr, "%d", &targetPort)

	c := New(Config{
		Host:     targetHost,
		Port:     targetPort,
		Password: "target-pass",
		ProxyJump: &Config{
			Host:     bastionHost,
			Port:     bastionPort,
			Password: "bastion-pass",
		},
	})

	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("Connect via ProxyJump failed: %v", err)
	}
	defer c.Close()

	if c.SSHClient() == nil {
		t.Error("expected target SSH client to be set")
	}
	if c.proxyClient == nil {
		t.Error("expected proxyClient to be set")
	}
	if c.proxyConn == nil {
		t.Error("expected proxyConn to be set")
	}
}
