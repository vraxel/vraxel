package agentgw

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
	apierrors "vraxel.io/vraxel/lib/api/errors"
	"vraxel.io/vraxel/lib/logger"
)

// handleRegister exchanges a join token for a long-lived agent token,
// creating or rebinding this machine's host row. POST /api/agent/v1/register.
func (h *protocolHandler) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	joinToken, ok := bearerToken(r)
	if !ok {
		http.Error(w, "missing bearer join-token", http.StatusUnauthorized)
		return
	}
	req, ok := decodeRegisterRequest(w, r)
	if !ok {
		return
	}

	// Validate the token before touching anything else, without claiming a
	// use. Doing the machine lookup first (as this did) made /register an
	// oracle for anyone with a guess at a machine id and no credential at
	// all: 409 meant "that host is registered here and its agent is live",
	// 401 meant it was not. It also spent a database round trip per
	// unauthenticated request on a public endpoint.
	tokenHash := HashToken(joinToken)
	if _, err := h.joinTokens.Peek(r.Context(), tokenHash); err != nil {
		if se := apierrors.FromDomain(err, "join token"); se != nil && apierrors.IsNotFound(se) {
			http.Error(w, "invalid, expired or exhausted join-token", http.StatusUnauthorized)
			return
		}
		logger.Warnf("agentgw register: check join token: %v", err)
		http.Error(w, "check join-token", http.StatusInternalServerError)
		return
	}

	agentID := AgentIDForMachine(req.MachineID)

	// Look the machine up BEFORE consuming the token. A rejected
	// re-registration must not burn a use: a max_uses=1 token would be
	// spent on a request that did nothing, and the operator would have to
	// mint another one to retry.
	prev, prevErr := h.agents.GetByAgentID(r.Context(), agentID)
	switch {
	case prevErr == nil && prev.Status == agentStatusOnline:
		// The agent id is derived from the machine id the caller reports,
		// so anyone holding a join token who also knows a target machine's
		// /etc/machine-id could otherwise re-register as that host: the
		// upsert bumps token_version (evicting the real agent) and hands
		// the caller a token for a host whose job vars carry that host's
		// secrets. A genuine reinstall stops the old agent first --
		// install-agent.sh does -- so it never lands here.
		logger.Warnf("agentgw register: refused re-registration of agent %s (host %d): its channel is live",
			agentID, prev.HostID)
		http.Error(w, "this machine already has a live agent; stop it before re-registering", http.StatusConflict)
		return
	case prevErr != nil:
		if se := apierrors.FromDomain(prevErr, "agent"); se == nil || !apierrors.IsNotFound(se) {
			logger.Warnf("agentgw register: lookup agent %s: %v", agentID, prevErr)
			http.Error(w, "lookup agent", http.StatusInternalServerError)
			return
		}
	}

	// Consuming first means a failed registration still burns a use.
	// That is the intended trade: the alternative (register the host,
	// then consume) lets a crash-looping agent onboard unboundedly many
	// hosts from a max_uses=1 token.
	token, err := h.joinTokens.Consume(r.Context(), tokenHash)
	if err != nil {
		if se := apierrors.FromDomain(err, "join token"); se != nil && apierrors.IsNotFound(se) {
			http.Error(w, "invalid, expired or exhausted join-token", http.StatusUnauthorized)
			return
		}
		logger.Warnf("agentgw register: consume join token: %v", err)
		http.Error(w, "consume join-token", http.StatusInternalServerError)
		return
	}

	// A known agent id means this machine has onboarded before; reuse its
	// host row rather than creating a second one.
	var existingHostID int64
	if prevErr == nil {
		existingHostID = prev.HostID
	}

	hostID, err := h.registrar.RegisterAgentHost(r.Context(), AgentHostSpec{
		ExistingHostID: existingHostID,
		AgentID:        agentID,
		Hostname:       req.Hostname,
		OS:             req.OS,
		Arch:           req.Arch,
		CPUCores:       req.CPUCores,
		MemoryMB:       req.MemoryMB,
		DiskGB:         req.DiskGB,
		PrimaryIP:      req.DefaultRouteIP,
		Scope:          token.Scope,
		WorkspaceID:    token.WorkspaceID,
		NamespaceID:    token.NamespaceID,
		// /register carries no user session, so the operator who minted
		// the join token is the traceable creator (CLAUDE.md created_by
		// rule).
		CreatedBy: token.CreatedBy,
	})
	if err != nil {
		logger.Warnf("agentgw register: register host for agent %s: %v", agentID, err)
		h.undoRegistration(r.Context(), agentID, existingHostID, 0, token.ID)
		// A rejected rebind is the caller's problem to fix (wrong token for
		// that host), everything else is ours. Neither answer carries the
		// underlying error: the peer holds nothing but a join token, and
		// constraint names and table names are not its business.
		if apierrors.IsForbidden(err) {
			http.Error(w, "this join token may not claim that host", http.StatusForbidden)
		} else {
			http.Error(w, "register host", http.StatusInternalServerError)
		}
		return
	}

	row, err := h.agents.Upsert(r.Context(), hostID, agentID, req.AgentVersion)
	if err != nil {
		logger.Warnf("agentgw register: upsert agent %s: %v", agentID, err)
		// The three writes behind /register cannot share a transaction:
		// hosts belongs to another module and reaching it through anything
		// but the interface would drag the store layer across a module
		// boundary. So the failure is compensated instead. Without this the
		// machine is stuck for good: its token is spent, its host row
		// exists but is bound to nothing, and every retry with a fresh
		// token creates yet another host, until all three candidate names
		// are taken by its own orphans and registration fails outright.
		h.undoRegistration(r.Context(), agentID, existingHostID, hostID, token.ID)
		http.Error(w, "persist agent", http.StatusInternalServerError)
		return
	}

	agentToken, err := h.signer.Issue(AgentClaims{
		AgentID:      agentID,
		HostID:       hostID,
		TokenVersion: row.TokenVersion,
		IssuedAtUnix: time.Now().Unix(),
	})
	if err != nil {
		logger.Warnf("agentgw register: issue token for agent %s: %v", agentID, err)
		http.Error(w, "issue agent token", http.StatusInternalServerError)
		return
	}

	logger.Infof("agentgw: agent %s registered as host %d (%s, %s/%s, ip=%s)",
		agentID, hostID, req.Hostname, req.OS, req.Arch, req.DefaultRouteIP)
	writeJSON(w, http.StatusOK, agenttypes.RegisterResponse{
		AgentID:    agentID,
		HostID:     hostID,
		AgentToken: agentToken,
	})
}

