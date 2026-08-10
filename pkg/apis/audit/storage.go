package audit

import (
	"strconv"
	"time"

	"vraxel.io/vraxel/lib/apiserver"
	"vraxel.io/vraxel/lib/list"
	"vraxel.io/vraxel/lib/runtime"
	modstore "vraxel.io/vraxel/pkg/apis/audit/store"
)

// register declares the read-only audit log resource. ID parsing,
// list-query translation, the list envelope and domain-error mapping
// all live in the framework now — the handlers are pure business.
//
// +openapi:noDerive
// +openapi:path=/logs
// +openapi:resource=AuditLog
func register(s *apiserver.Server, stores modstore.Stores) {
	apiserver.Register(s, apiserver.ResourceDef[AuditLog]{
		Group:  "audit",
		Name:   "logs",
		Scopes: apiserver.ScopePlatform,
		Ops: apiserver.Ops[AuditLog]{
			Get: func(ctx apiserver.Ctx, id int64) (*AuditLog, error) {
				row, err := stores.Log.GetByID(ctx, id)
				if err != nil {
					return nil, err // framework maps domain sentinels
				}
				return dbAuditLogToAPI(row), nil
			},
			List: func(ctx apiserver.Ctx, q list.Query) (*list.Result[AuditLog], error) {
				result, err := stores.Log.List(ctx, q)
				if err != nil {
					return nil, err
				}
				items := make([]AuditLog, len(result.Items))
				for i := range result.Items {
					items[i] = *dbAuditLogToAPI(&result.Items[i])
				}
				return &list.Result[AuditLog]{Items: items, TotalCount: result.TotalCount}, nil
			},
		},
	})
}

func dbAuditLogToAPI(row *modstore.AuditLogRow) *AuditLog {
	spec := AuditLogSpec{
		ID:             strconv.FormatInt(row.ID, 10),
		Username:       row.Username,
		EventType:      row.EventType,
		Action:         row.Action,
		ResourceType:   row.ResourceType,
		ResourceID:     row.ResourceID,
		Module:         row.Module,
		Scope:          row.Scope,
		HTTPMethod:     row.HTTPMethod,
		HTTPPath:       row.HTTPPath,
		StatusCode:     int(row.StatusCode),
		ClientIP:       row.ClientIP,
		UserAgent:      row.UserAgent,
		DurationMs:     int(row.DurationMs),
		Success:        row.Success,
		Detail:         row.Detail,
		ResponseDetail: row.ResponseDetail,
		CreatedAt:      row.CreatedAt.Format(time.RFC3339),
	}
	if row.UserID != nil {
		s := strconv.FormatInt(*row.UserID, 10)
		spec.UserID = &s
	}
	if row.WorkspaceID != nil {
		s := strconv.FormatInt(*row.WorkspaceID, 10)
		spec.WorkspaceID = &s
	}
	if row.NamespaceID != nil {
		s := strconv.FormatInt(*row.NamespaceID, 10)
		spec.NamespaceID = &s
	}
	return &AuditLog{
		TypeMeta: runtime.TypeMeta{Kind: "AuditLog"},
		Spec:     spec,
	}
}
