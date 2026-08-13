package transport

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeCA writes a server's certificate out as a PEM bundle, which is
// what an operator would hand the agent for a private CA.
func writeCA(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ca.pem")
	der := srv.Certificate().Raw
	body := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatalf("write CA: %v", err)
	}
	return path
}

// TestSelfSignedServerNeedsItsCA is the whole reason this package exists:
// an agent pointed at a deployment with a private CA cannot connect at
// all -- not register, not open a channel -- until it is told to trust
// that CA.
func TestSelfSignedServerNeedsItsCA(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	// Without the CA: refused, and refused for the right reason.
	resp, err := HTTPClient(nil).Get(srv.URL)
	if err == nil {
		resp.Body.Close()
		t.Fatal("the system trust store accepted a self-signed certificate")
	}
	if !strings.Contains(err.Error(), "certificate") {
		t.Fatalf("failed for an unrelated reason: %v", err)
	}

	// With it: connected.
	cfg, err := LoadCA(writeCA(t, srv))
	if err != nil {
		t.Fatalf("LoadCA: %v", err)
	}
	resp, err = HTTPClient(cfg).Get(srv.URL)
	if err != nil {
		t.Fatalf("the CA was loaded but the connection still failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
}

// newTLSServerWithOwnCert starts a TLS server holding a freshly minted
// self-signed certificate. httptest's own TLS servers all present one
// shared built-in certificate, so two of them cannot stand in for two
// different issuers.
func newTLSServerWithOwnCert(t *testing.T) *httptest.Server {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(2),
		Subject:               pkix.Name{CommonName: "other-issuer"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{{
		Certificate: [][]byte{der},
		PrivateKey:  key,
	}}}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv
}

// TestPrivateCADoesNotInheritPublicRoots pins the deliberate choice not
// to merge the system pool: a deployment that switched to a private CA
// did so to stop trusting several hundred commercial ones.
func TestPrivateCADoesNotInheritPublicRoots(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	other := newTLSServerWithOwnCert(t)

	cfg, err := LoadCA(writeCA(t, srv))
	if err != nil {
		t.Fatalf("LoadCA: %v", err)
	}
	if cfg.RootCAs == nil {
		t.Fatal("no root pool was built")
	}
	// A certificate from a different issuer must not pass.
	if resp, err := HTTPClient(cfg).Get(other.URL); err == nil {
		resp.Body.Close()
		t.Fatal("a certificate outside the configured CA was accepted")
	}
}

func TestEmptyPathMeansSystemDefaults(t *testing.T) {
	cfg, err := LoadCA("")
	if err != nil {
		t.Fatalf("LoadCA(\"\"): %v", err)
	}
	if cfg != nil {
		t.Fatalf("cfg = %+v, want nil so the standard trust store applies", cfg)
	}
}

func TestBadCAFileIsRejectedLoudly(t *testing.T) {
	if _, err := LoadCA(filepath.Join(t.TempDir(), "missing.pem")); err == nil {
		t.Fatal("a missing CA file was accepted")
	}

	junk := filepath.Join(t.TempDir(), "junk.pem")
	if err := os.WriteFile(junk, []byte("not a certificate"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	// A file with no certificate in it must not silently produce an empty
	// pool, which would reject every connection with a confusing error.
	if _, err := LoadCA(junk); err == nil {
		t.Fatal("a file containing no certificate was accepted")
	}
}
