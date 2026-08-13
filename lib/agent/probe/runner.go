// Package probe runs the host's probes locally and reports verdicts.
//
// It carries no platform dependencies: the checks are twenty lines of net and
// os/exec rather than a call into lib/probe, which would drag the ansible
// executor in through its health helpers.
package probe

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"sort"
	"strconv"
	"sync"
	"time"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
)

const (
	defaultPeriod           = 10 * time.Second
	defaultTimeout          = 3 * time.Second
	defaultSuccessThreshold = 1
	defaultFailureThreshold = 3
)

// Runner owns the set of probes for this host.
//
// SetSpecs replaces the whole set rather than patching it: the server
// always knows the full list of services it deployed here, and a
// full-replace protocol cannot drift into a state where a deleted
// service's probe keeps running forever.
type Runner struct {
	// OnChange fires whenever a probe flips verdict, carrying only the
	// probes that changed. The full set still rides every heartbeat
	// (design §5.6), so a lost report self-corrects within a beat instead
	// of leaving the server's view permanently wrong.
	OnChange func([]agenttypes.ProbeState)

	mu      sync.Mutex
	workers map[string]*worker
}

// New builds an empty Runner.
func New(onChange func([]agenttypes.ProbeState)) *Runner {
	return &Runner{OnChange: onChange, workers: map[string]*worker{}}
}

