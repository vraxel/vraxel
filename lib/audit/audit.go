package audit

import (
	"context"
	"encoding/json"
	"time"
)

// Event represents a single audit log entry.
type Event struct {
	UserID         *int64
	Username       string
	EventType      string // "api_operation" | "authentication"
	Action         string
	ResourceType   string
	ResourceID     string
	Module         string
	Scope          string
	WorkspaceID    *int64
	NamespaceID    *int64
	HTTPMethod     string
	HTTPPath       string
	StatusCode     int
	ClientIP       string
	UserAgent      string
	DurationMs     int
	Success        bool
	Detail         json.RawMessage
	ResponseDetail json.RawMessage
	CreatedAt      time.Time
}

// JSONString wraps a plain string as a JSON-encoded string value (e.g. `"hello"`).
func JSONString(s string) json.RawMessage {
	b, _ := json.Marshal(s)
	return b
}

// Logger is the interface for emitting audit events.
type Logger interface {
	Log(event Event)
}

// Sink persists audit events to storage.
type Sink interface {
	BatchCreate(ctx context.Context, events []Event) error
}
