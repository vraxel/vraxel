package apiserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apierrors "vraxel.io/vraxel/lib/api/errors"
	"vraxel.io/vraxel/lib/api/types"
	"vraxel.io/vraxel/lib/audit"
	"vraxel.io/vraxel/lib/list"
	"vraxel.io/vraxel/lib/oidc"
	"vraxel.io/vraxel/lib/rest/filters"
	"vraxel.io/vraxel/lib/runtime"
)

// Widget is the test resource type — same envelope conventions as every
// Vraxel API type (TypeMeta + metadata + spec).
type Widget struct {
	runtime.TypeMeta `json:",inline"`
	types.ObjectMeta `json:"metadata"`
	Spec             WidgetSpec `json:"spec"`
}

func (w *Widget) GetTypeMeta() *runtime.TypeMeta { return &w.TypeMeta }

type WidgetSpec struct {
	Color string `json:"color,omitempty"`
}

func newWidget(id, color string) Widget {
	return Widget{
		TypeMeta:   runtime.TypeMeta{Kind: "Widget"},
		ObjectMeta: types.ObjectMeta{ID: id, Name: "w" + id},
		Spec:       WidgetSpec{Color: color},
	}
}

// testDef returns a fully-populated def backed by canned responses.
func testDef() ResourceDef[Widget] {
	return ResourceDef[Widget]{
		Group:  "test",
		Name:   "widgets",
		Scopes: ScopeAll,
		Ops: Ops[Widget]{
			List: func(ctx Ctx, q list.Query) (*list.Result[Widget], error) {
				items := []Widget{newWidget("1", "red")}
				if ws, ok := q.Filters["workspace_id"]; ok {
					items[0].Spec.Color = "ws-" + ws.(string)
				}
				return &list.Result[Widget]{Items: items, TotalCount: 1}, nil
			},
			Get: func(ctx Ctx, id int64) (*Widget, error) {
				if id == 404 {
					return nil, apierrors.NewNotFound("widget", "404")
				}
				w := newWidget(fmt.Sprint(id), "blue")
				return &w, nil
			},
			Create: func(ctx Ctx, in *Widget) (*Widget, error) {
				if ctx.DryRun {
					return in, nil
				}
				out := newWidget("9", in.Spec.Color)
				return &out, nil
			},
			Update: func(ctx Ctx, id int64, in *Widget) (*Widget, error) {
				out := newWidget(fmt.Sprint(id), in.Spec.Color)
				return &out, nil
			},
			Delete: func(ctx Ctx, id int64) error { return nil },
			BatchDelete: func(ctx Ctx, ids []int64) (*BatchResult, error) {
				return &BatchResult{SuccessCount: len(ids)}, nil
			},
		},
		Verbs: []VerbDef{
			Verb("gears", func(ctx Ctx, id int64, q list.Query) (*list.Result[Widget], error) {
				w := newWidget(fmt.Sprint(id), "gear")
				return &list.Result[Widget]{Items: []Widget{w}, TotalCount: 1}, nil
			}),
		},
		Actions: []ActionDef{
			Action("spin", "POST", []string{"test:widgets:update"},
				func(ctx Ctx, req *struct {
					Speed int `json:"speed"`
				}) (*Widget, error) {
					// Typed item actions read the target from ctx.ID.
					w := newWidget(fmt.Sprint(ctx.ID), fmt.Sprintf("spin-%d", req.Speed))
					return &w, nil
				}),
			Action("silence", "POST", []string{"test:widgets:update"},
				func(ctx Ctx, req *struct{}) (*Widget, error) {
					return nil, nil // nil result → 204 (v1 parity)
				}),
		},
	}
}

func newTestServer(t *testing.T, defs ...func(*Server)) *Server {
	t.Helper()
	s := New(Config{})
	if len(defs) == 0 {
		Register(s, testDef())
	}
	for _, f := range defs {
		f(s)
	}
	return s
}

func doJSON(t *testing.T, s *Server, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}

