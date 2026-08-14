package compute

import (
	"strconv"

	"vraxel.io/vraxel/lib/list"

	apierrors "vraxel.io/vraxel/lib/api/errors"
	"vraxel.io/vraxel/lib/apiserver"
	"vraxel.io/vraxel/pkg/apis/agentgw"
	modstore "vraxel.io/vraxel/pkg/apis/compute/store"
	"vraxel.io/vraxel/pkg/apis/shared/scope"
)

// hostMergeOps folds a duplicate host record into the one it duplicates.
//
// Merging exists because the system deliberately splits rather than
// merges when it cannot tell two machines apart. A machine whose
// motherboard was replaced presents exactly the evidence a fresh clone
// does -- same disk image, different hardware id -- and the two are
// indistinguishable to anything but a human who knows what they did to
// it. Guessing wrong towards "same machine" silently points one host's
// jobs, and one host's secrets, at another machine; guessing wrong
// towards "different machine" leaves a spare row. So the system always
// leaves the spare row, and this is how an operator collects it.
type hostMergeOps struct {
	hosts  modstore.HostStore
	agents agentgw.AgentStore
}

// +openapi:summary=获取与该主机同源（同一磁盘镜像）的主机
// +openapi:summary.workspaces.hosts=获取工作空间下与该主机同源的主机
// +openapi:summary.workspaces.namespaces.hosts=获取项目下与该主机同源的主机
//
// The candidates for a merge, and the evidence for the operator deciding
// one. Read-only, so it inherits the host's own get permission rather
// than declaring anything.
//
// Sameness is /etc/machine-id: it says the two records were built from
// one disk image, which is exactly the question a merge turns on. It is
// deliberately NOT exposed as a field -- the id itself means nothing to
// an operator, while "these three hosts came from your template" does.
func (o hostMergeOps) imageSiblings(ctx apiserver.Ctx, id int64, _ list.Query) (*list.Result[Host], error) {
	sf := scope.FromIDs(ctx.Scope.WorkspaceID, ctx.Scope.NamespaceID)
	// Reading the host first is the authorisation: it applies the scope
	// filter, so an id from another tenant is a 404 before anything else
	// happens.
	if _, err := o.hosts.GetByID(ctx, id, sf); err != nil {
		return nil, domainErr(err)
	}
	self, err := o.agents.GetByHostID(ctx, id)
	if err != nil {
		if apierrors.IsNotFound(apierrors.FromDomain(err, "agent")) {
			// No agent, so no disk image to be a sibling of.
			return &list.Result[Host]{Items: []Host{}}, nil
		}
		return nil, domainErr(err)
	}
	group, err := o.agents.FindByMachineID(ctx, self.MachineID)
	if err != nil {
		return nil, domainErr(err)
	}
	items := make([]Host, 0, len(group))
	for _, row := range group {
		if row.HostID == id {
			continue
		}
		// Each sibling is re-read through the scope filter: sharing an
		// image says nothing about tenancy, and the group query runs
		// across the whole table.
		h, err := o.hosts.GetByID(ctx, row.HostID, sf)
		if err != nil {
			if apierrors.IsNotFound(apierrors.FromDomain(err, "host")) {
				continue
			}
			return nil, domainErr(err)
		}
		items = append(items, hostToAPI(h))
	}
	return &list.Result[Host]{Items: items, TotalCount: int64(len(items))}, nil
}

// +openapi:summary=合并重复的主机记录
// +openapi:summary.workspaces.hosts=合并工作空间下重复的主机记录
// +openapi:summary.workspaces.namespaces.hosts=合并项目下重复的主机记录
//
// The host in the URL survives; the one in the body is deleted. That way
// round because the survivor is the record with the history -- jobs,
// labels, the name people use -- while the row being absorbed is
// typically minutes old, created by a machine that came back looking new.
func (o hostMergeOps) merge(ctx apiserver.Ctx, in *HostMergeRequest) (*HostMergeResponse, error) {
	targetID := ctx.ID
	sourceID, err := strconv.ParseInt(in.SourceHostID, 10, 64)
	if err != nil || sourceID <= 0 {
		return nil, apierrors.NewBadRequest("sourceHostId must be a host id", nil)
	}
	if sourceID == targetID {
		return nil, apierrors.NewBadRequest("a host cannot be merged into itself", nil)
	}

	// Both ends are read through the scope filter, so a merge can only
	// ever join two records the caller can already see. Without that, an
	// id in a request body would reach across tenants.
	sf := scope.FromIDs(ctx.Scope.WorkspaceID, ctx.Scope.NamespaceID)
	target, err := o.hosts.GetByID(ctx, targetID, sf)
	if err != nil {
		return nil, domainErr(err)
	}
	source, err := o.hosts.GetByID(ctx, sourceID, sf)
	if err != nil {
		return nil, domainErr(err)
	}
	// Scope is read from the rows rather than trusted from the URL: the
	// filter above admits a platform-scope caller to both, and merging
	// across tenants would move a machine between them.
	if source.Scope != target.Scope ||
		!int64PtrEqual(source.WorkspaceID, target.WorkspaceID) ||
		!int64PtrEqual(source.NamespaceID, target.NamespaceID) {
		return nil, apierrors.NewBadRequest("these hosts are in different scopes", nil)
	}

	// A live agent on the survivor means it is not the same machine as
	// the source -- two machines cannot both be this host. Refusing is the
	// conservative half of the same trade the split makes: the operator
	// can still delete whichever record is wrong.
	targetAgent, err := o.agents.GetByHostID(ctx, targetID)
	switch {
	case err == nil && targetAgent.Status == "online":
		return nil, apierrors.NewConflictMessage(
			"this host has a live agent of its own, so it is not the same machine")
	case err != nil && !apierrors.IsNotFound(apierrors.FromDomain(err, "agent")):
		return nil, domainErr(err)
	}

	agentMoved := false
	if _, err := o.agents.GetByHostID(ctx, sourceID); err == nil {
		// Moves host_id and nothing else. The agent authenticates as an
		// agent, and its host is looked up rather than carried in its
		// token, so it survives this -- it loses only the control channel,
		// which it re-establishes on its own within seconds.
		if err := o.agents.MoveBinding(ctx, sourceID, targetID); err != nil {
			return nil, domainErr(err)
		}
		agentMoved = true
	} else if !apierrors.IsNotFound(apierrors.FromDomain(err, "agent")) {
		return nil, domainErr(err)
	}

	// Last: the source row goes, and with it (by cascade) any agent row
	// still attached. Ordered after the move so a failure leaves both
	// records intact rather than a host whose machine was deleted out
	// from under it.
	if err := o.hosts.Delete(ctx, sourceID, sf); err != nil {
		return nil, domainErr(err)
	}
	return &HostMergeResponse{HostID: strconv.FormatInt(targetID, 10), AgentMoved: agentMoved}, nil
}
