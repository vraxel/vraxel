package datachan

import (
	"context"
	"io"
	"net"
	"sync"
	"time"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
)

const (
	// defaultDialTimeout bounds the connect to a local service when the
	// gateway does not specify one.
	defaultDialTimeout = 10 * time.Second
)

// serveTCP is the raw kind: connect to a loopback port and copy bytes
// both ways until either side finishes.
//
// Nothing here parses HTTP. The gateway drives an http.Transport over
// this stream (design §6.6), which is what makes one implementation
// serve middleware REST, k8s client-go, SPDY pod exec, streaming pod
// logs and TCP probes -- and what makes response streaming automatic
// rather than a feature: bytes are forwarded as they arrive because
// there is nowhere to buffer them.
//
// There is no data-activity idle timeout. A quiet stream is not a stuck
// one: an SSE feed or a `kubectl logs -f` on a silent pod legitimately
// moves no bytes for minutes, and the agent cannot tell that apart from a
// hung service. The two ends that matter are covered without guessing --
// the gateway closes the stream when its request ends (an EOF here), and
// yamux keepalive (channel.go, 30s) tears the session down if the gateway
// vanishes. A genuinely hung local service holds one goroutine until the
// viewer gives up and the gateway closes the stream, which is the same
// self-healing every reverse proxy relies on.
func (c *Channel) serveTCP(ctx context.Context, stream net.Conn, open agenttypes.StreamOpen) {
	// The guard hands back a checked IP, and that is what gets dialled.
	// Dialling open.Target instead would resolve the name a second time,
	// which is all a DNS-controlling attacker needs to pass the check
	// with a loopback answer and land the connection somewhere else.
	dialAddr, err := c.cfg.Guard.Resolve(open.Target)
	if err != nil {
		c.cfg.Log.Warnf("data channel: refused target %s: %v", open.Target, err)
		reject(stream, agenttypes.StreamErrTargetNotAllowed, err.Error())
		return
	}

	timeout := defaultDialTimeout
	if open.TimeoutMs > 0 {
		timeout = time.Duration(open.TimeoutMs) * time.Millisecond
	}
	dctx, cancelDial := context.WithTimeout(ctx, timeout)
	defer cancelDial()

	var d net.Dialer
	conn, err := d.DialContext(dctx, "tcp", dialAddr)
	if err != nil {
		reject(stream, agenttypes.StreamErrDialFailed, err.Error())
		return
	}
	defer conn.Close()

	if err := accept(stream); err != nil {
		return
	}

	// Half-close on each direction as it finishes, so a request that ends
	// with "no more bytes from the client" (HTTP/1 without
	// content-length, a piped stdin) reaches the local service as a real
	// EOF instead of hanging until the whole stream is torn down.
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(conn, stream)
		if tcp, ok := conn.(*net.TCPConn); ok {
			_ = tcp.CloseWrite()
		}
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(stream, conn)
		// yamux Close is a FIN: the peer sees EOF, and this side can
		// still drain anything already in flight.
		_ = stream.Close()
	}()

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-ctx.Done():
		// The session died (yamux keepalive) while a copy was still
		// blocked. Force both ends shut so io.Copy unblocks.
		_ = conn.Close()
		_ = stream.Close()
		<-done
	}
}