func TestScopeExpansion(t *testing.T) {
	s := newTestServer(t)
	want := []string{
		"GET /api/test/v1/widgets",
		"GET /api/test/v1/workspaces/{workspaceId}/widgets",
		"GET /api/test/v1/workspaces/{workspaceId}/namespaces/{namespaceId}/widgets",
		"POST /api/test/v1/widgets/{widgetId}/spin",
	}
	got := strings.Join(s.Patterns(), "\n")
	for _, p := range want {
		if !strings.Contains(got, p) {
			t.Errorf("missing pattern %q in:\n%s", p, got)
		}
	}
}

func TestListEnvelope(t *testing.T) {
	s := newTestServer(t)
	w := doJSON(t, s, "GET", "/api/test/v1/widgets", "")
	if w.Code != 200 {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
	var resp struct {
		Kind       string   `json:"kind"`
		APIVersion string   `json:"apiVersion"`
		Items      []Widget `json:"items"`
		TotalCount int64    `json:"totalCount"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Kind != "WidgetList" || resp.APIVersion != "test/v1" || resp.TotalCount != 1 || len(resp.Items) != 1 {
		t.Errorf("unexpected envelope: %s", w.Body.String())
	}
	// v1 wire uses totalCount (camel), never total_count.
	if strings.Contains(w.Body.String(), "total_count") {
		t.Error("list envelope leaked snake_case total_count")
	}
}

func TestScopedCtxValues(t *testing.T) {
	var got ScopeInfo
	def := testDef()
	def.Ops.List = func(ctx Ctx, q list.Query) (*list.Result[Widget], error) {
		got = ctx.Scope
		return &list.Result[Widget]{}, nil
	}
	s := New(Config{})
	Register(s, def)

	doJSON(t, s, "GET", "/api/test/v1/workspaces/7/namespaces/42/widgets", "")
	if got.WorkspaceID != 7 || got.NamespaceID != 42 || got.Level != ScopeNamespace {
		t.Errorf("scope = %+v", got)
	}

	doJSON(t, s, "GET", "/api/test/v1/widgets", "")
	if got.WorkspaceID != 0 || got.NamespaceID != 0 || got.Level != ScopePlatform {
		t.Errorf("platform scope = %+v", got)
	}
}

func TestGetAndInvalidID(t *testing.T) {
	s := newTestServer(t)

	w := doJSON(t, s, "GET", "/api/test/v1/widgets/5", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"apiVersion":"test/v1"`) {
		t.Errorf("get: %d %s", w.Code, w.Body.String())
	}

	w = doJSON(t, s, "GET", "/api/test/v1/widgets/abc", "")
	if w.Code != 400 || !strings.Contains(w.Body.String(), "invalid widget ID: abc") {
		t.Errorf("invalid id: %d %s", w.Code, w.Body.String())
	}

	w = doJSON(t, s, "GET", "/api/test/v1/widgets/404", "")
	if w.Code != 404 {
		t.Errorf("not found: %d %s", w.Code, w.Body.String())
	}
}

func TestCreateStatusCodes(t *testing.T) {
	s := newTestServer(t)

	w := doJSON(t, s, "POST", "/api/test/v1/widgets", `{"spec":{"color":"green"}}`)
	if w.Code != 201 {
		t.Errorf("create = %d, want 201", w.Code)
	}
	w = doJSON(t, s, "POST", "/api/test/v1/widgets?dryRun=true", `{"spec":{"color":"green"}}`)
	if w.Code != 200 {
		t.Errorf("dryRun create = %d, want 200", w.Code)
	}
}

func TestDeleteAndBatch(t *testing.T) {
	s := newTestServer(t)

	w := doJSON(t, s, "DELETE", "/api/test/v1/widgets/5", "")
	if w.Code != 204 {
		t.Errorf("delete = %d, want 204", w.Code)
	}

	w = doJSON(t, s, "DELETE", "/api/test/v1/widgets", `{"ids":["1","2"]}`)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"successCount":2`) {
		t.Errorf("batch = %d %s", w.Code, w.Body.String())
	}

	// v1 quirk parity: empty id list is a 500.
	w = doJSON(t, s, "DELETE", "/api/test/v1/widgets", `{"ids":[]}`)
	if w.Code != 500 {
		t.Errorf("empty ids = %d, want 500 (v1 parity)", w.Code)
	}
}

func TestVerbRoutes(t *testing.T) {
	s := newTestServer(t)
	w := doJSON(t, s, "GET", "/api/test/v1/widgets/3/gears", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"color":"gear"`) {
		t.Errorf("verb route: %d %s", w.Code, w.Body.String())
	}

	// Unknown verb has no route: plain 404. The historical colon URL is
	// gone with the shim (frontend migrated to path-segment verbs).
	w = doJSON(t, s, "GET", "/api/test/v1/widgets/3/nope", "")
	if w.Code != 404 {
		t.Errorf("unknown verb = %d, want 404", w.Code)
	}
	w = doJSON(t, s, "GET", "/api/test/v1/widgets/3:gears", "")
	if w.Code != 400 {
		t.Errorf("colon URL = %d, want 400 (invalid id, shim removed)", w.Code)
	}
}

