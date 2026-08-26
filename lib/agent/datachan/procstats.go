package datachan

import (
	"context"
	"encoding/json"
	"net"
	"time"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
)

// ProcessStatsQuerier answers "what is running here, and what is it
// using". Declared here rather than importing the collector, so the data
// channel stays wiring-agnostic the way it is for Metrics and Shell.
type ProcessStatsQuerier interface {
	// Live samples cpu over a short window, so it BLOCKS for a fraction
	// of a second. That is why it rides the data channel and not the
	// control channel: a control frame handler that sleeps would stall
	// every other frame behind it.
	Live() agenttypes.HostProcesses
}

// serveProcessStats answers one StreamKindProcessStats request.
//
// Request/response like serveMetrics: accept, one JSON body, close. There
// is no window in the header because there is no window -- the answer is
// what is true now, which is the whole reason this is not the gated
// host.processes frame.
func (c *Channel) serveProcessStats(ctx context.Context, stream net.Conn, _ agenttypes.StreamOpen) {
	if c.cfg.ProcessStats == nil {
		reject(stream, agenttypes.StreamErrOpFailed, "this host does not collect process stats")
		return
	}
	// Sampled BEFORE the accept, so a slow read cannot be mistaken for a
	// slow handshake, and so the deadline below covers only the write.
	res := c.cfg.ProcessStats.Live()
	_ = stream.SetWriteDeadline(time.Now().Add(metricsWriteTimeout))
	if err := accept(stream); err != nil {
		return
	}
	if err := json.NewEncoder(stream).Encode(res); err != nil && ctx.Err() == nil {
		c.cfg.Log.Warnf("data channel: write process stats: %v", err)
	}
}
