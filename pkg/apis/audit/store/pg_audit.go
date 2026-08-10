package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

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

func (s *pgAuditLogStore) List(ctx context.Context, query list.Query) (*list.Result[AuditLogRow], error) {
	offset, limit := list.PaginationToOffsetLimit(query.Pagination)

	filterParams := buildFilterParams(query.Filters)

	q := s.Q()
	count, err := q.CountAuditLogs(ctx, generated.CountAuditLogsParams{
		UserID:       filterParams.UserID,
		EventType:    filterParams.EventType,
		Action:       filterParams.Action,
		ResourceType: filterParams.ResourceType,
		ResourceID:   filterParams.ResourceID,
		Module:       filterParams.Module,
		ClientIp:     filterParams.ClientIp,
		WorkspaceID:  filterParams.WorkspaceID,
		NamespaceID:  filterParams.NamespaceID,
		Success:      filterParams.Success,
		StatusCode:   filterParams.StatusCode,
		StartTime:    filterParams.StartTime,
		EndTime:      filterParams.EndTime,
		Search:       filterParams.Search,
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
		UserID:       filterParams.UserID,
		EventType:    filterParams.EventType,
		Action:       filterParams.Action,
		ResourceType: filterParams.ResourceType,
		ResourceID:   filterParams.ResourceID,
		Module:       filterParams.Module,
		ClientIp:     filterParams.ClientIp,
		WorkspaceID:  filterParams.WorkspaceID,
		NamespaceID:  filterParams.NamespaceID,
		Success:      filterParams.Success,
		StatusCode:   filterParams.StatusCode,
		StartTime:    filterParams.StartTime,
		EndTime:      filterParams.EndTime,
		Search:       filterParams.Search,
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

func buildFilterParams(filters map[string]any) generated.ListAuditLogsParams {
	return generated.ListAuditLogsParams{
		UserID:       list.FilterInt64(filters, "userId"),
		EventType:    list.FilterStr(filters, "eventType"),
		Action:       list.FilterStr(filters, "action"),
		ResourceType: list.FilterStr(filters, "resourceType"),
		ResourceID:   list.FilterStr(filters, "resourceId"),
		Module:       list.FilterStr(filters, "module"),
		ClientIp:     list.FilterStr(filters, "clientIp"),
		WorkspaceID:  list.FilterInt64(filters, "workspaceId"),
		NamespaceID:  list.FilterInt64(filters, "namespaceId"),
		Success:      list.FilterBool(filters, "success"),
		StatusCode:   list.FilterInt32(filters, "statusCode"),
		StartTime:    list.FilterTime(filters, "startTime"),
		EndTime:      list.FilterTime(filters, "endTime"),
		Search:       list.FilterStr(filters, "search"),
	}
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