func TestTypedActions(t *testing.T) {
	s := newTestServer(t)

	w := doJSON(t, s, "POST", "/api/test/v1/widgets/7/spin", `{"speed":3}`)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "spin-3") {
		t.Errorf("typed action: %d %s", w.Code, w.Body.String())
	}
	// D2 regression: the typed handler saw the item id via ctx.ID.
	if !strings.Contains(w.Body.String(), `"id":"7"`) {
		t.Errorf("typed action missing ctx.ID: %s", w.Body.String())
	}

	// D1 regression: nil typed result is a 204, not a panic.
	w = doJSON(t, s, "POST", "/api/test/v1/widgets/7/silence", `{}`)
	if w.Code != 204 {
		t.Errorf("nil action result = %d, want 204", w.Code)
	}
}

func TestNilGetResultMatchesV1(t *testing.T) {
	// D1 regression: (nil, nil) from Ops must not panic; v1 serialized a
	// nil interface (JSON "null", 200).
	def := testDef()
	def.Ops.Get = func(ctx Ctx, id int64) (*Widget, error) { return nil, nil }
	s := New(Config{})
	Register(s, def)

	w := doJSON(t, s, "GET", "/api/test/v1/widgets/1", "")
	if w.Code != 200 {
		t.Errorf("nil get = %d, want 200 (v1 serialized null)", w.Code)
	}
}

func TestActionStatusCodeFieldEdit(t *testing.T) {
	// D11 regression: editing StatusCode on the returned ActionDef must
	// take effect just like Sensitive/OnItem edits do.
	ad := Action("later", "POST", []string{"test:widgets:update"},
		func(ctx Ctx, req *struct{}) (*Widget, error) {
			w := newWidget("1", "later")
			return &w, nil
		})
	ad.StatusCode = 202

	def := testDef()
	def.Actions = []ActionDef{ad}
	s := New(Config{})
	Register(s, def)

	w := doJSON(t, s, "POST", "/api/test/v1/widgets/1/later", "{}")
	if w.Code != 202 {
		t.Errorf("edited StatusCode = %d, want 202", w.Code)
	}
}

func TestFailClosed(t *testing.T) {
	s := newTestServer(t)
	w := doJSON(t, s, "GET", "/api/test/v1/unknown-things", "")
	if w.Code != 404 {
		t.Errorf("unregistered path = %d, want 404 (fail-closed)", w.Code)
	}
}

func TestPermRecords(t *testing.T) {
	s := newTestServer(t)
	records := s.PermRecordsByModule()["test"]

	type key struct{ code, method, path, scope string }
	got := make(map[key]bool, len(records))
	for _, rec := range records {
		got[key{rec.Code, rec.Method, rec.Path, rec.Scope}] = true
	}

	// Natural scope namespace → fan out to all three levels (v1 scopesUpTo).
	for _, scope := range []string{"namespace", "workspace", "platform"} {
		if !got[key{"test:widgets:list", "GET", "/api/test/v1/widgets", scope}] {
			t.Errorf("missing list perm at scope %s", scope)
		}
		if !got[key{"test:widgets:get", "GET", "/api/test/v1/widgets/{widgetId}", scope}] {
			t.Errorf("missing get perm at scope %s", scope)
		}
	}
	// Action targets covered by resource verbs generate no extra records.
	for k := range got {
		if k.path == "/api/test/v1/widgets/{widgetId}/spin" {
			t.Errorf("covered action target should not emit records: %+v", k)
		}
	}
}

