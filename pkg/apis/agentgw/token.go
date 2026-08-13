package agentgw

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// --- join tokens ---

// joinTokenBytes is the entropy size of a join token. Same width as the
// CICD runner register-token.
const joinTokenBytes = 32

// GenerateJoinToken mints a fresh join-token plaintext. The caller stores
// only HashToken(plaintext) and hands the plaintext back to the operator
// once -- there is no recovery path afterwards.
func GenerateJoinToken() (string, error) {
	buf := make([]byte, joinTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// HashToken is the at-rest form of a join token
// (host_agent_join_tokens.token_hash). SHA-256 without a salt is correct
// here for the same reason it is in cicd: the input is 256 bits of
// machine-generated entropy, so there is no dictionary to precompute.
func HashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// --- agent tokens ---

// AgentTokenPrefix tags the token format so a future rotation to a
// different signing scheme is distinguishable on sight.
const AgentTokenPrefix = "vra1"

// AgentClaims is the payload an agent token carries. The gateway
// validates it against host_agents: agent_id must exist, host_id must
// match, and token_version must equal the row's current value (bumping
// that column is how revocation works).
//
// Scope of use: the agent token authenticates ONLY the control-channel
// upgrade (GET /api/agent/v1/channel). REST endpoints take the
// short-lived session token delivered over the established channel
// (design §4.1). Keeping the 90-day credential off the REST surface is
// what bounds the blast radius of a stolen token: it can open a channel,
// which the server sees, rather than silently fetching job vars carrying
// etcd private keys or DB root passwords.
type AgentClaims struct {
	AgentID      string `json:"aid"`
	HostID       int64  `json:"hid"`
	TokenVersion int32  `json:"ver"`
	IssuedAtUnix int64  `json:"iat"`
}

// ErrInvalidToken is returned for any malformed / mis-signed token. The
// caller maps it to 401 without distinguishing the failure mode.
var ErrInvalidToken = errors.New("invalid agent token")

// TokenSigner mints and verifies agent tokens.
//
// Format: "vra1.<base64url(payload)>.<base64url(hmac-sha256)>".
//
// A JWT library would add a dependency and a parser surface for a token
// that only this package issues and only this package reads. The signing
// key is derived from the platform's PKI master key rather than a new
// config knob: every instance already loads it at boot from the DB, so
// tokens stay valid across restarts and across horizontally scaled
// instances with zero operator action.
type TokenSigner struct {
	key []byte
}

// NewTokenSigner derives the agent-token signing key from the PKI master
// key. The HKDF-style domain separation means an agent token can never be
// confused with, or used to attack, anything else encrypted under the
// master key.
func NewTokenSigner(masterKey []byte) *TokenSigner {
	mac := hmac.New(sha256.New, masterKey)
	mac.Write([]byte("agent-token-v1"))
	return &TokenSigner{key: mac.Sum(nil)}
}

// Issue mints a token for the given claims.
func (s *TokenSigner) Issue(claims AgentClaims) (string, error) {
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshal agent claims: %w", err)
	}
	body := base64.RawURLEncoding.EncodeToString(payload)
	return AgentTokenPrefix + "." + body + "." + base64.RawURLEncoding.EncodeToString(s.sign(body)), nil
}

// Parse verifies the signature and returns the claims. It does NOT check
// token_version -- that requires a DB read and lives in the handler, so
// the crypto here stays pure and unit-testable.
//
// It also does NOT enforce a time expiry on IssuedAtUnix, deliberately.
// Adding a 90-day cutoff before the renewal path exists (Step 14) would
// mean every agent silently 401s at day 90 with no recovery but a manual
// re-register -- strictly worse than the current state, where the only
// way to invalidate a token is a token_version bump. Expiry and renewal
// land together in Step 14. IssuedAtUnix is carried now so that change
// needs no wire revision.
func (s *TokenSigner) Parse(token string) (*AgentClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] != AgentTokenPrefix {
		return nil, ErrInvalidToken
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, ErrInvalidToken
	}
	if !hmac.Equal(sig, s.sign(parts[1])) {
		return nil, ErrInvalidToken
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, ErrInvalidToken
	}
	var claims AgentClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, ErrInvalidToken
	}
	if claims.AgentID == "" || claims.HostID <= 0 {
		return nil, ErrInvalidToken
	}
	return &claims, nil
}

func (s *TokenSigner) sign(body string) []byte {
	mac := hmac.New(sha256.New, s.key)
	mac.Write([]byte(body))
	return mac.Sum(nil)
}

// --- agent identity ---

// agentIDNamespace is the UUIDv5 namespace for the agent identities. A
// fixed random UUID, generated once for this purpose.
var agentIDNamespace = uuid.MustParse("6f1d4b2e-9c3a-5f77-8d21-0a5e6b74c910")

// AgentIDForMachine derives a stable agent id from a machine identifier
// (/etc/machine-id on Linux).
//
// This is what makes re-running install-agent.sh idempotent: the second
// registration presents the same agent id, hits the ON CONFLICT
// (agent_id) branch of UpsertHostAgent, and rebinds the existing host row
// instead of creating a duplicate. Deriving it server-side (rather than
// letting the agent pick a random uuid and persist it) also survives the
// agent's state directory being wiped -- a reinstall onto the same
// machine still resolves to the same host.
func AgentIDForMachine(machineID string) string {
	return uuid.NewSHA1(agentIDNamespace, []byte(machineID)).String()
}

// NameSuffixForAgent returns a short deterministic disambiguator derived
// from an agent id. The host registrar appends it when the machine's
// hostname is already taken as a host name in the target scope.
//
// Deterministic rather than random so a re-registration that has lost its
// host binding still converges on the same candidate name instead of
// accumulating host-1, host-2, host-3 rows.
func NameSuffixForAgent(agentID string, n int) string {
	sum := sha256.Sum256([]byte(agentID))
	full := fmt.Sprintf("%x", binary.BigEndian.Uint64(sum[:8]))
	for len(full) < n {
		full += "0"
	}
	return full[:n]
}
