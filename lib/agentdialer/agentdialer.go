// Package agentdialer turns a host id into a net.Conn / http.Client / raw
// stream that reaches that host's loopback services through its reverse
// data channel.
//
// It defines the injection seam only. The SessionOpener that actually
// opens a yamux stream on a live data channel lives in pkg/apis/agentgw,
// which lib/ must not import (lint-layers). The assembly layer injects the
// concrete opener, so any module can dial an agent host without importing
// the gateway -- the same reason lib/agent/types holds the wire contract.
package agentdialer

import (
	"context"
	"net"
	"net/http"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
)

// SessionOpener opens one logical stream to a host's data channel. The
// gateway implements it; the assembly layer injects the implementation.
//
// OpenStream ensures the host's data channel is up (triggering it over the
// control channel if needed), opens one yamux stream, completes the
// StreamOpen/StreamAccept handshake, and returns the stream ready for
// payload. The caller owns the conn and must Close it.
type SessionOpener interface {
	OpenStream(ctx context.Context, hostID int64, open agenttypes.StreamOpen) (net.Conn, error)
}

// Dialer wraps a SessionOpener with the call shapes upper layers need. One
// is built at assembly time from the gateway's opener and shared.
type Dialer struct {
	opener SessionOpener
}

// New builds a Dialer over an opener.
func New(opener SessionOpener) *Dialer { return &Dialer{opener: opener} }

// DialFor returns a dial function bound to one host, shaped for
// rest.Config.Dial and http.Transport.DialContext. addr is
// host:port and becomes the stream target; the agent's guard enforces the
// loopback rule regardless of what network the caller names, so network is
// not forwarded.
func (d *Dialer) DialFor(hostID int64) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		return d.opener.OpenStream(ctx, hostID, agenttypes.StreamOpen{
			Kind:   agenttypes.StreamKindTCP,
			Target: addr,
		})
	}
}

// HTTPClientFor returns an http.Client whose every connection tunnels to
// the host's loopback through the data channel -- the one-line swap the
// C-class middleware and k8s-REST modules make.
//
// No client-wide Timeout: it would abort streaming bodies (SSE,
// `kubectl logs -f`, large list responses), which must stay streaming. Callers bound individual requests with the request context.
func (d *Dialer) HTTPClientFor(hostID int64) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: d.DialFor(hostID),
		},
	}
}

// StreamFor opens a raw stream of an explicit kind (pty / exec / file, or a
// tcp stream carrying SPDY), returning it after the handshake. The web
// terminal, log tail, file browser and SPDY passthrough drive their own
// framing on top.
func (d *Dialer) StreamFor(ctx context.Context, hostID int64, open agenttypes.StreamOpen) (net.Conn, error) {
	return d.opener.OpenStream(ctx, hostID, open)
}
