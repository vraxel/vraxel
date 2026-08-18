package apiserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"vraxel.io/vraxel/lib/audit"
	"vraxel.io/vraxel/lib/rest"
	ws "vraxel.io/vraxel/lib/websocket"
)

// blockingShell stands in for a terminal handler: it upgrades, then does
// not return until told, exactly like a session an operator is sitting in.
func blockingShell(release <-chan struct{}) rest.WebSocketHandler {
	return func(_ context.Context, _ map[string]string, conn *ws.Conn) {
		<-release
		_ = conn.Close(ws.StatusNormalClosure, "")
	}
}

// TestInteractiveAuditLogsAtUpgrade is the regression for the gap this
// route class introduces. The audit link runs after the handler, and an
// interactive handler does not return until the operator closes the
// shell -- so the record used to appear only at the end. An open root
// session was absent from audit_logs for its whole life, and absent
// forever if the process was killed while it ran.
//
// The assertion is deliberately made WHILE the session is still open.
func TestInteractiveAuditLogsAtUpgrade(t *testing.T) {
	events := make(chan audit.Event, 4)
	logger := auditLoggerFunc(func(e audit.Event) { events <- e })

	release := make(chan struct{})
	defer close(release)

	def := testDef()
	def.Actions = append(def.Actions,
		WSAction("shell", []string{"test:widgets:shell"}, blockingShell(release), MarkInteractive()))

	s := New(Config{AuditLogger: logger})
	Register(s, def)
	srv := httptest.NewServer(s)
	defer srv.Close()

	dialCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := ws.Dial(dialCtx, "ws"+srv.URL[4:]+"/api/test/v1/widgets/7/shell", nil)
	if err != nil {
		t.Fatalf("dial shell: %v", err)
	}
	defer conn.CloseNow()

	// The session is live and the handler has not returned.
	select {
	case e := <-events:
		if e.Action != "shell" {
			t.Fatalf("audit action = %q, want shell", e.Action)
		}
		if e.ResourceID != "7" {
			t.Fatalf("audit resource id = %q, want 7", e.ResourceID)
		}
		if e.StatusCode != http.StatusSwitchingProtocols {
			t.Fatalf("audit status = %d, want 101", e.StatusCode)
		}
		if !e.Success {
			t.Fatal("a successful upgrade was audited as a failure")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("an open interactive session produced no audit record")
	}

	// And exactly one: the post-handler write must not add a second row,
	// or every session an auditor counts is counted twice.
	select {
	case e := <-events:
		t.Fatalf("a second audit event for one session: %+v", e)
	case <-time.After(200 * time.Millisecond):
	}
}

// TestInteractiveAuditLogsAFailedUpgrade keeps the other half: a request
// that never becomes a session still has to be recorded, and with the
// status that says so.
func TestInteractiveAuditLogsAFailedUpgrade(t *testing.T) {
	var got []audit.Event
	logger := auditLoggerFunc(func(e audit.Event) { got = append(got, e) })

	def := testDef()
	def.Actions = append(def.Actions,
		WSAction("shell", []string{"test:widgets:shell"},
			func(_ context.Context, _ map[string]string, conn *ws.Conn) {
				_ = conn.Close(ws.StatusNormalClosure, "")
			}, MarkInteractive()))

	s := New(Config{AuditLogger: logger})
	Register(s, def)

	// A GET carrying the Upgrade header but no WebSocket handshake keys:
	// auditable() counts it, the upgrade fails, so the 101 branch must not
	// swallow it.
	r := httptest.NewRequest("GET", "/api/test/v1/widgets/7/shell", nil)
	r.Header.Set("Upgrade", "websocket")
	s.ServeHTTP(httptest.NewRecorder(), r)

	if len(got) != 1 {
		t.Fatalf("audit events = %d, want 1", len(got))
	}
	if got[0].StatusCode == http.StatusSwitchingProtocols {
		t.Fatalf("a failed upgrade was audited as 101")
	}
}
