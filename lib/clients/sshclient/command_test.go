package sshclient

import (
	"bytes"
	"context"
	"testing"
)

func TestExecResult(t *testing.T) {
	r := &ExecResult{
		Stdout: []byte("hello\n"),
		Stderr: []byte("warning\n"),
	}

	if string(r.Stdout) != "hello\n" {
		t.Errorf("expected stdout %q, got %q", "hello\n", string(r.Stdout))
	}
	if string(r.Stderr) != "warning\n" {
		t.Errorf("expected stderr %q, got %q", "warning\n", string(r.Stderr))
	}
}

func TestExecResult_Empty(t *testing.T) {
	r := &ExecResult{}

	if r.Stdout != nil {
		t.Errorf("expected nil stdout, got %v", r.Stdout)
	}
	if r.Stderr != nil {
		t.Errorf("expected nil stderr, got %v", r.Stderr)
	}
}

func TestExec_NotConnected(t *testing.T) {
	c := New(Config{Host: "example.com"})

	result, err := c.Exec(context.Background(), "echo hello")
	if err == nil {
		t.Fatal("expected error when client is not connected")
	}
	if result != nil {
		t.Errorf("expected nil result when not connected, got %+v", result)
	}
	if err.Error() != "sshclient: not connected" {
		t.Errorf("expected 'sshclient: not connected' error, got %q", err.Error())
	}
}

func TestExecWithSudo_NotConnected(t *testing.T) {
	c := New(Config{Host: "example.com"})

	result, err := c.ExecWithSudo(context.Background(), "whoami", "password")
	if err == nil {
		t.Fatal("expected error when client is not connected")
	}
	if result != nil {
		t.Errorf("expected nil result when not connected, got %+v", result)
	}
	if err.Error() != "sshclient: not connected" {
		t.Errorf("expected 'sshclient: not connected' error, got %q", err.Error())
	}
}

func TestExecWithSudo_NotConnected_NoPassword(t *testing.T) {
	c := New(Config{Host: "example.com"})

	result, err := c.ExecWithSudo(context.Background(), "whoami", "")
	if err == nil {
		t.Fatal("expected error when client is not connected")
	}
	if result != nil {
		t.Errorf("expected nil result when not connected, got %+v", result)
	}
	if err.Error() != "sshclient: not connected" {
		t.Errorf("expected 'sshclient: not connected' error, got %q", err.Error())
	}
}

func TestExecStream_NotConnected(t *testing.T) {
	c := New(Config{Host: "example.com"})

	var stdout, stderr bytes.Buffer
	err := c.ExecStream(context.Background(), "echo hello", &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error when client is not connected")
	}
	if err.Error() != "sshclient: not connected" {
		t.Errorf("expected 'sshclient: not connected' error, got %q", err.Error())
	}
}

func TestNewSession_NotConnected(t *testing.T) {
	c := New(Config{Host: "example.com"})

	session, err := c.newSession()
	if err == nil {
		t.Fatal("expected error when client is not connected")
	}
	if session != nil {
		t.Error("expected nil session when not connected")
	}
}
