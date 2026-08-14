package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	libaudit "vraxel.io/vraxel/lib/audit"
	"vraxel.io/vraxel/lib/list"
	vraxeldb "vraxel.io/vraxel/pkg/db"
	"vraxel.io/vraxel/pkg/db/generated"
	"vraxel.io/vraxel/pkg/db/pgerrors"
)

type pgAuditLogStore struct {
	vraxeldb.Store
}

// NewPGAuditLogStore creates a PostgreSQL-backed store that implements
// both AuditLogStore (query) and libaudit.Sink (batch write).
func NewPGAuditLogStore(d *vraxeldb.DB) *pgAuditLogStore {
	return &pgAuditLogStore{Store: vraxeldb.Store{DB: d}}
}

// --- libaudit.Sink implementation ---

func (s *pgAuditLogStore) BatchCreate(ctx context.Context, events []libaudit.Event) error {
	q := s.Q()
	for _, e := range events {
		detail := e.Detail
		if detail == nil {
			detail = json.RawMessage("null")
		}
		responseDetail := e.ResponseDetail
		if responseDetail == nil {
			responseDetail = json.RawMessage("null")
		}
		err := q.CreateAuditLog(ctx, generated.CreateAuditLogParams{
			UserID:         e.UserID,
			Username:       e.Username,
			EventType:      e.EventType,
			Action:         e.Action,
			ResourceType:   e.ResourceType,
			ResourceID:     e.ResourceID,
			Module:         e.Module,
			Scope:          e.Scope,
			WorkspaceID:    e.WorkspaceID,
			NamespaceID:    e.NamespaceID,
			HttpMethod:     e.HTTPMethod,
			HttpPath:       e.HTTPPath,
			StatusCode:     int32(e.StatusCode),
			ClientIp:       e.ClientIP,
			UserAgent:      e.UserAgent,
			DurationMs:     int32(e.DurationMs),
			Success:        e.Success,
			Detail:         detail,
			ResponseDetail: responseDetail,
			CreatedAt:      e.CreatedAt,
		})
		if err != nil {
			return fmt.Errorf("create audit log: %w", err)
		}
	}
	return nil
}

// --- AuditLogStore implementation ---

func (s *pgAuditLogStore) GetByID(ctx context.Context, id int64) (*AuditLogRow, error) {
	row, err := s.Q().GetAuditLog(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("audit log %d: %w", id, pgerrors.ErrNotFound)
		}
		return nil, fmt.Errorf("get audit log: %w", err)
	}
	out := rowToDomain(row)
	return &out, nil
}

type auditFilters struct {
	UserID       *int64     `filter:"user_id"`
	EventType    *string    `filter:"event_type"`
	Action       *string    `filter:"action"`
	ResourceType *string    `filter:"resource_type"`
	ResourceID   *string    `filter:"resource_id"`
	Module       *string    `filter:"module"`
	ClientIp     *string    `filter:"client_ip"`
	WorkspaceID  *int64     `filter:"workspace_id"`
	NamespaceID  *int64     `filter:"namespace_id"`
	Success      *bool      `filter:"success"`
	StatusCode   *int32     `filter:"status_code"`
	StartTime    *time.Time `filter:"start_time"`
	EndTime      *time.Time `filter:"end_time"`
	Search       *string    `filter:"search"`
}

func (s *pgAuditLogStore) List(ctx context.Context, query list.Query) (*list.Result[AuditLogRow], error) {
	offset, limit := list.PaginationToOffsetLimit(query.Pagination)
	f := list.Parse[auditFilters](query.Filters)

	q := s.Q()
	count, err := q.CountAuditLogs(ctx, generated.CountAuditLogsParams{
		UserID:       f.UserID,
		EventType:    f.EventType,
		Action:       f.Action,
		ResourceType: f.ResourceType,
		ResourceID:   f.ResourceID,
		Module:       f.Module,
		ClientIp:     f.ClientIp,
		WorkspaceID:  f.WorkspaceID,
		NamespaceID:  f.NamespaceID,
		Success:      f.Success,
		StatusCode:   f.StatusCode,
		StartTime:    f.StartTime,
		EndTime:      f.EndTime,
		Search:       f.Search,
	})
	if err != nil {
		return nil, fmt.Errorf("count audit logs: %w", err)
	}

	sortField := query.SortBy
	if sortField == "" {
		sortField = "created_at"
	}
	sortOrder := query.SortOrder
	if sortOrder == "" {
		sortOrder = "desc"
	}

	rows, err := q.ListAuditLogs(ctx, generated.ListAuditLogsParams{
		UserID:       f.UserID,
		EventType:    f.EventType,
		Action:       f.Action,
		ResourceType: f.ResourceType,
		ResourceID:   f.ResourceID,
		Module:       f.Module,
		ClientIp:     f.ClientIp,
		WorkspaceID:  f.WorkspaceID,
		NamespaceID:  f.NamespaceID,
		Success:      f.Success,
		StatusCode:   f.StatusCode,
		StartTime:    f.StartTime,
		EndTime:      f.EndTime,
		Search:       f.Search,
		SortField:    sortField,
		SortOrder:    sortOrder,
		PageOffset:   offset,
		PageSize:     limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list audit logs: %w", err)
	}

	items := make([]AuditLogRow, len(rows))
	for i := range rows {
		items[i] = rowToDomain(rows[i])
	}
	return &list.Result[AuditLogRow]{
		Items:      items,
		TotalCount: count,
	}, nil
}

// rowToDomain maps the sqlc audit_logs row onto the store's domain type.
// Get and List select the same column list, so one mapper serves both.
func rowToDomain(r generated.AuditLog) AuditLogRow {
	return AuditLogRow{
		ID:             r.ID,
		UserID:         r.UserID,
		Username:       r.Username,
		EventType:      r.EventType,
		Action:         r.Action,
		ResourceType:   r.ResourceType,
		ResourceID:     r.ResourceID,
		Module:         r.Module,
		Scope:          r.Scope,
		WorkspaceID:    r.WorkspaceID,
		NamespaceID:    r.NamespaceID,
		HTTPMethod:     r.HttpMethod,
		HTTPPath:       r.HttpPath,
		StatusCode:     r.StatusCode,
		ClientIP:       r.ClientIp,
		UserAgent:      r.UserAgent,
		DurationMs:     r.DurationMs,
		Success:        r.Success,
		Detail:         nonNullJSON(r.Detail),
		ResponseDetail: nonNullJSON(r.ResponseDetail),
		CreatedAt:      r.CreatedAt,
	}
}

// nonNullJSON strips SQL NULL / JSON-null so omitempty works.
func nonNullJSON(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil
	}
	return raw
}
