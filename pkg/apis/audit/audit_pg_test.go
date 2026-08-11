package audit_test

import (
	"context"
	"testing"
	"time"

	libaudit "vraxel.io/vraxel/lib/audit"
	"vraxel.io/vraxel/lib/list"
	"vraxel.io/vraxel/pkg/apis/audit"
	"vraxel.io/vraxel/pkg/apis/audit/store"
	"vraxel.io/vraxel/pkg/db/dbtest"
)

// End-to-end through the real pipeline: async writer -> pg sink ->
// list query with filters.
func TestAuditWriterRoundtrip(t *testing.T) {
	database := dbtest.New(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w := audit.NewAuditWriter(database)
	w.Start(ctx)
	w.Log(libaudit.Event{
		Username:   "it-user",
		EventType:  "authentication",
		Action:     "login",
		HTTPMethod: "POST",
		HTTPPath:   "/oidc/login",
		StatusCode: 200,
		Success:    true,
		CreatedAt:  time.Now(),
	})
	w.Stop() // flushes the batch

	res, err := store.NewPGAuditLogStore(database).List(context.Background(), list.Query{
		Pagination: list.Pagination{Page: 1, PageSize: 10},
		Filters:    map[string]any{"username": "it-user"},
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if res.TotalCount != 1 || res.Items[0].Action != "login" {
		t.Fatalf("unexpected result: %+v", res)
	}
}
