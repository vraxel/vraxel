package compute

import (
	"encoding/json"

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
	if err != nil {
		// The host is real and the caller may see it -- visible() just
		// said so -- it has simply never reported. An empty inventory,
		// not a 404: the host was already found, and "not found" here
		// would send somebody looking for a host that is on their screen.
		if notReported(err) {
			return out, nil
		}
		return nil, domainErr(err)
	}
	// Agent JSON into jsonb and out again, decoded here only because the
	// API type is concrete for the schema generators. A decode failure
	// leaves the list empty rather than failing the request: the next
	// report overwrites it, and a detail page that renders nothing is
	// better than one that errors.
	_ = json.Unmarshal(row.Groups, &out.Groups)
	out.ReportedAt = &row.ReportedAt
	return out, nil
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
