package executor

import "time"

// EventType identifies the kind of executor event.
type EventType string

const (
	EventPlayStart EventType = "play_start"
	EventPlayEnd   EventType = "play_end"
	EventTaskStart EventType = "task_start"
	EventTaskEnd   EventType = "task_end"

	// EventLog is a generic log line used by non-Ansible async
	// operations that reuse the tracker infrastructure.
	//
	// Specifically: the Phase 3.1 "provision host from vCenter
	// template" orchestrator (pkg/apis/infra/provision_host.go)
	// runs a sequence of vCenter API calls — not an Ansible
	// playbook — and wants to stream progress lines to the same
	// WebSocket subscribers that existing PaaS deploy flows use.
	// EventLog lets it push plain messages without pretending to
	// be an Ansible task. For EventLog, consumers should read
	// Message (not Task/Play) for display, and Status=="error"
	// (instead of "ok") to flag errors.
	EventLog EventType = "log"
)

// Event represents a structured executor event emitted during playbook execution.
//
// The Task / Play / Index / Total / Duration fields are populated only
// by the Ansible path (Types play_start / task_start / ...). The
// non-Ansible EventLog path uses Message for the display text and
// Status ("info" / "error") for the level.
type Event struct {
	Type      EventType      `json:"type"`
	Timestamp time.Time      `json:"timestamp"`
	Play      string         `json:"play,omitempty"`
	Task      string         `json:"task,omitempty"`
	Index     int            `json:"index,omitempty"`
	Total     int            `json:"total,omitempty"`
	Status    string         `json:"status,omitempty"`
	Output    map[string]any `json:"output,omitempty"`
	Error     string         `json:"error,omitempty"`
	Duration  int64          `json:"duration,omitempty"` // milliseconds

	// Message is a human-readable log line used by non-Ansible async
	// operations (EventLog type). Empty for Ansible events, which
	// identify their step via Task/Play instead.
	Message string `json:"message,omitempty"`
}

// EventCallback is called for each executor event during playbook execution.
type EventCallback func(Event)