// undoRegistration compensates a registration that failed after the join
// token was claimed: it gives the use back and, when this request was the
// one that created the host row, deletes it again.
//
// Both steps are best-effort. If they fail the caller is no worse off
// than before they existed, which is why nothing here changes the
// response. A process killed mid-registration still leaves the residue
// they clean up -- compensation is not atomicity -- but that window is
// now a crash rather than any ordinary database error.
func (h *protocolHandler) undoRegistration(ctx context.Context, agentID string, existingHostID, createdHostID, tokenID int64) {
	if createdHostID > 0 && existingHostID == 0 {
		if err := h.registrar.UnregisterAgentHost(ctx, createdHostID); err != nil {
			logger.Warnf("agentgw register: roll back host %d for agent %s: %v", createdHostID, agentID, err)
		}
	}
	if err := h.joinTokens.Refund(ctx, tokenID); err != nil {
		logger.Warnf("agentgw register: refund join token %d: %v", tokenID, err)
	}
}

// decodeRegisterRequest reads and validates the register body.
// ok=false means a response was already written.
func decodeRegisterRequest(w http.ResponseWriter, r *http.Request) (agenttypes.RegisterRequest, bool) {
	var req agenttypes.RegisterRequest
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, agenttypes.MaxFrameBytes))
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return req, false
	}
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return req, false
	}
	if strings.TrimSpace(req.MachineID) == "" {
		http.Error(w, "machineId is required", http.StatusBadRequest)
		return req, false
	}
	if strings.TrimSpace(req.Hostname) == "" {
		http.Error(w, "hostname is required", http.StatusBadRequest)
		return req, false
	}
	return req, true
}
