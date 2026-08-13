package agentgw

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
	gwstore "vraxel.io/vraxel/pkg/apis/agentgw/store"
)

// AgentProtocolPathPrefix is the URL prefix routed to the agent protocol
// handler. Defined off the wire contract so the server mount branch and
// the agent client cannot drift.
const AgentProtocolPathPrefix = agenttypes.ProtocolPathPrefix

// protocolHandler serves the machine-facing agent surface. Anonymous to
// the platform's IAM chain -- bearer credentials are validated inside each
// route:
//
//   - POST /api/agent/v1/register   join token  -> agent token         (register.go)
//   - GET  /api/agent/v1/channel     agent token -> WS control channel  (channel.go)
//
// data-channel / bundles / jobs / scrape-targets and the install script
// arrive with later slices; everything else 404s. This file is only the
// router plus the credentials/JSON helpers the routes share.
type protocolHandler struct {
	agents        gwstore.AgentStore
	joinTokens    gwstore.JoinTokenStore
	registrar     HostRegistrar
	signer        *TokenSigner
	sessionSigner *SessionTokenSigner
	registry      *Registry
	runManager    *RunManager

	// ctx bounds session goroutines to the server's lifetime. Sessions
	// cannot use the request context: it stays alive only while ServeHTTP
	// is on the stack, and the deregistration path runs after the read loop
	// unwinds.
	ctx context.Context
}

// NewProtocolHandler builds the /api/agent/v1/ handler.
func NewProtocolHandler(ctx context.Context, stores gwstore.Stores, registrar HostRegistrar, signer *TokenSigner, sessionSigner *SessionTokenSigner, registry *Registry, runManager *RunManager) http.HandlerFunc {
	h := &protocolHandler{
		agents:        stores.Agent,
		joinTokens:    stores.JoinToken,
		registrar:     registrar,
		signer:        signer,
		sessionSigner: sessionSigner,
		registry:      registry,
		runManager:    runManager,
		ctx:           ctx,
	}
	return h.serve
}

func (h *protocolHandler) serve(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, AgentProtocolPathPrefix)
	switch {
	case rest == "register":
		h.handleRegister(w, r)
	case rest == "channel":
		h.handleChannel(w, r)
	default:
		http.NotFound(w, r)
	}
}

// --- shared helpers ---

func bearerToken(r *http.Request) (string, bool) {
	auth := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(auth) <= len(prefix) || !strings.EqualFold(auth[:len(prefix)], prefix) {
		return "", false
	}
	t := strings.TrimSpace(auth[len(prefix):])
	if t == "" {
		return "", false
	}
	return t, true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
