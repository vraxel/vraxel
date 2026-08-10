package httpserver

import (
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"net"
	"time"

	"github.com/VictoriaMetrics/metrics"
	"vraxel.io/vraxel/lib/logger"
)

var enableTCP6 = flag.Bool("enableTCP6", false, "Whether to enable IPv6 for listening and dialing. By default, only IPv4 TCP and UDP are used")

func NewTCPListener(name, addr string, useProxyProtocol bool, tlsConfig *tls.Config) (net.Listener, error) {
	network := GetTCPNetwork()
	ln, err := net.Listen(network, addr)
	if err != nil {
		return nil, err
	}

	ms := metrics.GetDefaultSet()
	tln := &TCPListener{
		Listener:         ln,
		tlsConfig:        tlsConfig,
		useProxyProtocol: useProxyProtocol,

		accepts:      ms.NewCounter(fmt.Sprintf(`vraxel_tcp_listeners_accepts_total{name=%q, addr=%q}`, name, addr)),
		acceptErrors: ms.NewCounter(fmt.Sprintf(`vraxel_tcp_listeners_errors_total{name=%q, addr=%q, type="accept"}`, name, addr)),
	}
	tln.cm.init(ms, "vraxel_tcp_listener", name, addr)
	return tln, err
}

// TCPListener listens for the addr passed to NewTCPListener
type TCPListener struct {
	net.Listener

	tlsConfig *tls.Config

	accepts      *metrics.Counter
	acceptErrors *metrics.Counter

	useProxyProtocol bool

	cm connMetrics
}

// Accept accepts connections from the addr passed to NewTCPListener
func (ln *TCPListener) Accept() (net.Conn, error) {
	for {
		conn, err := ln.Listener.Accept()
		ln.accepts.Inc()
		if err != nil {
			if ne, ok := errors.AsType[net.Error](err); ok && ne.Temporary() {
				logger.Errorf("temporary error when listening for TCP addr %q: %s", ln.Addr(), err)
				time.Sleep(time.Second)
				continue
			}
			ln.acceptErrors.Inc()
			return nil, err
		}

		if ln.useProxyProtocol {
			pConn := newProxyProtocolConn(conn)
			conn = pConn
		}

		ln.cm.conns.Inc()
		sc := &statConn{
			Conn: conn,
			cm:   &ln.cm,
		}
		if ln.tlsConfig == nil {
			return sc, nil
		}

		return tls.Server(sc, ln.tlsConfig), nil
	}
}

// EnableIPv6 enables IPv6 for dialing and listening
func EnableIPv6() {
	*enableTCP6 = true
}

// TCP6Enabled returns true if dialing and listening for IPv4 TCP is enabled
func TCP6Enabled() bool {
	return *enableTCP6
}

// GetUDPNetwork returns current udp network
func GetUDPNetwork() string {
	if *enableTCP6 {
		// Enable both udp4 and udp6
		return "udp"
	}
	return "udp4"
}

// GetTCPNetwork returns current tcp network
func GetTCPNetwork() string {
	if *enableTCP6 {
		// Enable both tcp4 and tcp6
		return "tcp"
	}
	return "tcp4"
}
