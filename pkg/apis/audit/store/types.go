package store

import (
	"encoding/json"
	"time"
)

// AuditLogRow is the domain-level view of one audit log row. Store
// implementations translate sqlc row types (generated.AuditLog et al.)
// into this shape so upper layers never see pgtype / generated.
type AuditLogRow struct {
	ID             int64
	UserID         *int64
	Username       string
	EventType      string
	Action         string
	ResourceType   string
	ResourceID     string
	Module         string
	Scope          string
	WorkspaceID    *int64
	NamespaceID    *int64
	HTTPMethod     string
	HTTPPath       string
	StatusCode     int32
	ClientIP       string
	UserAgent      string
	DurationMs     int32
	Success        bool
	Detail         json.RawMessage
	ResponseDetail json.RawMessage
	CreatedAt      time.Time
}
