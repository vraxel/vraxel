package agentgw

import "context"

// RunManager is a stub for the register / control-channel slice.
//
// The real (play, host) job orchestrator -- dispatch, reconnect
// reconciliation, job acks, orphan-run sweep -- lands with the jobs slice
// (it needs lib/tasktracker and the run/job/lock tables). channel.go calls
// OnAgentReconnect / OnJobAck on every control channel, so the type and
// those methods exist now as no-ops; the jobs slice only adds behaviour,
// it changes no call site here.
type RunManager struct{}

// OnAgentReconnect reconciles a host's in-flight jobs against the fresh
// hello. No-op until the jobs slice.
func (m *RunManager) OnAgentReconnect(ctx context.Context, hostID int64, runningJobs []int64, sess *Session) {
}

// OnJobAck flips a dispatched job to running. No-op until the jobs slice.
func (m *RunManager) OnJobAck(ctx context.Context, hostID int64, jobID int64) {}
