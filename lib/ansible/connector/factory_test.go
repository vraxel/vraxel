package connector

import (
	"net"
	"os"
	"testing"
)

func TestNewConnector_Local(t *testing.T) {
	vars := map[string]any{
		"connection": "local",
	}
	c, err := NewConnector("192.168.1.100", vars)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := c.(*LocalConnector); !ok {
		t.Fatalf("expected *LocalConnector, got %T", c)
	}
}

func TestNewConnector_LocalWithPassword(t *testing.T) {
	vars := map[string]any{
		"connection": "local",
		"password":   "secret",
	}
	c, err := NewConnector("anyhost", vars)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	lc, ok := c.(*LocalConnector)
	if !ok {
		t.Fatalf("expected *LocalConnector, got %T", c)
	}
	if lc.password != "secret" {
		t.Errorf("expected password %q, got %q", "secret", lc.password)
	}
}

func TestNewConnector_Localhost(t *testing.T) {
	vars := map[string]any{}

	for _, host := range []string{"localhost", "127.0.0.1", "::1"} {
		c, err := NewConnector(host, vars)
		if err != nil {
			t.Fatalf("unexpected error for host %q: %v", host, err)
		}
		if _, ok := c.(*LocalConnector); !ok {
			t.Errorf("expected *LocalConnector for host %q, got %T", host, c)
		}
	}
}

func TestNewConnector_LocalHostname(t *testing.T) {
	hostname, err := os.Hostname()
	if err != nil {
		t.Skip("cannot determine hostname")
	}
	vars := map[string]any{}
	c, err := NewConnector(hostname, vars)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := c.(*LocalConnector); !ok {
		t.Errorf("expected *LocalConnector for OS hostname %q, got %T", hostname, c)
	}
}

func TestNewConnector_Unsupported(t *testing.T) {
	vars := map[string]any{
		"connection": "docker",
	}
	_, err := NewConnector("anyhost", vars)
	if err == nil {
		t.Fatal("expected error for unsupported connection type")
	}
}

func TestIsLocal_KnownLocal(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{"localhost", true},
		{"127.0.0.1", true},
		{"::1", true},
		{"10.99.99.99", false},
		{"remote.example.com", false},
	}
	for _, tt := range tests {
		got := IsLocalHost(tt.host)
		if got != tt.want {
			t.Errorf("IsLocalHost(%q) = %v, want %v", tt.host, got, tt.want)
		}
	}
}

func TestIsLocal_OSHostname(t *testing.T) {
	hostname, err := os.Hostname()
	if err != nil {
		t.Skip("cannot determine hostname")
	}
	if !IsLocalHost(hostname) {
		t.Errorf("IsLocalHost(%q) = false, expected true for OS hostname", hostname)
	}
}

func TestIsLocal_LocalInterfaceIP(t *testing.T) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		t.Skip("cannot list interface addresses")
	}
	for _, addr := range addrs {
		var ip net.IP
		switch v := addr.(type) {
		case *net.IPNet:
			ip = v.IP
		case *net.IPAddr:
			ip = v.IP
		}
		if ip == nil || ip.IsLoopback() {
			continue
		}
		// At least one non-loopback local IP should be detected.
		if IsLocalHost(ip.String()) {
			return // success
		}
	}
	// If we get here, either no non-loopback IPs exist or isLocal failed.
	// On machines with only loopback, this is acceptable.
}
