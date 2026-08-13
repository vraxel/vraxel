package sshclient

import (
	"bytes"
	"strings"
	"testing"
)

func TestPutFile_NotConnected(t *testing.T) {
	c := New(Config{Host: "example.com"})

	err := c.PutFile([]byte("hello"), "/tmp/test.txt", 0644)
	if err == nil {
		t.Fatal("expected error when client is not connected")
	}
	if !strings.Contains(err.Error(), "not connected") {
		t.Errorf("expected 'not connected' in error, got: %v", err)
	}
}

func TestFetchFile_NotConnected(t *testing.T) {
	c := New(Config{Host: "example.com"})

	var buf bytes.Buffer
	err := c.FetchFile("/tmp/test.txt", &buf)
	if err == nil {
		t.Fatal("expected error when client is not connected")
	}
	if !strings.Contains(err.Error(), "not connected") {
		t.Errorf("expected 'not connected' in error, got: %v", err)
	}
}
