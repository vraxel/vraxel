package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"vraxel.io/vraxel/lib/apiserver"
	"vraxel.io/vraxel/lib/list"
	modstore "vraxel.io/vraxel/pkg/apis/audit/store"
	"vraxel.io/vraxel/pkg/db/pgerrors"
)

type stubLogStore struct {
	lastQuery list.Query
}

func (s *stubLogStore) GetByID(ctx context.Context, id int64) (*modstore.AuditLogRow, error) {
	if id == 404 {
		return nil, fmt.Errorf("audit log %d: %w", id, pgerrors.ErrNotFound)
	}
	return &modstore.AuditLogRow{ID: id, Username: "alice", Action: "create", CreatedAt: time.Unix(1720000000, 0).UTC()}, nil
}

func (s *stubLogStore) List(ctx context.Context, q list.Query) (*list.Result[modstore.AuditLogRow], error) {
	s.lastQuery = q
	return &list.Result[modstore.AuditLogRow]{
		Items:      []modstore.AuditLogRow{{ID: 1, Username: "alice", CreatedAt: time.Unix(1720000000, 0).UTC()}},
		TotalCount: 1,
	}, nil
}

func newServer(t *testing.T) (*apiserver.Server, *stubLogStore) {
	t.Helper()
	stub := &stubLogStore{}
	s := apiserver.New(apiserver.Config{})
	register(s, modstore.Stores{Log: stub})
	return s, stub
}

func get(t *testing.T, s *apiserver.Server, path string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
	return w
}

func TestAuditLogList(t *testing.T) {
	s, stub := newServer(t)

	w := get(t, s, "/api/audit/v1/logs?page=2&pageSize=10&action=create&sortBy=created_at&sortOrder=asc")
	if w.Code != 200 {
		t.Fatalf("list = %d %s", w.Code, w.Body.String())
	}
	var resp struct {
		Kind       string     `json:"kind"`
		APIVersion string     `json:"apiVersion"`
		Items      []AuditLog `json:"items"`
		TotalCount int64      `json:"totalCount"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Kind != "AuditLogList" || resp.APIVersion != "audit/v1" || resp.TotalCount != 1 {
		t.Errorf("envelope: %s", w.Body.String())
	}

	// v1 ParseListOptions semantics reached the store untranslated.
	q := stub.lastQuery
	if q.Pagination.Page != 2 || q.Pagination.PageSize != 10 {
		t.Errorf("pagination = %+v", q.Pagination)
	}
	if q.Filters["action"] != "create" {
		t.Errorf("filters = %+v", q.Filters)
	}
	if q.SortBy != "created_at" || q.SortOrder != "asc" {
		t.Errorf("sort = %s/%s", q.SortBy, q.SortOrder)
	}
	// Reserved pagination keys never leak into filters.
	for _, reserved := range []string{"page", "pageSize", "sortBy", "sortOrder"} {
		if _, ok := q.Filters[reserved]; ok {
			t.Errorf("reserved key %q leaked into filters", reserved)
		}
	}
}

func TestAuditLogGet(t *testing.T) {
	s, _ := newServer(t)

	w := get(t, s, "/api/audit/v1/logs/7")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"kind":"AuditLog"`) {
		t.Errorf("get = %d %s", w.Code, w.Body.String())
	}

	// Byte parity: v1 message is "invalid log ID: abc".
	w = get(t, s, "/api/audit/v1/logs/abc")
	if w.Code != 400 || !strings.Contains(w.Body.String(), "invalid log ID: abc") {
		t.Errorf("invalid id = %d %s", w.Code, w.Body.String())
	}

	// Store NotFound sentinel maps to 404 with the store's message.
	w = get(t, s, "/api/audit/v1/logs/404")
	if w.Code != 404 || !strings.Contains(w.Body.String(), "audit log 404") {
		t.Errorf("not found = %d %s", w.Code, w.Body.String())
	}
}

func TestAuditPermRecords(t *testing.T) {
	s, _ := newServer(t)
	got := map[string]bool{}
	for _, rec := range s.PermRecordsByModule()["audit"] {
		got[rec.Code+"|"+rec.Scope] = true
	}
	for _, want := range []string{"audit:logs:get|platform", "audit:logs:list|platform"} {
		if !got[want] {
			t.Errorf("missing perm %s in %v", want, got)
		}
	}
	if len(got) != 2 {
		t.Errorf("platform-only resource must emit exactly 2 records, got %v", got)
	}
}