func TestSingletonGet(t *testing.T) {
	s := New(Config{})
	Register(s, ResourceDef[Widget]{
		Group: "test", Name: "summary", Scopes: ScopePlatform | ScopeWorkspace,
		SingletonGet: func(ctx Ctx) (*Widget, error) {
			w := newWidget(fmt.Sprint(ctx.Scope.WorkspaceID), "single")
			return &w, nil
		},
	})

	w := doJSON(t, s, "GET", "/api/test/v1/summary", "")
	if w.Code != 200 {
		t.Fatalf("singleton = %d %s", w.Code, w.Body.String())
	}
	// Single object on the wire — never the list envelope.
	if strings.Contains(w.Body.String(), `"items"`) || strings.Contains(w.Body.String(), `"totalCount"`) {
		t.Errorf("singleton got list-enveloped: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"apiVersion":"test/v1"`) {
		t.Errorf("missing apiVersion stamp: %s", w.Body.String())
	}

	// Scoped route sees typed scope.
	w = doJSON(t, s, "GET", "/api/test/v1/workspaces/7/summary", "")
	if !strings.Contains(w.Body.String(), `"id":"7"`) {
		t.Errorf("scoped singleton: %s", w.Body.String())
	}

	// Permission verb is "list" (v1 Lister-derived code compatibility).
	found := false
	for _, rec := range s.PermRecordsByModule()["test"] {
		if rec.Code == "test:summary:list" {
			found = true
		}
	}
	if !found {
		t.Error("singleton must derive the list permission code")
	}
}

func TestSingletonListMutuallyExclusive(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("SingletonGet + Ops.List should panic")
		}
	}()
	s := New(Config{})
	def := testDef()
	def.SingletonGet = func(ctx Ctx) (*Widget, error) { return nil, nil }
	Register(s, def)
}

func TestVerbOnlyResourcePermRecord(t *testing.T) {
	// D4 regression: the shim's item GET route checks the get code, so
	// the permission record must exist even without Ops.Get — otherwise
	// non-admins are permanently 403 on a code no role can hold.
	s := New(Config{})
	Register(s, ResourceDef[Widget]{
		Group: "test", Name: "views", Scopes: ScopePlatform,
		Ops: Ops[Widget]{
			List: func(ctx Ctx, q list.Query) (*list.Result[Widget], error) {
				return &list.Result[Widget]{}, nil
			},
		},
		Verbs: []VerbDef{
			Verb("kids", func(ctx Ctx, id int64, q list.Query) (*list.Result[Widget], error) {
				return &list.Result[Widget]{}, nil
			}),
		},
	})
	found := false
	for _, p := range s.PermRecordsByModule()["test"] {
		if p.Code == "test:views:get" {
			found = true
		}
	}
	if !found {
		t.Error("verb-only resource must emit its get permission record")
	}
}

// auditLoggerFunc adapts a func to audit.Logger for assertions.
type auditLoggerFunc func(audit.Event)

func (f auditLoggerFunc) Log(e audit.Event) { f(e) }

func TestAuditSensitiveAndResourceID(t *testing.T) {
	var got []audit.Event
	logger := auditLoggerFunc(func(e audit.Event) { got = append(got, e) })

	def := testDef()
	def.Sensitive = true // D3 regression: resource-level redaction
	s := New(Config{AuditLogger: logger})
	Register(s, def)

	doJSON(t, s, "POST", "/api/test/v1/widgets", `{"spec":{"color":"secret"}}`)
	if len(got) != 1 {
		t.Fatalf("audit events = %d, want 1", len(got))
	}
	if got[0].Detail != nil {
		t.Errorf("sensitive resource leaked request body into audit detail: %s", got[0].Detail)
	}
	// A sensitive create RESPONSE may reveal a one-time secret (api_server_key
	// / token); it must NOT be persisted to audit either — Sensitive gates
	// both directions.
	if got[0].ResponseDetail != nil {
		t.Errorf("sensitive resource leaked response body into audit ResponseDetail: %s", got[0].ResponseDetail)
	}
	if got[0].ResourceID != "9" { // still extracted from create response metadata.id
		t.Errorf("create ResourceID = %q, want 9", got[0].ResourceID)
	}

	got = nil
	doJSON(t, s, "DELETE", "/api/test/v1/widgets/5", "")
	if len(got) != 1 || got[0].ResourceID != "5" || got[0].Action != "delete" {
		t.Errorf("delete audit = %+v", got)
	}

	// GET is not audited (non-interactive).
	got = nil
	doJSON(t, s, "GET", "/api/test/v1/widgets", "")
	if len(got) != 0 {
		t.Errorf("list should not be audited, got %+v", got)
	}
}

