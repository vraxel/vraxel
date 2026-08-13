package types

// Probe types, mirroring pkg/apis/app.K8sProbe field for field so the
// server maps a deployed service's probe spec onto the wire without a
// translation table.
const (
	ProbeTypeTCP  = "tcp"
	ProbeTypeHTTP = "http"
	ProbeTypeExec = "exec"
)

// ProbeSpec is one probe the agent runs locally and periodically.
//
// There is no host field on purpose: the agent probes its own machine,
// so the target is always loopback. That is the whole point of moving
// probes to the agent (design §5.6) -- a probe is a periodic local check,
// and running it from the control plane cost a cross-network round trip
// per service per period and did not work at all once the host stopped
// being dialable.
type ProbeSpec struct {
	// Name identifies the probe across config pushes and reports. The
	// server owns the format; the agent only needs it to be stable.
	Name string `json:"name"`
	// Type is one of the ProbeType* constants.
	Type string `json:"type"`
	// Port is the loopback port for tcp and http probes.
	Port int32 `json:"port,omitempty"`
	// Path is the request path for http probes.
	Path string `json:"path,omitempty"`
	// Command is argv for exec probes.
	Command []string `json:"command,omitempty"`

	InitialDelaySec  int32 `json:"initialDelaySec,omitempty"`
	PeriodSec        int32 `json:"periodSec,omitempty"`
	TimeoutSec       int32 `json:"timeoutSec,omitempty"`
	SuccessThreshold int32 `json:"successThreshold,omitempty"`
	FailureThreshold int32 `json:"failureThreshold,omitempty"`
}

// ProbeState is one probe's current verdict.
type ProbeState struct {
	Name    string `json:"name"`
	Healthy bool   `json:"healthy"`
	// Message is the last failure reason; empty while healthy.
	Message string `json:"message,omitempty"`
	// ChangedUnixMs is when Healthy last flipped, so the server can age
	// a state without keeping its own history.
	ChangedUnixMs int64 `json:"changedUnixMs,omitempty"`
}
