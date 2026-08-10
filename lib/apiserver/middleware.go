package apiserver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"vraxel.io/vraxel/lib/audit"
	"vraxel.io/vraxel/lib/oidc"
	"vraxel.io/vraxel/lib/rest/filters"
)

// ─── authorization (route-level) ───
//
// The permission codes were derived from the ResourceDef at registration
// and baked into the route's chain. There is no URL parsing, no lookup
// misses, and therefore no fail-open path: a route either carries codes
// or was registered via Public() on purpose.

// authzMiddleware returns the authorization link for one route.
func (s *Server) authzMiddleware(m *routeMeta, next http.Handler) http.Handler {
	if s.checker == nil {
		return next // authorization disabled (dev/test), v1 parity
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Whitelisted single code: any authenticated user (v1 skipSet).
		if len(m.PermCodes) == 1 && s.skipCodes[m.PermCodes[0]] {
			next.ServeHTTP(w, r)
			return
		}

		userID, ok := oidc.UserIDFromContext(r.Context())
		if !ok {
			forbiddenError(w, "no authenticated user")
			return
		}

		// Per-route allowance hook (absorbs v1's hardcoded self-user rule
		// when iam migrates; nil everywhere else).
		if m.ExtraAllow != nil && m.ExtraAllow(r, userID) {
			next.ServeHTTP(w, r)
			return
		}

		isAdmin, err := s.checker.IsPlatformAdmin(r.Context(), userID)
		if err != nil {
			forbiddenError(w, "failed to check permissions")
			return
		}
		if isAdmin {
			next.ServeHTTP(w, r)
			return
		}

		// Platform-level workspaces/namespaces list: any authenticated
		// user is allowed, scoped by an injected AccessFilter of the IDs
		// they hold bindings for (v1 serveWorkspaceListAccessFilter).
		if m.AccessScope != "" {
			af := &filters.AccessFilter{}
			if m.AccessScope == "workspaces" {
				ids, ferr := s.checker.GetAccessibleWorkspaceIDs(r.Context(), userID)
				if ferr != nil {
					forbiddenError(w, "failed to check accessible workspaces")
					return
				}
				af.WorkspaceIDs = ids
			} else {
				ids, ferr := s.checker.GetAccessibleNamespaceIDs(r.Context(), userID)
				if ferr != nil {
					forbiddenError(w, "failed to check accessible namespaces")
					return
				}
				af.NamespaceIDs = ids
			}
			next.ServeHTTP(w, r.WithContext(filters.WithAccessFilter(r.Context(), af)))
			return
		}

		scope := m.scope(r)
		allowed, err := s.checker.CheckAnyPermission(r.Context(), userID, m.PermCodes, scope.Level.name(), scope.WorkspaceID, scope.NamespaceID)
		if err != nil {
			forbiddenError(w, "failed to check permissions")
			return
		}
		if !allowed {
			forbiddenError(w, fmt.Sprintf("access denied: requires %s", m.PermCodes[0]))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// forbiddenError reproduces v1 filters.forbiddenError byte-for-byte:
// the authorization 403 wire shape is a hand-written Status JSON with
// status:"Failure" (NOT the numeric-status StatusError shape), and the
// frontend matches on it.
func forbiddenError(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	_, _ = fmt.Fprintf(w, `{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"Forbidden","message":"%s"}`, message)
}

// ─── audit (route-level) ───
//
// Same event shape and capture rules as v1 filters.WithAudit, with
// module/resource-chain/verb coming from route metadata instead of URL
// reverse-parsing, and body redaction driven by the declarative
// Sensitive flag instead of a hardcoded verb list.

const maxBodyCapture = 64 * 1024

// auditMiddleware returns the audit link for one route.
func (s *Server) auditMiddleware(m *routeMeta, next http.Handler) http.Handler {
	if s.auditLog == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !auditable(m, r) {
			next.ServeHTTP(w, r)
			return
		}
		bodyDetail := captureRequestBody(r, m.Sensitive)
		sw := &statusWriter{ResponseWriter: w, code: http.StatusOK}
		start := time.Now()

		next.ServeHTTP(sw, r)

		event := s.buildAuditEvent(m, r, sw.code, time.Since(start))
		event.Detail = bodyDetail
		// Sensitive resources gate BOTH directions: a create response may
		// reveal a one-time secret (e.g. an api_server_key / token) that must
		// never be persisted to audit_logs in plaintext. captureRequestBody
		// already honors m.Sensitive; the response must too, or the
		// encryption-at-rest + one-time-reveal guarantee is defeated.
		if resp := sw.captured(); !m.Sensitive && json.Valid(resp) {
			event.ResponseDetail = resp
		}
		if m.Verb == "create" && sw.code >= 200 && sw.code < 300 {
			// ResourceID reads sw.buf directly (not the gated ResponseDetail),
			// so "who created which id" is still audited for Sensitive routes.
			event.ResourceID = extractCreatedID(sw.buf.Bytes())
		}
		s.auditLog.Log(event)
	})
}

// auditable mirrors v1: writes always; GETs only for interactive
// WebSocket sessions (exec/console).
func auditable(m *routeMeta, r *http.Request) bool {
	switch r.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	}
	return m.Interactive && strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
}

