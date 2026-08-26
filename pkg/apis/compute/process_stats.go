package compute

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
)

// errDataPlaneUnwired is a server without the agent dialer. Not a
// StatusError: every failure on this path is swallowed by the caller and
// turned into "no live numbers", because the stored inventory is still a
// complete answer to what the tab asks.
var errDataPlaneUnwired = errors.New("the agent data plane is not wired on this server")

const (
	// processStatsTimeout bounds the whole live read. The agent holds a
	// one-second window open inside it to measure cpu, so this is that
	// plus the round trip plus room for a busy host, and short enough that
	// a stuck agent does not hold a page open.
	processStatsTimeout = 10 * time.Second
	// processStatsMaxBody caps the answer. A capped inventory is 128
	// groups; this is far above any honest one and far below anything
	// that would matter to hold in memory.
	processStatsMaxBody = 4 << 20
)

// ProcessStatsBackend answers "what is running right now, and what is it
// using". Separate from MetricsBackend because there is only one source:
// the host itself. Utilisation per workload is not in any time-series
// store, and it is not worth putting there -- see the stream kind's
// comment for why this is measured on demand rather than collected.
type ProcessStatsBackend interface {
	Live(ctx context.Context, hostID int64) (*agenttypes.HostProcesses, error)
}

type agentProcessStats struct {
	dialer *AgentDialerHolder
}

// NewAgentProcessStats reads a host's live workload over its data
// channel. Nothing is transmitted or stored while nobody is looking.
func NewAgentProcessStats(dialer *AgentDialerHolder) ProcessStatsBackend {
	return agentProcessStats{dialer: dialer}
}

func (b agentProcessStats) Live(ctx context.Context, hostID int64) (*agenttypes.HostProcesses, error) {
	d := b.dialer.Get()
	if d == nil {
		return nil, errDataPlaneUnwired
	}
	dctx, cancel := context.WithTimeout(ctx, processStatsTimeout)
	defer cancel()

	stream, err := d.StreamFor(dctx, hostID, agenttypes.StreamOpen{Kind: agenttypes.StreamKindProcessStats})
	if err != nil {
		return nil, err
	}
	defer stream.Close()

	// The context deadline does not reach into a blocked net.Conn read;
	// the stream's own deadline is what does.
	_ = stream.SetReadDeadline(time.Now().Add(processStatsTimeout))
	var res agenttypes.HostProcesses
	if err := json.NewDecoder(io.LimitReader(stream, processStatsMaxBody)).Decode(&res); err != nil {
		return nil, err
	}
	return &res, nil
}
