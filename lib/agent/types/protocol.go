// Package types is the vr-agent wire contract: the control-channel
// frames and the /register request/response bodies, shared verbatim by
// the server gateway (pkg/apis/agentgw) and the agent binary
// (app/vr-agent).
//
// It has zero dependencies, and that is the point. The agent runs on
// customer machines; if it linked the gateway package it would compile
// against vraxel's database layer, so every store change would rebuild the
// agent.
package types

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// ProtocolPathPrefix is the URL prefix every agent endpoint hangs off.
// It lives in the wire contract so the server gateway and the agent
// client both reference one constant without either importing the other
// (the whole reason this package exists).
const ProtocolPathPrefix = "/api/agent/v1/"

// Control-channel frame types. Step 4 implements hello / heartbeat; the
// remaining constants pin the wire vocabulary that Step 5 (jobs) and
// Step 7 (data channel) fill in, so agents built now recognize the names
// without a protocol revision.
//
// There is deliberately NO application-level ping/pong: WebSocket-layer
// pings (lib/websocket keepalive, both directions) already prove the
// transport, and the heartbeat frame already proves the agent's frame
// loop -- an app ping would be a third probe with no distinct failure
// mode to detect.
const (
	// server -> agent
	FrameTypeJobDispatch  = "job.dispatch"
	FrameTypeJobCancel    = "job.cancel"
	FrameTypeChannelOpen  = "channel.open"
	FrameTypeConfigReload = "config.reload"
	// FrameTypeSessionToken delivers the short-lived session token the
	// agent must use on every REST endpoint (design §4.1 three-tier
	// credential model). Reserved here so the wire vocabulary is fixed;
	// issuing is Step 5 work, since Step 4's three endpoints
	// (register / channel / binary) need no session token.
	FrameTypeSessionToken = "session.token"
	// FrameTypeProbeConfig replaces the host's whole probe set
	// (design §5.6).
	FrameTypeProbeConfig = "probe.config"
	// FrameTypeAgentUpgrade tells the agent to replace its own binary.
	FrameTypeAgentUpgrade = "agent.upgrade"

	// agent -> server
	FrameTypeHello     = "hello"
	FrameTypeHeartbeat = "heartbeat"
	FrameTypeJobAck    = "job.ack"
	FrameTypeError     = "error"
	// FrameTypeProbeReport carries probe verdicts that just flipped. The
	// full set also rides every heartbeat, so a lost report self-corrects.
	FrameTypeProbeReport = "probe.report"
	// FrameTypeAgentStatus reports a lifecycle state the server cannot
	// infer, currently only pending_restart.
	FrameTypeAgentStatus = "agent.status"
)

// Agent lifecycle statuses for FrameTypeAgentStatus.
const (
	// AgentStatusPendingRestart means a new binary is staged and the
	// agent is waiting for its jobs to finish before restarting into it.
	AgentStatusPendingRestart = "pending_restart"
)

// MaxFrameBytes is the hard per-frame ceiling on the control channel
// (design §4.2). The control channel carries instructions only: anything
// bulky (playbook bundles, job vars, task events) moves over its own HTTP
// request, so 64 KiB is a generous ceiling rather than a constraint. Both
// ends enforce it; a larger frame is a protocol error, not a fragment.
const MaxFrameBytes = 64 * 1024

