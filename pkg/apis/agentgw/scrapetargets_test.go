package agentgw

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
	gwstore "vraxel.io/vraxel/pkg/apis/agentgw/store"
)

func scrapeTargetsGet(t *testing.T, srv *httptest.Server, token string) (*http.Response, agenttypes.ScrapeTargetsResponse) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, srv.URL+agenttypes.ScrapeTargetsPath, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	var out agenttypes.ScrapeTargetsResponse
	if resp.StatusCode == http.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatalf("decode: %v", err)
		}
	}
	return resp, out
}

func TestScrapeTargets(t *testing.T) {
	const hostID = int64(42)
	signer := NewSessionTokenSigner([]byte("master"))
	epoch := time.Unix(1_700_000_000, 0)
	store := &rowStore{fakeAgentStore: &fakeAgentStore{}, row: &gwstore.AgentRow{Status: agentStatusOnline, ConnectedAt: &epoch}}

	h := &protocolHandler{agents: store, sessionSigner: signer,
		metricsPushURL: "http://vm.internal:8428", serverName: "vraxel-test"}
	srv := httptest.NewServer(http.HandlerFunc(h.serve))
	t.Cleanup(srv.Close)

	tok, err := signer.Issue(hostID, "inst", epoch.UnixMicro())
	if err != nil {
		t.Fatal(err)
	}

	resp, body := scrapeTargetsGet(t, srv, tok)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if body.PushURL != "http://vm.internal:8428" || body.NodeMetrics == nil {
		t.Fatalf("full tier must set pushUrl and the node-metrics switch: %+v", body)
	}
	// The labels are identity decisions the server owns: the host id the
	// chart backend filters on, the deployment name that keeps several
	// deployments apart in one VM, and a job distinct from a real
	// node_exporter's.
	l := body.NodeMetrics.Labels
	if l["host_id"] != "42" || l["server"] != "vraxel-test" || l["job"] != "vraxel-agent" {
		t.Fatalf("labels: %+v", l)
	}

	if resp, _ := scrapeTargetsGet(t, srv, ""); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no token must be 401, got %d", resp.StatusCode)
	}
}

func TestScrapeTargetsLiteTier(t *testing.T) {
	const hostID = int64(7)
	signer := NewSessionTokenSigner([]byte("master"))
	epoch := time.Unix(1_700_000_000, 0)
	store := &rowStore{fakeAgentStore: &fakeAgentStore{}, row: &gwstore.AgentRow{Status: agentStatusOnline, ConnectedAt: &epoch}}
	h := &protocolHandler{agents: store, sessionSigner: signer}
	srv := httptest.NewServer(http.HandlerFunc(h.serve))
	t.Cleanup(srv.Close)

	tok, _ := signer.Issue(hostID, "inst", epoch.UnixMicro())
	resp, body := scrapeTargetsGet(t, srv, tok)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if body.PushURL != "" || body.NodeMetrics != nil {
		t.Fatalf("lite tier must push nothing: %+v", body)
	}
}
