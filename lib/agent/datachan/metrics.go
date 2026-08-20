package datachan

import (
	"context"
	"encoding/json"
	"net"
	"time"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
)

// metricsWriteTimeout bounds writing one result. The reader asked for
// this data milliseconds ago, so a write that cannot finish promptly
// means the gateway stalled without dying -- the same peer failure
// file.go's idle timeout exists for. Without a bound, Encode blocks on
// stream flow control forever, and the leaked handler holds the stream
// count above zero so the data channel never idle-closes.
const metricsWriteTimeout = 30 * time.Second

// MetricsQuerier answers windowed reads of the host's metric history.
// Declared here rather than importing the collector package, so the
// data channel stays wiring-agnostic the way it is for Guard and Shell.
type MetricsQuerier interface {
	Query(fromMs, toMs int64, stepSec int, names []string) (agenttypes.MetricsResult, error)
}

// serveMetrics answers one StreamKindMetrics request.
//
// Unlike every other kind this is a request/response, not a session: the
// window came in the header, so the whole exchange is accept, one JSON
// body, close. There is no idle handling because there is no idle --
// the stream is over as soon as the result is written.
func (c *Channel) serveMetrics(ctx context.Context, stream net.Conn, open agenttypes.StreamOpen) {
	if c.cfg.Metrics == nil {
		reject(stream, agenttypes.StreamErrOpFailed, "this host does not collect metrics")
		return
	}
	res, err := c.cfg.Metrics.Query(open.FromMs, open.ToMs, open.StepSec, open.Series)
	if err != nil {
		reject(stream, agenttypes.StreamErrOpFailed, err.Error())
		return
	}
	_ = stream.SetWriteDeadline(time.Now().Add(metricsWriteTimeout))
	if err := accept(stream); err != nil {
		return
	}
	if err := json.NewEncoder(stream).Encode(res); err != nil && ctx.Err() == nil {
		c.cfg.Log.Warnf("data channel: write metrics result: %v", err)
	}
}
