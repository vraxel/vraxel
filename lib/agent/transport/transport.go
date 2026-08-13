// Package transport builds the TLS configuration and HTTP client every
// outbound call from the agent shares.
//
// It exists because the agent talks to its server over six code paths
// (register, control channel, data channel, REST, binary download,
// metrics push) and all six must trust the same certificate authority.
// Six independently constructed clients is six chances to forget one,
// and forgetting one produces a failure that only appears in the
// deployment that needed it.
package transport

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"time"
)

// LoadCA reads a PEM bundle and returns a TLS config that trusts it in
// addition to nothing else.
//
// The system pool is deliberately NOT included. A private CA is used
// precisely because the deployment is not part of the public trust
// chain, and keeping the public roots alongside it would mean any of
// several hundred commercial CAs could still mint a certificate this
// agent accepts -- which is the property the private CA was chosen to
// avoid. An empty path returns nil, meaning "use the system defaults",
// which is correct for a deployment with a publicly trusted certificate.
func LoadCA(path string) (*tls.Config, error) {
	if path == "" {
		return nil, nil
	}
	pem, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read CA bundle: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("CA bundle %s contains no certificate", path)
	}
	return &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}, nil
}

// HTTPClient builds a client that trusts cfg. A nil cfg yields a client
// with the standard trust store, so callers never branch on it.
//
// Timeouts are left to the caller's context on purpose: the agent's
// requests range from a sub-second register to a ten-minute binary
// download, and a client-wide timeout would have to be sized for the
// longest, making it useless for the rest.
func HTTPClient(cfg *tls.Config) *http.Client {
	return &http.Client{Transport: NewTransport(cfg)}
}

// NewTransport builds the shared transport. Exposed for the callers that
// need to adjust it further (the metrics scraper disables compression so
// it can forward an exporter's gzip untouched).
func NewTransport(cfg *tls.Config) *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.TLSClientConfig = cfg
	t.IdleConnTimeout = 90 * time.Second
	return t
}