// captureRequestBody reads and restores the body; sensitive routes skip
// capture so secrets never land in the audit log.
func captureRequestBody(r *http.Request, sensitive bool) json.RawMessage {
	if r.Body == nil {
		return nil
	}
	switch r.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch:
	default:
		return nil
	}
	// Read at most maxBodyCapture+1 bytes for the audit copy and splice
	// the remainder back untouched. Buffering the whole body here would
	// let any authenticated client balloon server memory with a huge
	// POST before handler-level MaxBytesReader limits ever run (this
	// middleware wraps them), and the capture beyond maxBodyCapture was
	// discarded anyway.
	buf, err := io.ReadAll(io.LimitReader(r.Body, int64(maxBodyCapture)+1))
	if err != nil {
		return nil
	}
	r.Body = struct {
		io.Reader
		io.Closer
	}{io.MultiReader(bytes.NewReader(buf), r.Body), r.Body}
	if sensitive || len(buf) == 0 {
		return nil
	}
	if len(buf) > maxBodyCapture {
		// Larger than the capture window: a truncated JSON slice is
		// invalid anyway, so there is nothing auditable to keep.
		return nil
	}
	if !json.Valid(buf) {
		return nil
	}
	return buf
}

// buildAuditEvent assembles the v1-shaped event from route metadata.
func (s *Server) buildAuditEvent(m *routeMeta, r *http.Request, statusCode int, duration time.Duration) audit.Event {
	scope := m.scope(r)
	event := audit.Event{
		EventType:    "api_operation",
		Action:       m.Verb,
		ResourceType: m.Chain,
		Module:       m.Module,
		Scope:        scope.Level.name(),
		HTTPMethod:   r.Method,
		HTTPPath:     r.URL.Path,
		StatusCode:   statusCode,
		ClientIP:     audit.ClientIP(r),
		UserAgent:    r.UserAgent(),
		DurationMs:   int(duration.Milliseconds()),
		Success:      statusCode == http.StatusSwitchingProtocols || (statusCode >= 200 && statusCode < 400),
		CreatedAt:    time.Now(),
	}
	if scope.WorkspaceID > 0 {
		ws := scope.WorkspaceID
		event.WorkspaceID = &ws
	}
	if scope.NamespaceID > 0 {
		ns := scope.NamespaceID
		event.NamespaceID = &ns
	}
	if userID, ok := oidc.UserIDFromContext(r.Context()); ok {
		event.UserID = &userID
	}
	event.Username = oidc.UsernameFromContext(r.Context())
	if m.IDParam != "" {
		event.ResourceID = pathValue(r, m.IDParam)
	}
	return event
}

// extractCreatedID pulls metadata.id from a create response (v1 parity).
func extractCreatedID(body []byte) string {
	var resp struct {
		Metadata struct {
			ID string `json:"id"`
		} `json:"metadata"`
	}
	if json.Unmarshal(body, &resp) == nil && resp.Metadata.ID != "" {
		return resp.Metadata.ID
	}
	return ""
}

// statusWriter captures the status code and up to maxBodyCapture bytes
// of the response for the audit event, unwrappable for WebSocket hijack.
type statusWriter struct {
	http.ResponseWriter
	code int
	buf  bytes.Buffer
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.code = code
	sw.ResponseWriter.WriteHeader(code)
}

func (sw *statusWriter) Write(b []byte) (int, error) {
	if sw.buf.Len() < maxBodyCapture {
		sw.buf.Write(b)
	}
	return sw.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController and coder/websocket reach the
// underlying Hijacker through the middleware layer.
func (sw *statusWriter) Unwrap() http.ResponseWriter {
	return sw.ResponseWriter
}

func (sw *statusWriter) captured() []byte {
	b := sw.buf.Bytes()
	if len(b) > maxBodyCapture {
		return b[:maxBodyCapture]
	}
	return b
}
