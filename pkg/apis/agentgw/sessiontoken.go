package agentgw

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	gwstore "vraxel.io/vraxel/pkg/apis/agentgw/store"
)

// SessionTokenPrefix tags the session-token format so it is
// distinguishable on sight from an agent token (AgentTokenPrefix).
const SessionTokenPrefix = "vrs1"

// sessionTokenTTL is the exp stamped into a session token. The token's
// real lifecycle is the control channel: authSession additionally
// requires the row to be online with a matching connEpoch, so a
// reconnect invalidates the token regardless of exp. The exp is defence
// in depth: it bounds how long a leaked token is usable even while its
// channel stays up.
//
// sessionTokenRefresh is what makes that bound affordable. A control
// channel lives for days, so without renewal every agent's REST calls
// would start 401ing one TTL after connecting. Half the TTL leaves a full
// 30 minutes of validity in hand when the replacement is sent, so a
// missed frame or a slow agent is not a cliff.
const (
	sessionTokenTTL     = time.Hour
	sessionTokenRefresh = sessionTokenTTL / 2
)

// SessionClaims is the session-token payload.
//
//   - HostID scopes every REST call to one host (the ownership check of
//     design §4.4.4).
//   - ConnEpoch is host_agents.connected_at (UnixMicro) at accept time;
//     a reconnect rewrites connected_at, so the old token stops matching.
//   - Instance records the channel-holding instance for cross-instance
//     routing (Step 7); not consulted by authSession.
type SessionClaims struct {
	HostID    int64  `json:"hid"`
	Instance  string `json:"inst"`
	ConnEpoch int64  `json:"epoch"`
	ExpUnix   int64  `json:"exp"`
}

// ErrInvalidSession is returned for any malformed / mis-signed / expired
// token. The caller maps it to 401 without distinguishing the cause.
var ErrInvalidSession = errors.New("invalid session token")

// SessionTokenSigner mints and verifies session tokens.
//
// Format: "vrs1.<base64url(payload)>.<base64url(hmac-sha256)>". Same
// scheme as TokenSigner but keyed off a DIFFERENT domain separator, so a
// session token can never be confused with, or forged from, an agent
// token even though both derive from the one master key.
//
// Stateless by design: the whole point of switching away from an
// in-memory registry (design §4.1 v6) is that a session token validates
// on ANY the server instance, since agent REST requests land wherever the
// LB routes them. Signature check is pure; the connEpoch/online check is
// one host_agents read, the same cost authAgent already pays.
type SessionTokenSigner struct {
	key []byte
	now func() time.Time // injectable for deterministic expiry tests
}

// NewSessionTokenSigner derives the session-token signing key from the
// PKI master key under its own domain separator.
func NewSessionTokenSigner(masterKey []byte) *SessionTokenSigner {
	mac := hmac.New(sha256.New, masterKey)
	mac.Write([]byte("agent-session-v1"))
	return &SessionTokenSigner{key: mac.Sum(nil), now: time.Now}
}

// Issue mints a token for a host on a specific channel connection.
// connEpoch is host_agents.connected_at as UnixMicro (Registry.Add
// returns the time; the caller converts).
func (s *SessionTokenSigner) Issue(hostID int64, instance string, connEpoch int64) (string, error) {
	payload, err := json.Marshal(SessionClaims{
		HostID:    hostID,
		Instance:  instance,
		ConnEpoch: connEpoch,
		ExpUnix:   s.now().Add(sessionTokenTTL).Unix(),
	})
	if err != nil {
		return "", fmt.Errorf("marshal session claims: %w", err)
	}
	body := base64.RawURLEncoding.EncodeToString(payload)
	return SessionTokenPrefix + "." + body + "." + base64.RawURLEncoding.EncodeToString(s.sign(body)), nil
}

// Parse verifies signature + expiry and returns the claims. It does NOT
// check connEpoch against host_agents -- that needs a DB read and lives
// in authSession, keeping the crypto here pure and unit-testable.
func (s *SessionTokenSigner) Parse(token string) (*SessionClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] != SessionTokenPrefix {
		return nil, ErrInvalidSession
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !hmac.Equal(sig, s.sign(parts[1])) {
		return nil, ErrInvalidSession
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, ErrInvalidSession
	}
	var claims SessionClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, ErrInvalidSession
	}
	if claims.HostID <= 0 || claims.ExpUnix <= s.now().Unix() {
		return nil, ErrInvalidSession
	}
	return &claims, nil
}

func (s *SessionTokenSigner) sign(body string) []byte {
	mac := hmac.New(sha256.New, s.key)
	mac.Write([]byte(body))
	return mac.Sum(nil)
}

// authSession validates the bearer session token against host_agents and
// returns the host_id every REST endpoint scopes itself to. ok=false
// means a 401 has already been written. This is the ONLY accept path for
// the REST surface (bundles / vars / events / result); the 90-day agent
// token is confined to the control-channel upgrade (design §4.1).
func (h *protocolHandler) authSession(w http.ResponseWriter, r *http.Request) (int64, bool) {
	token, ok := bearerToken(r)
	if !ok {
		http.Error(w, "missing bearer session-token", http.StatusUnauthorized)
		return 0, false
	}
	claims, err := h.sessionSigner.Parse(token)
	if err != nil {
		http.Error(w, "invalid session-token", http.StatusUnauthorized)
		return 0, false
	}
	row, err := h.agents.GetByHostID(r.Context(), claims.HostID)
	if err != nil {
		http.Error(w, "unknown agent", http.StatusUnauthorized)
		return 0, false
	}
	if !sessionRowValid(claims, row) {
		http.Error(w, "session-token no longer valid", http.StatusUnauthorized)
		return 0, false
	}
	return claims.HostID, true
}

// sessionRowValid is the DB-side half of session validation, split out
// pure for testing: the channel must still be online and the token's
// connEpoch must equal the row's current connected_at (a reconnect
// rewrote it, so a token from the previous connection no longer matches).
func sessionRowValid(claims *SessionClaims, row *gwstore.AgentRow) bool {
	if row.Status != agentStatusOnline || row.ConnectedAt == nil {
		return false
	}
	return row.ConnectedAt.UnixMicro() == claims.ConnEpoch
}
