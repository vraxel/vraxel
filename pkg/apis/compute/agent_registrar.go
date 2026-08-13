package compute

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	apierrors "vraxel.io/vraxel/lib/api/errors"
	"vraxel.io/vraxel/pkg/apis/agentgw"
	modstore "vraxel.io/vraxel/pkg/apis/compute/store"
)

// agentHostNameInvalid matches every character not allowed in a host
// name (see nameRegexp in validation.go). Reported hostnames are
// arbitrary strings, so they are normalised before use as a name.
var agentHostNameInvalid = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

// agentHostRegistrar implements agentgw.HostRegistrar over the hosts
// table. It lives in compute because hosts is compute's table; the
// gateway reaches it only through the interface.
type agentHostRegistrar struct {
	store modstore.AgentHostStore
}

// RegisterAgentHost creates or refreshes the host row backing one agent.
//
// Idempotency has two layers. The gateway resolves ExistingHostID from
// host_agents.agent_id (itself derived from /etc/machine-id), so a
// re-running install script lands in the update branch and touches no new
// row. The create branch additionally tolerates a name collision, because
// two different machines can legitimately share a hostname (two "node-1"s
// in different customer networks) and the hosts unique index is per-scope
// on name.
func (r *agentHostRegistrar) RegisterAgentHost(ctx context.Context, spec agentgw.AgentHostSpec) (int64, error) {
	facts := modstore.AgentHostFactsInput{
		Hostname:  spec.Hostname,
		OS:        spec.OS,
		Arch:      spec.Arch,
		CPUCores:  spec.CPUCores,
		MemoryMB:  spec.MemoryMB,
		DiskGB:    spec.DiskGB,
		PrimaryIP: spec.PrimaryIP,
	}

	if spec.ExistingHostID > 0 {
		err := r.store.UpdateFacts(ctx, spec.ExistingHostID, facts)
		if err == nil {
			return spec.ExistingHostID, nil
		}
		if se := apierrors.FromDomain(err, "host"); se == nil || !apierrors.IsNotFound(se) {
			return 0, err
		}
		// The host row was deleted while host_agents still pointed at it
		// (only reachable if the FK cascade was bypassed). Fall through
		// and re-create rather than failing the agent forever.
	}

	base := agentHostName(spec.Hostname)
	for _, name := range agentHostNameCandidates(base, spec.AgentID) {
		id, err := r.store.Create(ctx, modstore.AgentHostCreateInput{
			Name:        name,
			DisplayName: spec.Hostname,
			Hostname:    spec.Hostname,
			OS:          spec.OS,
			Arch:        spec.Arch,
			CPUCores:    spec.CPUCores,
			MemoryMB:    spec.MemoryMB,
			DiskGB:      spec.DiskGB,
			Scope:       spec.Scope,
			WorkspaceID: spec.WorkspaceID,
			NamespaceID: spec.NamespaceID,
			PrimaryIP:   spec.PrimaryIP,
			CreatedBy:   spec.CreatedBy,
		})
		if err == nil {
			return id, nil
		}
		if se := apierrors.FromDomain(err, "host"); se == nil || !apierrors.IsConflict(se) {
			return 0, err
		}
	}
	return 0, fmt.Errorf("host name %q is taken and no disambiguated variant was free", base)
}

// agentHostNameCandidates lists the names to try, in order: the plain
// normalised hostname, then progressively longer agent-derived suffixes.
// Six hex digits already makes a collision between two machines sharing a
// hostname a 1-in-16-million event; twelve is the belt-and-braces step.
func agentHostNameCandidates(base, agentID string) []string {
	return []string{
		base,
		base + "-" + agentgw.NameSuffixForAgent(agentID, 6),
		base + "-" + agentgw.NameSuffixForAgent(agentID, 12),
	}
}

// agentHostName normalises a reported hostname into something that
// satisfies compute's host-name rules: alphanumerics, underscore and
// hyphen only, 3-50 characters, alphanumeric at both ends.
func agentHostName(hostname string) string {
	name := agentHostNameInvalid.ReplaceAllString(strings.TrimSpace(hostname), "-")
	name = strings.Trim(name, "-_")
	if len(name) > 40 {
		// Leave room for a suffix without breaching the 50-char ceiling.
		name = strings.Trim(name[:40], "-_")
	}
	if len(name) < 3 {
		// Degenerate hostnames ("a", "", "..") still need a usable name;
		// the agent suffix appended by the caller makes it unique.
		name = "agent-host"
	}
	return name
}