// Frame is the single JSON envelope used in both directions. One struct
// rather than a type-per-message union: every field is optional, the
// decoder switches on Type, and the agent stays a few hundred lines.
type Frame struct {
	// Type is one of the FrameType* constants.
	Type string `json:"type"`
	// ID is the sender-assigned frame id; echoed back in Ref by any
	// frame that answers this one.
	ID string `json:"id,omitempty"`
	// Ref points at the ID of the frame being answered (error / pong).
	Ref string `json:"ref,omitempty"`

	// --- hello ---
	AgentVersion string `json:"agentVersion,omitempty"`
	// RunningJobs lets a reconnecting agent tell the server which jobs
	// it is still executing so they are not dispatched twice (Step 5).
	RunningJobs []int64 `json:"runningJobs,omitempty"`

	// --- hello + heartbeat ---
	// ClockUnixMs is the agent's wall clock at send time. The server
	// diffs it against its own to maintain host_agents.clock_skew_ms;
	// a drifting host clock corrupts metric timestamps.
	//
	// The heartbeat carries no utilisation payload on purpose: liveness
	// is the frame arriving, and every host utilisation series comes from
	// node_exporter (scraped in Step 11), so an agent-side /proc sample
	// would have no consumer and would risk colliding with node_* names.
	ClockUnixMs int64 `json:"clockUnixMs,omitempty"`

	// --- job.dispatch ---
	// Job carries the work order. Bulky material (bundle, vars) moves over
	// its own HTTP request; the frame holds references only (design §4.4.1).
	Job *JobDispatch `json:"job,omitempty"`

	// --- job.ack (agent->server) / job.cancel (server->agent) ---
	JobID int64 `json:"jobId,omitempty"`

	// --- session.token ---
	// Token is the short-lived session credential for every REST endpoint
	// (design §4.1). Opaque to the agent: expiry and identity live inside,
	// signed server-side, so validation works on any vraxel-server instance.
	Token string `json:"token,omitempty"`

	// --- probe.config (server->agent) / probe.report + heartbeat (agent->server) ---
	// Probes is the full spec set on a config frame. ProbeStates carries
	// the flipped verdicts on a report, and the full set on a heartbeat.
	Probes      []ProbeSpec  `json:"probes,omitempty"`
	ProbeStates []ProbeState `json:"probeStates,omitempty"`

	// --- agent.upgrade ---
	Upgrade *AgentUpgrade `json:"upgrade,omitempty"`

	// --- agent.status ---
	Status string `json:"status,omitempty"`

	// --- config.reload ---
	// AllowedPorts narrows the data channel's port allowlist to the
	// service ports vraxel knows it deployed on this host. Empty means any
	// loopback port; the loopback rule itself is not configurable.
	AllowedPorts []int `json:"allowedPorts,omitempty"`

	// --- error ---
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

// AgentUpgrade is the agent.upgrade payload.
//
// It names a version and its digest, not a URL: the agent builds the
// download URL from its own server address and BinaryPath, so a frame
// cannot point the agent's self-replacement at an attacker's host.
type AgentUpgrade struct {
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
}

// JobDispatch is the job.dispatch payload: one (play, host) execution
// unit (design §4.4.3). The server has already resolved play ordering,
// host-group patterns and serial batching; the agent's only planning
// duty is to rewrite the play's hosts pattern to HostName and run it
// against itself with connection: local.
type JobDispatch struct {
	ID   int64  `json:"id"`
	Kind string `json:"kind"`
	// Entry is the playbook path inside the bundle, e.g.
	// "node_exporter/install.yml".
	Entry      string `json:"entry"`
	BundleHash string `json:"bundleHash"`
	BundleURL  string `json:"bundleUrl"`
	VarsURL    string `json:"varsUrl"`
	// HostName is this job's target host name exactly as it appears in
	// the server-side inventory. The agent substitutes it for the play's
	// hosts pattern, so group membership never crosses the wire.
	HostName  string `json:"hostName"`
	PlayIndex int    `json:"playIndex"`
	// IsRunOnceTarget marks the batch leader that executes run_once
	// blocks. Carried since Step 5 so the wire needs no revision; the
	// executor-side skip lands with the first multi-host scenario
	// (Step 6).
	IsRunOnceTarget bool `json:"isRunOnceTarget"`
	TimeoutSec      int  `json:"timeoutSec"`
}

// Job result statuses (design §4.4.4).
const (
	JobResultSucceeded = "succeeded"
	JobResultFailed    = "failed"
	JobResultTimeout   = "timeout"
)

// JobEventsRequest is the POST jobs/{id}/events body. Seq numbers the
// batch monotonically for server-side dedup of retried uploads and MUST
// be 1-based (the server rejects seq < 1: a 0 would collide with the
// server's zero default and both drop the batch and never close the vars
// re-fetch window). Events elements are executor.Event JSON, produced and
// consumed via lib/ansible/executor on both ends; they stay raw here so
// this package keeps its zero-dependency guarantee.
type JobEventsRequest struct {
	Seq    int64             `json:"seq"`
	Events []json.RawMessage `json:"events"`
}

// JobResultRequest is the POST jobs/{id}/result body.
type JobResultRequest struct {
	Status           string `json:"status"`
	Message          string `json:"message,omitempty"`
	FinishedAtUnixMs int64  `json:"finishedAt,omitempty"`
	EventsTruncated  bool   `json:"eventsTruncated,omitempty"`
}

// Endpoint path builders, shared by the server router and the agent
// client so the URL shapes cannot drift (same reason ProtocolPathPrefix
// lives here).

// TokenRenewPath is where the agent exchanges a live session token for a
// fresh agentToken. The session token is the credential on purpose: it
// proves a live control channel, so a leaked agentToken on its own
// cannot be used to extend itself indefinitely.
const TokenRenewPath = ProtocolPathPrefix + "token:renew"

// TokenRenewResponse is the token:renew answer.
type TokenRenewResponse struct {
	AgentToken string `json:"agentToken"`
}

// BinaryPath returns the download path for an agent binary. The agent
// builds its upgrade URL from this rather than from a URL in the
// upgrade frame: the binary it replaces itself with must come from the
// server it is enrolled with, never from an address a frame names.
func BinaryPath(goos, goarch string) string {
	return ProtocolPathPrefix + "binary/" + goos + "/" + goarch
}

// BundlePath returns the download path for a content-addressed bundle.
func BundlePath(hash string) string { return ProtocolPathPrefix + "bundles/" + hash }

// JobVarsPath returns the one-shot vars fetch path for a job.
func JobVarsPath(jobID int64) string {
	return ProtocolPathPrefix + "jobs/" + strconv.FormatInt(jobID, 10) + "/vars"
}

// JobEventsPath returns the event-batch upload path for a job.
func JobEventsPath(jobID int64) string {
	return ProtocolPathPrefix + "jobs/" + strconv.FormatInt(jobID, 10) + "/events"
}

// JobResultPath returns the terminal-status upload path for a job.
func JobResultPath(jobID int64) string {
	return ProtocolPathPrefix + "jobs/" + strconv.FormatInt(jobID, 10) + "/result"
}

// EncodeFrame marshals a frame and enforces the per-frame ceiling on the
// sending side, so an over-large frame surfaces as a caller bug rather
// than as a peer connection reset. Both ends of the channel share it, so
// the 64 KiB invariant lives in exactly one place.
func EncodeFrame(f Frame) ([]byte, error) {
	data, err := json.Marshal(f)
	if err != nil {
		return nil, fmt.Errorf("marshal %s frame: %w", f.Type, err)
	}
	if len(data) > MaxFrameBytes {
		return nil, fmt.Errorf("%s frame is %d bytes, exceeds %d limit", f.Type, len(data), MaxFrameBytes)
	}
	return data, nil
}

// DecodeFrame unmarshals a received frame. The read-limit on the socket
// (set to MaxFrameBytes) already caps the input, so this only reports a
// malformed body.
func DecodeFrame(data []byte) (Frame, error) {
	var f Frame
	if err := json.Unmarshal(data, &f); err != nil {
		return Frame{}, fmt.Errorf("decode frame: %w", err)
	}
	return f, nil
}

// RegisterRequest is the POST /api/agent/v1/register body. Authorization
// carries the join token as a bearer credential.
// Every field here has a server-side consumer; facts the server does not
// persist (e.g. the default-route interface NAME) are deliberately not
// sent.
type RegisterRequest struct {
	// MachineID is a stable per-machine identifier (/etc/machine-id on
	// Linux). The server derives agent_id from it, which is what makes
	// re-running install-agent.sh idempotent instead of creating a
	// second host row.
	MachineID string `json:"machineId"`
	Hostname  string `json:"hostname"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	CPUCores  int32  `json:"cpuCores"`
	// MemoryMB / DiskGB are BINARY units (MiB / GiB), named MB / GB to
	// match hosts.memory_mb / disk_gb and the OS-tool convention. See
	// hostinfo.Static for the rationale. Display-only inventory.
	MemoryMB int64 `json:"memoryMb"`
	DiskGB   int64 `json:"diskGb"`
	// DefaultRouteIP is the IPv4 on the default-route interface. It
	// becomes hosts.reported_primary_ip: used for UI display and for
	// PaaS cluster peer addresses, never for vraxel-initiated dials
	// (design §5.13).
	DefaultRouteIP string `json:"defaultRouteIp"`
	AgentVersion   string `json:"agentVersion,omitempty"`
}

// RegisterResponse is what the agent persists to disk after a successful
// join-token exchange. Exactly the three values the agent consumes --
// earlier drafts also echoed the host name, the channel path and a
// heartbeat interval, all of which the agent never read (the path and
// cadence are protocol constants both sides already share).
type RegisterResponse struct {
	AgentID    string `json:"agentId"`
	HostID     int64  `json:"hostId"`
	AgentToken string `json:"agentToken"`
}