// SetSpecs reconciles the running probes to specs: start the new, stop
// the removed, restart the changed, leave the identical alone (so a
// config push does not reset every probe's threshold counters).
func (r *Runner) SetSpecs(ctx context.Context, specs []agenttypes.ProbeSpec) {
	wanted := make(map[string]agenttypes.ProbeSpec, len(specs))
	for _, s := range specs {
		if s.Name != "" {
			wanted[s.Name] = s
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for name, w := range r.workers {
		spec, keep := wanted[name]
		if keep && sameSpec(w.spec, spec) {
			continue
		}
		w.stop()
		delete(r.workers, name)
	}
	for name, spec := range wanted {
		if _, running := r.workers[name]; running {
			continue
		}
		w := newWorker(ctx, spec, r.report)
		r.workers[name] = w
		go w.run()
	}
}

// States reports every probe's current verdict, sorted by name so the
// heartbeat payload is stable.
func (r *Runner) States() []agenttypes.ProbeState {
	r.mu.Lock()
	workers := make([]*worker, 0, len(r.workers))
	for _, w := range r.workers {
		workers = append(workers, w)
	}
	r.mu.Unlock()

	out := make([]agenttypes.ProbeState, 0, len(workers))
	for _, w := range workers {
		out = append(out, w.state())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Close stops every probe.
func (r *Runner) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for name, w := range r.workers {
		w.stop()
		delete(r.workers, name)
	}
}

func (r *Runner) report(s agenttypes.ProbeState) {
	if r.OnChange != nil {
		r.OnChange([]agenttypes.ProbeState{s})
	}
}

func sameSpec(a, b agenttypes.ProbeSpec) bool {
	if a.Name != b.Name || a.Type != b.Type || a.Port != b.Port || a.Path != b.Path ||
		a.InitialDelaySec != b.InitialDelaySec || a.PeriodSec != b.PeriodSec ||
		a.TimeoutSec != b.TimeoutSec || a.SuccessThreshold != b.SuccessThreshold ||
		a.FailureThreshold != b.FailureThreshold || len(a.Command) != len(b.Command) {
		return false
	}
	for i := range a.Command {
		if a.Command[i] != b.Command[i] {
			return false
		}
	}
	return true
}

// worker runs one probe.
type worker struct {
	spec   agenttypes.ProbeSpec
	report func(agenttypes.ProbeState)
	// ctx / cancel are built before the goroutine starts, so a stop that
	// races the start still cancels: deriving them inside run() would let
	// SetSpecs drop a worker it can no longer stop.
	ctx    context.Context
	cancel context.CancelFunc

	mu       sync.Mutex
	cur      agenttypes.ProbeState
	streak   int32
	streakOK bool
}

func newWorker(parent context.Context, spec agenttypes.ProbeSpec, report func(agenttypes.ProbeState)) *worker {
	ctx, cancel := context.WithCancel(parent)
	return &worker{
		spec:   spec,
		report: report,
		ctx:    ctx,
		cancel: cancel,
		// A probe starts unhealthy for the same reason a k8s pod starts
		// not-ready: nothing has been observed yet, and assuming health
		// would report a dead service as alive for one whole period.
		cur: agenttypes.ProbeState{Name: spec.Name, Message: "not yet probed"},
	}
}

func (w *worker) state() agenttypes.ProbeState {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.cur
}

func (w *worker) stop() { w.cancel() }

func (w *worker) run() {
	defer w.cancel()

	if d := time.Duration(w.spec.InitialDelaySec) * time.Second; d > 0 {
		select {
		case <-w.ctx.Done():
			return
		case <-time.After(d):
		}
	}

	ticker := time.NewTicker(w.period())
	defer ticker.Stop()
	for {
		w.check(w.ctx)
		select {
		case <-w.ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (w *worker) period() time.Duration {
	if w.spec.PeriodSec > 0 {
		return time.Duration(w.spec.PeriodSec) * time.Second
	}
	return defaultPeriod
}

func (w *worker) timeout() time.Duration {
	if w.spec.TimeoutSec > 0 {
		return time.Duration(w.spec.TimeoutSec) * time.Second
	}
	return defaultTimeout
}

// check runs the probe once and applies the threshold rules. A verdict
// only flips after enough consecutive results in the same direction,
// which is what keeps one dropped packet from restarting a service.
func (w *worker) check(ctx context.Context) {
	cctx, cancel := context.WithTimeout(ctx, w.timeout())
	err := w.probe(cctx)
	cancel()
	if ctx.Err() != nil {
		return
	}

	ok := err == nil
	msg := ""
	if err != nil {
		msg = err.Error()
	}

	w.mu.Lock()
	if ok == w.streakOK {
		w.streak++
	} else {
		w.streakOK, w.streak = ok, 1
	}
	need := w.failureThreshold()
	if ok {
		need = w.successThreshold()
	}
	flip := ok != w.cur.Healthy && w.streak >= need
	if flip {
		w.cur = agenttypes.ProbeState{
			Name:          w.spec.Name,
			Healthy:       ok,
			Message:       msg,
			ChangedUnixMs: time.Now().UnixMilli(),
		}
	} else if !w.cur.Healthy {
		// Keep the latest reason visible while still failing, without
		// touching ChangedUnixMs.
		w.cur.Message = msg
	}
	state := w.cur
	w.mu.Unlock()

	if flip {
		w.report(state)
	}
}

func (w *worker) successThreshold() int32 {
	if w.spec.SuccessThreshold > 0 {
		return w.spec.SuccessThreshold
	}
	return defaultSuccessThreshold
}

func (w *worker) failureThreshold() int32 {
	if w.spec.FailureThreshold > 0 {
		return w.spec.FailureThreshold
	}
	return defaultFailureThreshold
}

// probe performs one check. Every network probe targets loopback: the
// agent runs on the machine being probed.
func (w *worker) probe(ctx context.Context) error {
	switch w.spec.Type {
	case agenttypes.ProbeTypeTCP:
		var d net.Dialer
		conn, err := d.DialContext(ctx, "tcp", loopbackAddr(w.spec.Port))
		if err != nil {
			return err
		}
		return conn.Close()

	case agenttypes.ProbeTypeHTTP:
		path := w.spec.Path
		if path == "" {
			path = "/"
		}
		if path[0] != '/' {
			path = "/" + path
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet,
			"http://"+loopbackAddr(w.spec.Port)+path, nil)
		if err != nil {
			return err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 400 {
			return fmt.Errorf("http status %d", resp.StatusCode)
		}
		return nil

	case agenttypes.ProbeTypeExec:
		if len(w.spec.Command) == 0 {
			return fmt.Errorf("exec probe has no command")
		}
		cmd := exec.CommandContext(ctx, w.spec.Command[0], w.spec.Command[1:]...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("%v: %s", err, trim(string(out)))
		}
		return nil

	default:
		return fmt.Errorf("unknown probe type %q", w.spec.Type)
	}
}

func loopbackAddr(port int32) string {
	return net.JoinHostPort("127.0.0.1", strconv.Itoa(int(port)))
}

// trim caps a probe failure message: it travels in every heartbeat, and
// a command that dumps a stack trace must not blow the frame ceiling.
func trim(s string) string {
	const max = 256
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