// stubChecker drives the authz middleware.
type stubChecker struct {
	admin bool
	allow bool
}

func (c *stubChecker) CheckPermission(ctx context.Context, userID int64, code, scope string, ws, ns int64) (bool, error) {
	return c.allow, nil
}
func (c *stubChecker) CheckAnyPermission(ctx context.Context, userID int64, codes []string, scope string, ws, ns int64) (bool, error) {
	return c.allow, nil
}
func (c *stubChecker) IsPlatformAdmin(ctx context.Context, userID int64) (bool, error) {
	return c.admin, nil
}
func (c *stubChecker) GetAccessibleWorkspaceIDs(ctx context.Context, userID int64) ([]int64, error) {
	return nil, nil
}
func (c *stubChecker) GetAccessibleNamespaceIDs(ctx context.Context, userID int64) ([]int64, error) {
	return nil, nil
}

func TestAuthzDeniedShape(t *testing.T) {
	s := New(Config{Checker: &stubChecker{allow: false}})
	Register(s, testDef())

	r := httptest.NewRequest("GET", "/api/test/v1/widgets", nil)
	r = oidc.WithUserID(r, 42)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)

	if w.Code != 403 {
		t.Fatalf("status = %d, want 403", w.Code)
	}
	// Byte-parity with v1 filters.forbiddenError: hand-written Status JSON
	// with status:"Failure", not the numeric StatusError shape.
	want := `{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"Forbidden","message":"access denied: requires test:widgets:list"}`
	if w.Body.String() != want {
		t.Errorf("403 body = %s, want %s", w.Body.String(), want)
	}
}

func TestAuthzNoUser(t *testing.T) {
	s := New(Config{Checker: &stubChecker{}})
	Register(s, testDef())

	w := doJSON(t, s, "GET", "/api/test/v1/widgets", "")
	if w.Code != 403 || !strings.Contains(w.Body.String(), "no authenticated user") {
		t.Errorf("unauthenticated = %d %s", w.Code, w.Body.String())
	}
}

func TestAuthzAdminBypassAndAllow(t *testing.T) {
	s := New(Config{Checker: &stubChecker{admin: true}})
	Register(s, testDef())

	r := httptest.NewRequest("GET", "/api/test/v1/widgets", nil)
	r = oidc.WithUserID(r, 1)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Errorf("admin bypass = %d", w.Code)
	}
}

func TestParentNesting(t *testing.T) {
	var gotParents []int64
	s := New(Config{})
	Register(s, testDef())

	child := ResourceDef[Widget]{
		Group: "test", Name: "cogs", Parent: "widgets", Scopes: ScopePlatform,
		Ops: Ops[Widget]{
			Get: func(ctx Ctx, id int64) (*Widget, error) {
				gotParents = ctx.Parents
				w := newWidget(fmt.Sprint(id), "cog")
				return &w, nil
			},
		},
	}
	Register(s, child)

	w := doJSON(t, s, "GET", "/api/test/v1/widgets/7/cogs/3", "")
	if w.Code != 200 {
		t.Fatalf("nested get = %d %s", w.Code, w.Body.String())
	}
	if len(gotParents) != 1 || gotParents[0] != 7 {
		t.Errorf("parents = %v, want [7]", gotParents)
	}

	perms := s.PermRecordsByModule()["test"]
	found := false
	for _, p := range perms {
		if p.Code == "test:widgets:cogs:get" && p.Path == "/api/test/v1/widgets/{widgetId}/cogs/{cogId}" {
			found = true
		}
	}
	if !found {
		t.Errorf("missing nested perm record; got %+v", perms)
	}
}

