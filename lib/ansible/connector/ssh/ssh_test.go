package ssh

import (
	"testing"

	"vraxel.io/vraxel/lib/clients/sshclient"
)

// ---------------------------------------------------------------------------
// NewConnector factory tests
// ---------------------------------------------------------------------------

func TestNewConnector_SSH(t *testing.T) {
	vars := map[string]any{
		"connection":  "ssh",
		"password":    "pw",
		"port":        2222,
		"remote_user": "deploy",
		"become":      true,
		"become_user": "admin",
	}
	c, err := NewConnector("10.0.0.5", vars)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sc, ok := c.(*SSHConnector)
	if !ok {
		t.Fatalf("expected *SSHConnector, got %T", c)
	}
	if sc.host != "10.0.0.5" {
		t.Errorf("expected host %q, got %q", "10.0.0.5", sc.host)
	}
	if sc.password != "pw" {
		t.Errorf("expected password %q, got %q", "pw", sc.password)
	}
	if !sc.become {
		t.Error("expected become=true")
	}
	if sc.becomeUser != "admin" {
		t.Errorf("expected becomeUser %q, got %q", "admin", sc.becomeUser)
	}
}

func TestNewConnector_SSHDefaultType(t *testing.T) {
	// Empty connection type with a remote host should produce an SSHConnector.
	vars := map[string]any{
		"password": "pw",
	}
	c, err := NewConnector("10.0.0.5", vars)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := c.(*SSHConnector); !ok {
		t.Fatalf("expected *SSHConnector for remote host, got %T", c)
	}
}

func TestNewConnector_SSHWithKeyParams(t *testing.T) {
	vars := map[string]any{
		"private_key":         "/path/to/key",
		"private_key_content": "PEM-CONTENT",
	}
	c, err := NewConnector("10.0.0.5", vars)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := c.(*SSHConnector); !ok {
		t.Fatalf("expected *SSHConnector, got %T", c)
	}
}

// ---------------------------------------------------------------------------
// SSHConnector unit tests (no real SSH connection)
// ---------------------------------------------------------------------------

func TestSSHConnector_NotConnected(t *testing.T) {
	sc := NewSSHConnector(sshclient.Config{
		Host: "10.0.0.1",
	}, false, "")

	// ExecuteCommand should fail because we never called Init.
	_, _, err := sc.ExecuteCommand(t.Context(), "echo hello")
	if err == nil {
		t.Error("expected error from ExecuteCommand without Init")
	}

	// PutFile should fail.
	err = sc.PutFile(t.Context(), []byte("data"), "/tmp/test", 0644)
	if err == nil {
		t.Error("expected error from PutFile without Init")
	}

	// FetchFile should fail.
	var buf []byte
	err = sc.FetchFile(t.Context(), "/tmp/test", nil)
	if err == nil {
		t.Error("expected error from FetchFile without Init")
	}
	_ = buf
}

func TestSSHConnector_CloseWithoutInit(t *testing.T) {
	sc := NewSSHConnector(sshclient.Config{
		Host: "10.0.0.1",
	}, false, "")

	// Close on an uninitialized connector should not panic or error.
	if err := sc.Close(t.Context()); err != nil {
		t.Errorf("Close without Init should not error, got: %v", err)
	}
}

func TestSSHConnector_Fields(t *testing.T) {
	sc := NewSSHConnector(sshclient.Config{
		Host:     "10.0.0.5",
		Port:     2222,
		User:     "deploy",
		Password: "secret",
	}, true, "admin")

	if sc.host != "10.0.0.5" {
		t.Errorf("expected host %q, got %q", "10.0.0.5", sc.host)
	}
	if sc.password != "secret" {
		t.Errorf("expected password %q, got %q", "secret", sc.password)
	}
	if !sc.become {
		t.Error("expected become=true")
	}
	if sc.becomeUser != "admin" {
		t.Errorf("expected becomeUser %q, got %q", "admin", sc.becomeUser)
	}
}

func TestNormalizePtyNewlines(t *testing.T) {
	// A PTY rewrites every \n as \r\n; the returned bytes must read as if it
	// had not, or every string comparison against a registered stdout fails.
	cases := map[string]string{
		"hello\r\nworld\r\n": "hello\nworld\n",
		"plain\n":            "plain\n",
		"":                   "",
		// A bare \r redraws a line and is part of the output, not framing.
		"progress\rdone\r\n": "progress\rdone\n",
	}
	for in, want := range cases {
		if got := string(normalizePtyNewlines([]byte(in))); got != want {
			t.Errorf("normalizePtyNewlines(%q) = %q, want %q", in, got, want)
		}
	}
}
