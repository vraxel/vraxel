package compute

import (
	"encoding/json"
	"time"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
	apierrors "vraxel.io/vraxel/lib/api/errors"
	"vraxel.io/vraxel/lib/apiserver"
	"vraxel.io/vraxel/lib/list"
	modstore "vraxel.io/vraxel/pkg/apis/compute/store"
	"vraxel.io/vraxel/pkg/apis/shared/scope"
)

// The two runtime-inventory reads, and they are deliberately mounted
// differently.
//
// Processes is a verb on hosts, so it checks compute:hosts:get. What a
// machine runs is the same class of fact as how loaded it is, which the
// metrics verb already serves under that permission.
//
// Accounts is a nested resource with a permission tree of its own, so it
// checks compute:hosts:accounts:list. The reasoning is the one the journal
// endpoint already applies: "may read this host's details" should not
// automatically carry which accounts exist, which can log in, which are
// root and whose keys open the machine. It declares only a read, so only
// the read code is derived -- there is no way to write any of this.

// hostRuntimeOps serves both, holding the host store for the
// authorisation read they share.
type hostRuntimeOps struct {
	hosts   modstore.HostStore
	runtime modstore.HostRuntimeStore
	// stats reads the host's live workload. Nil disables the live path
	// entirely, which is what a server with no agent dialer gets.
	stats ProcessStatsBackend
}

// visible resolves the host the caller is asking about, or an error that
// is already a 404.
//
// This read IS the authorisation, exactly as it is on the metrics verb:
// it applies the scope filter, so an id belonging to another tenant fails
// here before any inventory is touched. The runtime queries filter by
// scope again -- one check would do, and two mean a future caller that
// reaches the store directly cannot leak by forgetting.
func (o hostRuntimeOps) visible(ctx apiserver.Ctx, hostID int64) (scope.Filter, error) {
	sf := scope.FromIDs(ctx.Scope.WorkspaceID, ctx.Scope.NamespaceID)
	if _, err := o.hosts.GetByID(ctx, hostID, sf); err != nil {
		return sf, domainErr(err)
	}
	return sf, nil
}

// notReported reports whether a runtime read found no row.
//
// Through apierrors rather than by comparing the store's sentinel
// directly: pgerrors is store-layer, and a handler that imports it is the
// leak scripts/check-layer-leak.sh exists to catch.
func notReported(err error) bool {
	return apierrors.IsNotFound(apierrors.FromDomain(err, "host"))
}

// +openapi:summary=查询主机进程
// +openapi:summary.workspaces.hosts=查询工作空间下主机进程
// +openapi:summary.workspaces.namespaces.hosts=查询项目下主机进程
func (o hostRuntimeOps) processes(ctx apiserver.Ctx, id int64, _ list.Query) (any, error) {
	sf, err := o.visible(ctx, id)
	if err != nil {
		return nil, err
	}
	out := &HostProcesses{}
	row, err := o.runtime.GetProcesses(ctx, id, sf)
	switch {
	case err == nil:
		// Agent JSON into jsonb and out again, decoded here only because
		// the API type is concrete for the schema generators. A decode
		// failure leaves the list empty rather than failing the request:
		// the next report overwrites it, and a detail page that renders
		// nothing is better than one that errors.
		_ = json.Unmarshal(row.Groups, &out.Groups)
		_ = json.Unmarshal(row.Units, &out.Units)
		out.ReportedAt = &row.ReportedAt
	case notReported(err):
		// The host is real and the caller may see it -- visible() just
		// said so -- it has simply never reported. An empty inventory,
		// not a 404: the host was already found, and "not found" here
		// would send somebody looking for a host that is on their screen.
		// Falls through to the live read rather than returning here: an
		// agent that has connected but not yet pushed its first inventory
		// can still answer, and refusing to ask it would show an empty
		// table next to a host that is plainly up.
	default:
		return nil, domainErr(err)
	}
	o.enrichLive(ctx, id, out)
	return out, nil
}

// enrichLive replaces the stored inventory with what the host reports
// right now, cpu and memory included.
//
// Best effort, and every failure is silent on purpose. The stored
// inventory is a complete answer to "what runs here"; the live read adds
// utilisation and freshness on top. A host whose agent is offline, on
// another replica, or slow should show its last known workload rather
// than an error page -- and an offline host is exactly when somebody
// asks what it was running.
func (o hostRuntimeOps) enrichLive(ctx apiserver.Ctx, hostID int64, out *HostProcesses) {
	if o.stats == nil {
		return
	}
	live, err := o.stats.Live(ctx, hostID)
	if err != nil || live == nil || len(live.Groups) == 0 {
		return
	}
	out.Groups = agentProcessGroupsToAPI(live.Groups)
	// Units are left as the stored report had them. The live read does
	// not carry them -- they are inventory, and asking systemd for them
	// on every page load cost five times the latency of the whole rest of
	// this call -- so overwriting here would blank a list the agent still
	// has, five minutes fresh, in exchange for nothing.
	if len(live.Units) > 0 {
		out.Units = agentSystemdUnitsToAPI(live.Units)
	}
	// Now, not when the agent last pushed. The two timestamps mean
	// different things and the UI says which it is showing.
	now := time.Now()
	out.ReportedAt = &now
	out.Live = true
}