func TestBodyLimit(t *testing.T) {
	def := testDef()
	def.MaxBodyBytes = 16
	s := New(Config{})
	Register(s, def)

	w := doJSON(t, s, "POST", "/api/test/v1/widgets", `{"spec":{"color":"`+strings.Repeat("x", 64)+`"}}`)
	if w.Code != 413 {
		t.Errorf("oversized body = %d, want 413", w.Code)
	}
}

func TestDuplicateResourcePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("duplicate registration should panic")
		}
	}()
	s := New(Config{})
	Register(s, testDef())
	Register(s, testDef())
}

func TestVerbOnlyResource(t *testing.T) {
	s := New(Config{})
	def := ResourceDef[Widget]{
		Group: "test", Name: "views", Scopes: ScopePlatform,
		Ops: Ops[Widget]{
			List: func(ctx Ctx, q list.Query) (*list.Result[Widget], error) {
				return &list.Result[Widget]{}, nil
			},
		},
		Verbs: []VerbDef{
			Verb("children", func(ctx Ctx, id int64, q list.Query) (*list.Result[Widget], error) {
				w := newWidget(fmt.Sprint(id), "child")
				return &list.Result[Widget]{Items: []Widget{w}, TotalCount: 1}, nil
			}),
		},
	}
	Register(s, def)

	// Verb route works even though Ops.Get is nil (verb-only resource);
	// the handler sees the item id via ctx.ID.
	w := doJSON(t, s, "GET", "/api/test/v1/views/5/children", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "child") {
		t.Errorf("verb-only route = %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"id":"5"`) {
		t.Errorf("verb saw wrong id: %s", w.Body.String())
	}
	// Plain item GET on a verb-only resource has no route: 404.
	w = doJSON(t, s, "GET", "/api/test/v1/views/5", "")
	if w.Code != 404 {
		t.Errorf("verb-only plain get = %d, want 404", w.Code)
	}
}

func TestHyphenatedResourceParams(t *testing.T) {
	// ServeMux rejects hyphenated wildcards, but the framework derives the
	// item param "call-logId" for "call-logs" and sanitizes the wildcard in
	// the route pattern. The id must still parse and reach the handler via
	// ctx.ID (registration would panic if the wildcard were not sanitized).
	s := New(Config{})
	Register(s, ResourceDef[Widget]{
		Group: "test", Name: "call-logs",
		Ops: Ops[Widget]{
			Get: func(ctx Ctx, id int64) (*Widget, error) {
				w := newWidget(fmt.Sprint(id), "hyphen")
				return &w, nil
			},
		},
	})

	w := doJSON(t, s, "GET", "/api/test/v1/call-logs/7", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"id":"7"`) {
		t.Fatalf("hyphenated get = %d %s", w.Code, w.Body.String())
	}
}

// fakeChecker: user 1 is admin; user 2 is a plain user with no grants
// but bindings on workspace 7 / namespace 9.
type fakeChecker struct{}

func (fakeChecker) CheckPermission(ctx context.Context, userID int64, code, scope string, ws, ns int64) (bool, error) {
	return false, nil
}
func (fakeChecker) CheckAnyPermission(ctx context.Context, userID int64, codes []string, scope string, ws, ns int64) (bool, error) {
	return false, nil
}
func (fakeChecker) IsPlatformAdmin(ctx context.Context, userID int64) (bool, error) {
	return userID == 1, nil
}
func (fakeChecker) GetAccessibleWorkspaceIDs(ctx context.Context, userID int64) ([]int64, error) {
	return []int64{7}, nil
}
func (fakeChecker) GetAccessibleNamespaceIDs(ctx context.Context, userID int64) ([]int64, error) {
	return []int64{9}, nil
}

func doAsUser(t *testing.T, s *Server, userID int64, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, nil)
	r = oidc.WithUserID(r, userID)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}

// TestResourceDefIAMHooks pins the typed-def pass-through of the three
// iam special fields (ExtraAllow / ListAccessScope; PermScope is pinned
// by the permission-record fan-out), exercised through Register.
func TestResourceDefIAMHooks(t *testing.T) {
	var got *filters.AccessFilter
	s := New(Config{Checker: fakeChecker{}})
	Register(s, ResourceDef[Widget]{
		Group: "iam", Name: "workspaces",
		Ops: Ops[Widget]{
			List: func(ctx Ctx, q list.Query) (*list.Result[Widget], error) {
				got = ctx.Access
				w := newWidget("7", "ws")
				return &list.Result[Widget]{Items: []Widget{w}, TotalCount: 1}, nil
			},
		},
		ListAccessScope: "workspaces",
		PermScope:       ScopeWorkspace,
	})
	Register(s, ResourceDef[Widget]{
		Group: "iam", Name: "users",
		Ops: Ops[Widget]{
			Get: func(ctx Ctx, id int64) (*Widget, error) {
				w := newWidget(fmt.Sprint(id), "self")
				return &w, nil
			},
		},
		ExtraAllow: func(permCode string, r *http.Request, userID int64) bool {
			if permCode != "iam:users:get" {
				return false
			}
			return r.PathValue("userId") == "2" && userID == 2
		},
	})

	// Non-admin list: allowed, with binding-scoped filter injected.
	if w := doAsUser(t, s, 2, "GET", "/api/iam/v1/workspaces"); w.Code != 200 {
		t.Fatalf("non-admin list = %d %s", w.Code, w.Body.String())
	}
	if got == nil || len(got.WorkspaceIDs) != 1 || got.WorkspaceIDs[0] != 7 {
		t.Errorf("access filter = %+v", got)
	}
	// PermScope override: records fan out from workspace scope.
	recs := s.PermRecordsByModule()["iam"]
	foundWS := false
	for _, r := range recs {
		if r.Code == "iam:workspaces:list" && r.Scope == "workspace" {
			foundWS = true
		}
	}
	if !foundWS {
		t.Errorf("PermScope override missing workspace-scope record: %+v", recs)
	}
	// User 2 reading themselves: ExtraAllow bypasses the failing checker.
	if w := doAsUser(t, s, 2, "GET", "/api/iam/v1/users/2"); w.Code != 200 {
		t.Errorf("self get = %d %s", w.Code, w.Body.String())
	}
	// User 2 reading user 3: falls through to checker -> 403.
	if w := doAsUser(t, s, 2, "GET", "/api/iam/v1/users/3"); w.Code != 403 {
		t.Errorf("other get = %d", w.Code)
	}
}

func TestNamedResource(t *testing.T) {
	s := New(Config{})
	RegisterNamed(s, NamedDef[Widget]{
		Group: "test", Name: "settings", IDParam: "settingKey",
		Ops: NamedOps[Widget]{
			List: func(ctx Ctx, q list.Query) (*list.Result[Widget], error) {
				w := newWidget("k1", "list")
				return &list.Result[Widget]{Items: []Widget{w}, TotalCount: 1}, nil
			},
			Get: func(ctx Ctx, name string) (*Widget, error) {
				w := newWidget(name, "got")
				return &w, nil
			},
			Patch: func(ctx Ctx, name string, body json.RawMessage) (*Widget, error) {
				w := newWidget(name, "patched")
				return &w, nil
			},
		},
	})

	w := doJSON(t, s, "GET", "/api/test/v1/settings/chat.enabled", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"id":"chat.enabled"`) {
		t.Errorf("named get = %d %s", w.Code, w.Body.String())
	}
	w = doJSON(t, s, "PATCH", "/api/test/v1/settings/chat.enabled", `{"spec":{}}`)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "patched") {
		t.Errorf("named patch = %d %s", w.Code, w.Body.String())
	}
	w = doJSON(t, s, "GET", "/api/test/v1/settings", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"kind":"WidgetList"`) {
		t.Errorf("named list = %d %s", w.Code, w.Body.String())
	}
}

// recordingChecker captures the scope the authz link runs the permission
// check at, and grants access only when the check runs at the item's own
// workspace scope -- mimicking a non-admin whose only binding is
// workspace-scoped (RBAC rule 2). Any platform-scoped check is denied.
type recordingChecker struct {
	scope  string
	ws, ns int64
}

func (c *recordingChecker) CheckPermission(_ context.Context, _ int64, _, _ string, _, _ int64) (bool, error) {
	return false, nil
}
func (c *recordingChecker) CheckAnyPermission(_ context.Context, _ int64, _ []string, scope string, ws, ns int64) (bool, error) {
	c.scope, c.ws, c.ns = scope, ws, ns
	return scope == "workspace" && ws == 42, nil
}
func (c *recordingChecker) IsPlatformAdmin(_ context.Context, _ int64) (bool, error) {
	return false, nil
}
func (c *recordingChecker) GetAccessibleWorkspaceIDs(_ context.Context, _ int64) ([]int64, error) {
	return nil, nil
}
func (c *recordingChecker) GetAccessibleNamespaceIDs(_ context.Context, _ int64) ([]int64, error) {
	return nil, nil
}

// TestItemScopeResolverAuthz pins the fix for the flat scope-defining item
// route regression: GET/PUT/DELETE /workspaces/{id} must authorize at the
// workspace's own scope (ws=id), not at platform. Without ItemScopeResolver
// the check runs at ("platform",0,0) and a workspace-admin is wrongly 403'd.
func TestItemScopeResolverAuthz(t *testing.T) {
	chk := &recordingChecker{}
	s := New(Config{Checker: chk})
	Register(s, ResourceDef[Widget]{
		Group: "iam", Name: "workspaces",
		Ops: Ops[Widget]{
			Get: func(_ Ctx, id int64) (*Widget, error) {
				w := newWidget(fmt.Sprint(id), "ws")
				return &w, nil
			},
		},
		PermScope: ScopeWorkspace,
		ItemScopeResolver: func(_ context.Context, id int64) ScopeInfo {
			return ScopeInfo{Level: ScopeWorkspace, WorkspaceID: id}
		},
	})

	// Item route authorizes at ("workspace", 42); the workspace admin passes.
	w := doAsUser(t, s, 2, "GET", "/api/iam/v1/workspaces/42")
	if chk.scope != "workspace" || chk.ws != 42 {
		t.Fatalf("item authz scope = %q ws=%d, want workspace/42", chk.scope, chk.ws)
	}
	if w.Code != 200 {
		t.Fatalf("workspace-admin get own workspace = %d %s", w.Code, w.Body.String())
	}

	// The id flows into the scope (not hardcoded): a different workspace is
	// checked at ws=99 and denied.
	w = doAsUser(t, s, 2, "GET", "/api/iam/v1/workspaces/99")
	if chk.scope != "workspace" || chk.ws != 99 {
		t.Fatalf("item authz scope = %q ws=%d, want workspace/99", chk.scope, chk.ws)
	}
	if w.Code != 403 {
		t.Fatalf("workspace-admin get other workspace = %d, want 403", w.Code)
	}
}

// TestActionAnyTypedNilNoPanic pins the escape-hatch hardening: an *Any
// handler returning a typed-nil pointer ((*Widget)(nil), boxed in `any`)
// must yield 204, not panic in GetTypeMeta's nil-receiver deref.
func TestActionAnyTypedNilNoPanic(t *testing.T) {
	s := New(Config{})
	Register(s, ResourceDef[Widget]{
		Group: "test", Name: "widgets",
		Ops: Ops[Widget]{
			Get: func(_ Ctx, id int64) (*Widget, error) {
				w := newWidget(fmt.Sprint(id), "w")
				return &w, nil
			},
		},
		Actions: []ActionDef{
			ActionAny("nilify", "POST", []string{"test:widgets:update"},
				func(_ Ctx, _ []byte) (any, error) {
					var w *Widget // typed nil, returned as any
					return w, nil
				}),
		},
	})
	w := doJSON(t, s, "POST", "/api/test/v1/widgets/5/nilify", "{}")
	if w.Code != http.StatusNoContent {
		t.Fatalf("typed-nil ActionAny result = %d %s, want 204 (no panic)", w.Code, w.Body.String())
	}
}
