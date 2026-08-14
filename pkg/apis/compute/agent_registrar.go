package compute

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	apierrors "vraxel.io/vraxel/lib/api/errors"
	"vraxel.io/vraxel/pkg/apis/agentgw"
	modstore "vraxel.io/vraxel/pkg/apis/compute/store"
	"vraxel.io/vraxel/pkg/apis/shared/scope"
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
//
// The second return value says which branch ran, so the gateway can roll
// back exactly the rows this request added and nothing else.
func (r *agentHostRegistrar) RegisterAgentHost(ctx context.Context, spec agentgw.AgentHostSpec) (int64, bool, error) {
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
		cur, err := r.store.GetScope(ctx, spec.ExistingHostID)
		switch {
		case err == nil:
			// The agent id is derived from the machine id in the request,
			// and a machine id is readable by anyone on that machine. So
			// possession of any valid join token plus a target's machine id
			// would otherwise be enough to rebind that target's host row --
			// taking over a platform-scope host with a namespace-scope
			// token, evicting the real agent (the upsert bumps
			// token_version) and inheriting whatever its jobs are handed.
			if !rebindAuthorised(spec, cur) {
				return 0, false, apierrors.NewForbidden(fmt.Sprintf(
					"host %d is scoped to %s; this join token cannot rebind it",
					spec.ExistingHostID, cur.Scope))
			}
			if err := r.store.UpdateFacts(ctx, spec.ExistingHostID, facts); err != nil {
				if se := apierrors.FromDomain(err, "host"); se == nil || !apierrors.IsNotFound(se) {
					return 0, false, err
				}
				break // deleted between the two reads; re-create below
			}
			return spec.ExistingHostID, false, nil
		case apierrors.IsNotFound(apierrors.FromDomain(err, "host")):
			// The host row was deleted while host_agents still pointed at it
			// (only reachable if the FK cascade was bypassed). Fall through
			// and re-create rather than failing the agent forever.
		default:
			return 0, false, err
		}
	}

	base := agentHostName(spec.Hostname)
	for _, name := range agentHostNameCandidates(base, spec.NameSeed) {
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
			return id, true, nil
		}
		if se := apierrors.FromDomain(err, "host"); se == nil || !apierrors.IsConflict(se) {
			return 0, false, err
		}
	}
	return 0, false, fmt.Errorf("host name %q is taken and no disambiguated variant was free", base)
}

// UnregisterAgentHost removes a host row whose registration failed after
// it was created.
func (r *agentHostRegistrar) UnregisterAgentHost(ctx context.Context, hostID int64) error {
	return r.store.Delete(ctx, hostID)
}

// rebindAuthorised reports whether a join token presented for spec may
// take over an existing host with scope cur.
//
// Same scope is the ordinary case: the machine is reinstalling with a
// token minted where it already lives. Platform scope is the deliberate
// exception -- it is the administrative scope, and without it an operator
// who re-scoped a host after onboarding would have no token that could
// ever re-onboard that machine again.
func rebindAuthorised(spec agentgw.AgentHostSpec, cur *modstore.AgentHostScope) bool {
	if spec.Scope == scope.Platform {
		return true
	}
	return spec.Scope == cur.Scope &&
		int64PtrEqual(spec.WorkspaceID, cur.WorkspaceID) &&
		int64PtrEqual(spec.NamespaceID, cur.NamespaceID)
}

func int64PtrEqual(a, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// agentHostNameCandidates lists the names to try, in order: the plain
// normalised hostname, then progressively longer machine-derived
// suffixes. Six hex digits already makes a collision between two machines
// sharing a hostname a 1-in-16-million event; twelve is the
// belt-and-braces step.
func agentHostNameCandidates(base, nameSeed string) []string {
	return []string{
		base,
		base + "-" + agentgw.NameSuffixForAgent(nameSeed, 6),
		base + "-" + agentgw.NameSuffixForAgent(nameSeed, 12),
	}
}

// agentHostName normalises a reported hostname into something that
// satisfies compute's host-name rules: alphanumerics, underscore and
// hyphen only, 3-50 characters, alphanumeric at both ends.
func agentHostName(hostname string) string {
	name := agentHostNameInvalid.ReplaceAllString(strings.TrimSpace(hostname), "-")
	name = strings.Trim(name, "-_")
	if len(name) > 37 {
		// Leave room for "-" + the 12-hex suffix without breaching the
		// 50-char ceiling.
		name = strings.Trim(name[:37], "-_")
	}
	if len(name) < 3 {
		// Degenerate hostnames ("a", "", "..") still need a usable name;
		// the agent suffix appended by the caller makes it unique.
		name = "agent-host"
	}
	return name
}
