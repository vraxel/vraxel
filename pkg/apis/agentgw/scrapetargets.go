package agentgw

import (
	"encoding/json"
	"net/http"
	"strconv"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
)

// handleScrapeTargets answers the agent's periodic "what should I be
// collecting" poll (design §5.9). Session-token authenticated like every
// other agent REST endpoint.
//
// The target list is empty for now -- nothing deploys exporters onto
// hosts yet -- but the endpoint is what carries the full-tier switch:
// when a metrics push URL is configured, every agent is told to push its
// embedded collector output there, labelled with the deployment name and
// its host id. The labels come from HERE, not the agent, because they
// are identity decisions: the host id is what the chart backend filters
// on, and the server label is what lets one VictoriaMetrics ingest
// several deployments without their series colliding.
func (h *protocolHandler) handleScrapeTargets(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	hostID, ok := h.authSession(w, r)
	if !ok {
		return
	}

	resp := agenttypes.ScrapeTargetsResponse{Targets: []agenttypes.ScrapeTarget{}}
	if h.metricsPushURL != "" {
		resp.PushURL = h.metricsPushURL
		resp.NodeMetrics = &agenttypes.NodeMetricsPush{Labels: map[string]string{
			// job follows the convention a REAL node_exporter deployment
			// would use job="node" for, deliberately distinct: if a host
			// ever runs both, dashboards can tell the producers apart.
			"job":     "vraxel-agent",
			"host_id": strconv.FormatInt(hostID, 10),
			"server":  h.serverName,
		}}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