// agentProcessGroupsToAPI converts the wire type to the API type.
//
// Hand-written rather than a re-marshal: the two structs are declared in
// different packages precisely so the agent protocol and the public API
// can move apart, and a json round trip between them would silently make
// every field name a shared contract.
func agentProcessGroupsToAPI(in []agenttypes.ProcessGroup) []HostProcessGroup {
	out := make([]HostProcessGroup, 0, len(in))
	for _, g := range in {
		row := HostProcessGroup{
			Name: g.Name, User: g.User, Count: g.Count, Unit: g.Unit,
			Container: g.Container, Exe: g.Exe, StartedAtMs: g.StartedAtMs,
			CPUPct: g.CPUPct, RSSBytes: g.RSSBytes,
		}
		for _, p := range g.Ports {
			row.Ports = append(row.Ports, HostListenPort{Proto: p.Proto, Addr: p.Addr, Port: p.Port})
		}
		out = append(out, row)
	}
	return out
}

// agentSystemdUnitsToAPI converts the wire type to the API type, on the
// same terms as agentProcessGroupsToAPI: hand-written so the agent
// protocol and the public API can move apart.
func agentSystemdUnitsToAPI(in []agenttypes.SystemdUnit) []HostSystemdUnit {
	out := make([]HostSystemdUnit, 0, len(in))
	for _, u := range in {
		out = append(out, HostSystemdUnit{
			Name: u.Name, Enabled: u.Enabled, Active: u.Active,
			Sub: u.Sub, Description: u.Description,
		})
	}
	return out
}

// +openapi:summary=查询主机账号
// +openapi:summary.workspaces.hosts=查询工作空间下主机账号
// +openapi:summary.workspaces.namespaces.hosts=查询项目下主机账号
func (o hostRuntimeOps) accounts(ctx apiserver.Ctx) (*HostAccounts, error) {
	// Parents[0] is the {hostId} segment: this resource is registered
	// under hosts, so its collection path is /hosts/{hostId}/accounts and
	// there is no item id of its own.
	if len(ctx.Parents) == 0 {
		return nil, apierrors.NewBadRequest("host id is required", nil)
	}
	hostID := ctx.Parents[0]
	sf, err := o.visible(ctx, hostID)
	if err != nil {
		return nil, err
	}
	out := &HostAccounts{}
	row, err := o.runtime.GetAccounts(ctx, hostID, sf)
	if err != nil {
		if notReported(err) {
			return out, nil
		}
		return nil, domainErr(err)
	}
	_ = json.Unmarshal(row.Users, &out.Users)
	_ = json.Unmarshal(row.Groups, &out.Groups)
	_ = json.Unmarshal(row.SudoRules, &out.SudoRules)
	// Left nil when the object holds nothing, so a host whose sshd never
	// answered has no sshd key at all rather than one full of zero
	// values -- which would read as a daemon that permits nothing.
	if len(row.SSHD) > 0 {
		var cfg HostSSHDConfig
		// Ports is the emptiness test because sshd -T always prints at
		// least one port line -- verified against a real daemon -- so its
		// absence means the object is the "{}" a host writes when its
		// sshd never answered, not a daemon with nothing to say.
		if json.Unmarshal(row.SSHD, &cfg) == nil && len(cfg.Ports) > 0 {
			out.SSHD = &cfg
		}
	}
	out.ReportedAt = &row.ReportedAt
	return out, nil
}

// HostAccountsDef registers the accounts read under hosts.
//
// SingletonGet rather than List: this is one object with three parts
// (users, groups, sudo rules), not a page of one of them, and a List
// contract would have to pick which part it returned. The permission verb
// stays "list", so the derived code is compute:hosts:accounts:list
// -- nested under the parent's tree, where a role editor groups it with
// the rest of what one may do to a host.
func HostAccountsDef(hosts modstore.HostStore, runtime modstore.HostRuntimeStore) apiserver.ResourceDef[HostAccounts] {
	o := hostRuntimeOps{hosts: hosts, runtime: runtime}
	return apiserver.ResourceDef[HostAccounts]{
		Group: "compute", Name: "accounts",
		Parent:       "hosts",
		Scopes:       apiserver.ScopeAll,
		SingletonGet: o.accounts,
	}
}
