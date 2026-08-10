package store

import (
	"context"

	"vraxel.io/vraxel/lib/list"
	vraxeldb "vraxel.io/vraxel/pkg/db"
)

// AuditLogStore abstracts database queries for audit logs.
type AuditLogStore interface {
	GetByID(ctx context.Context, id int64) (*AuditLogRow, error)
	List(ctx context.Context, query list.Query) (*list.Result[AuditLogRow], error)
}

// Stores aggregates every store impl the audit module exposes to its
// business layer. NewModule wires these.
type Stores struct {
	Log AuditLogStore
}

// NewStores constructs the default pg-backed Stores bundle.
func NewStores(d *vraxeldb.DB) Stores {
	return Stores{
		Log: NewPGAuditLogStore(d),
	}
}
