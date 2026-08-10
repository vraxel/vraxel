package httpserver

import (
	"crypto/tls"
	"sync"

	"vraxel.io/vraxel/lib/fasttime"
)

// GetServerTLSConfig returns TLS config for the server
func GetServerTLSConfig(tlsCertFile, tlsKeyFile string) (*tls.Config, error) {
	cfg := &tls.Config{}
	cfg.GetCertificate = newGetCertificateFunc(tlsCertFile, tlsKeyFile)
	return cfg, nil
}

func newGetCertificateFunc(tlsCertFile, tlsKeyFile string) func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	var certLock sync.Mutex
	var certDeadline uint64
	var cert *tls.Certificate
	return func(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
		certLock.Lock()
		defer certLock.Unlock()
		if fasttime.UnixTimestamp() > certDeadline {
			c, err := tls.LoadX509KeyPair(tlsCertFile, tlsKeyFile)
			if err != nil {
				return nil, err
			}
			certDeadline = fasttime.UnixTimestamp() + 60
			cert = &c
		}
		return cert, nil
	}
}
