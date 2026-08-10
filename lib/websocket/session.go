package websocket

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

// Session represents an active WebSocket session.
type Session struct {
	ID         string
	UserID     string
	Resource   string // "host", "container", "database" ...
	ResourceID string
	Label      string // display label, e.g. "root@10.0.1.5"
	CreatedAt  time.Time
	cancel     context.CancelFunc // unexported
}

// SessionManager is a generic registry for active WebSocket connections.
// It tracks sessions, supports listing/cancellation, but does NOT enforce
// concurrency limits. Callers that need per-user limits (e.g. SSH terminals)
// should check CountByResource before calling Acquire.
// It is safe for concurrent use.
type SessionManager struct {
	mu          sync.Mutex
	sessions    map[string][]*Session // key: userID
	byID        map[string]*Session   // key: sessionID
	idleTimeout time.Duration
	counter     atomic.Int64 // per-instance counter for unique session IDs
	instanceID  string       // per-process id; prefixes session IDs so they are globally unique
}

// NewSessionManager creates a SessionManager. The idleTimeout is stored for
// callers to query but is not enforced by the manager itself.
func NewSessionManager(idleTimeout time.Duration) *SessionManager {
	return &SessionManager{
		sessions:    make(map[string][]*Session),
		byID:        make(map[string]*Session),
		idleTimeout: idleTimeout,
		instanceID:  uuid.NewString(),
	}
}

// IdleTimeout returns the configured idle timeout duration.
func (m *SessionManager) IdleTimeout() time.Duration {
	return m.idleTimeout
}

// InstanceID returns this manager's per-process id. Session IDs are prefixed
// with it, so a cross-instance force-disconnect can target exactly the one
// instance that owns the session (every instance receives the broadcast; only
// the owner finds the id in its map and acts).
func (m *SessionManager) InstanceID() string { return m.instanceID }

// Acquire registers a new session for the connection. It does not enforce
// concurrency limits; callers that need limits should check CountByResource
// before calling Acquire.
//
// Force-disconnect (Cancel) ALWAYS closes conn -- the safe default the
// caller cannot forget. Handlers that need extra teardown on an admin kill
// (an interactive shell cancelling its ssh-session context so a blocked
// output pump unwinds) pass it as extra; every extra runs after the close.
// Taking the conn instead of a hand-passed CancelFunc removes the old
// footgun where a func(){} cancel made Cancel a silent no-op -- the session
// then vanished from the list on delete yet kept streaming, un-killable.
func (m *SessionManager) Acquire(conn *Conn, userID, resource, resourceID, label string, extra ...context.CancelFunc) *Session {
	m.mu.Lock()
	defer m.mu.Unlock()

	id := fmt.Sprintf("%s-%d", m.instanceID, m.counter.Add(1))
	sess := &Session{
		ID:         id,
		UserID:     userID,
		Resource:   resource,
		ResourceID: resourceID,
		Label:      label,
		CreatedAt:  time.Now(),
		cancel: func() {
			if conn != nil {
				_ = conn.Close(StatusGoingAway, "session disconnected by administrator")
			}
			for _, c := range extra {
				if c != nil {
					c()
				}
			}
		},
	}

	m.sessions[userID] = append(m.sessions[userID], sess)
	m.byID[id] = sess

	return sess
}

// Release removes a session from tracking. If the session ID does not exist,
// this is a no-op.
func (m *SessionManager) Release(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	sess, ok := m.byID[sessionID]
	if !ok {
		return
	}

	delete(m.byID, sessionID)

	userSessions := m.sessions[sess.UserID]
	for i, s := range userSessions {
		if s.ID == sessionID {
			m.sessions[sess.UserID] = append(userSessions[:i], userSessions[i+1:]...)
			break
		}
	}

	// Clean up empty user entries
	if len(m.sessions[sess.UserID]) == 0 {
		delete(m.sessions, sess.UserID)
	}
}

// Cancel calls the cancel function associated with the session for forced
// disconnect. The session remains tracked until Release is called.
// If the session ID does not exist, this is a no-op.
func (m *SessionManager) Cancel(sessionID string) {
	m.mu.Lock()
	sess, ok := m.byID[sessionID]
	m.mu.Unlock()

	if !ok {
		return
	}
	sess.cancel()
}

// Count returns the number of active sessions for the given user.
func (m *SessionManager) Count(userID string) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	return len(m.sessions[userID])
}

// CountByResource returns the number of active sessions for the given user
// and resource type. Useful for callers that enforce per-resource concurrency
// limits (e.g. max 5 SSH terminal sessions per user).
func (m *SessionManager) CountByResource(userID, resource string) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	count := 0
	for _, s := range m.sessions[userID] {
		if s.Resource == resource {
			count++
		}
	}
	return count
}

// List returns copies of all sessions for the given user. The cancel function
// is nil in the returned copies to avoid exposing internal state.
func (m *SessionManager) List(userID string) []Session {
	m.mu.Lock()
	defer m.mu.Unlock()

	userSessions := m.sessions[userID]
	if len(userSessions) == 0 {
		return nil
	}

	result := make([]Session, len(userSessions))
	for i, s := range userSessions {
		result[i] = Session{
			ID:         s.ID,
			UserID:     s.UserID,
			Resource:   s.Resource,
			ResourceID: s.ResourceID,
			Label:      s.Label,
			CreatedAt:  s.CreatedAt,
			// cancel intentionally left nil
		}
	}
	return result
}

// ListAll returns copies of all active sessions across all users. The cancel
// function is nil in the returned copies to avoid exposing internal state.
func (m *SessionManager) ListAll() []Session {
	m.mu.Lock()
	defer m.mu.Unlock()

	var result []Session
	for _, userSessions := range m.sessions {
		for _, s := range userSessions {
			result = append(result, Session{
				ID:         s.ID,
				UserID:     s.UserID,
				Resource:   s.Resource,
				ResourceID: s.ResourceID,
				Label:      s.Label,
				CreatedAt:  s.CreatedAt,
			})
		}
	}
	return result
}

// CountAll returns the total number of active sessions across all users.
func (m *SessionManager) CountAll() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	return len(m.byID)
}
