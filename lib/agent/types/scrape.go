package types

// ScrapeTargetsPath is where the agent fetches the list of exporters to
// scrape on its own host. The list is derived from the platform's database (which
// middleware is deployed here, on which ports, with which labels), so it
// can only come from the server; the agent contributes the scraping.
const ScrapeTargetsPath = ProtocolPathPrefix + "scrape-targets"

// ScrapeTarget is one local exporter.
type ScrapeTarget struct {
	// URL is the full metrics URL, always on loopback. The agent refuses
	// anything else, on the same rule as a data-channel target: it does
	// not trust an address it is handed.
	URL string `json:"url"`
	// Labels are attached to every sample via extra_label, which VM
	// applies with override semantics -- the same as promscrape with
	// honor_labels: false. That equivalence is what makes the resulting
	// series identical to VM scraping the host directly, so dashboards
	// and PromQL need no change.
	Labels map[string]string `json:"labels,omitempty"`
}

// ScrapeTargetsResponse is the scrape-targets body.
type ScrapeTargetsResponse struct {
	Targets []ScrapeTarget `json:"targets"`
	// PushURL is the VictoriaMetrics base URL reachable FROM the host.
	// Empty disables scraping entirely.
	PushURL string `json:"pushUrl"`
	// IngestToken, when set, is sent as a bearer token on every push.
	// Carried from the first version so enabling ingest auth later needs
	// no agent upgrade (design §5.9).
	IngestToken string `json:"ingestToken,omitempty"`
	// IntervalSec overrides the 15s scrape interval. Zero uses the
	// default.
	IntervalSec int `json:"intervalSec,omitempty"`
}
