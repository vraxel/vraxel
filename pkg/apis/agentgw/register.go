package agentgw

import (
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
	token, err := h.joinTokens.Consume(r.Context(), HashToken(joinToken))
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
		http.Error(w, "register host: "+err.Error(), http.StatusInternalServerError)
		return
	}

	row, err := h.agents.Upsert(r.Context(), hostID, agentID, req.AgentVersion)
	if err != nil {
		logger.Warnf("agentgw register: upsert agent %s: %v", agentID, err)
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
