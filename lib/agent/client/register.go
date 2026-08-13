// Package client is the agent-side half of the vr-agent protocol:
// join-token registration and the persistent control channel.
//
// It lives in lib/ (not app/vr-agent) so it depends on nothing but the
// wire contract in lib/agent/types -- the binary that runs on customer
// machines must not link vraxel's business or database packages.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
)

// registerTimeout bounds one registration attempt.
const registerTimeout = 30 * time.Second

// Register exchanges a one-time join token for a long-lived agent token,
// creating or rebinding this machine's host row server-side.
//
// Bearers the one-time credential and receives the durable one.
// httpClient is the caller's shared client, carrying the TLS trust
// store; nil falls back to the default one.
func Register(ctx context.Context, httpClient *http.Client, serverURL, joinToken string, req agenttypes.RegisterRequest) (*agenttypes.RegisterResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal register request: %w", err)
	}
	cctx, cancel := context.WithTimeout(ctx, registerTimeout)
	defer cancel()

	url := strings.TrimRight(serverURL, "/") + agenttypes.ProtocolPathPrefix + "register"
	httpReq, err := http.NewRequestWithContext(cctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build register request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+joinToken)
	httpReq.Header.Set("Content-Type", "application/json")

	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("do register: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("register returned %d: %s", resp.StatusCode, truncate(raw, 200))
	}
	var out agenttypes.RegisterResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("parse register response: %w", err)
	}
	if out.AgentToken == "" || out.HostID == 0 {
		return nil, fmt.Errorf("register response missing agentToken or hostId")
	}
	return &out, nil
}

func truncate(b []byte, n int) string {
	s := strings.TrimSpace(string(b))
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
