package compute

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"vraxel.io/vraxel/lib/apiserver"
	"vraxel.io/vraxel/lib/list"
	"vraxel.io/vraxel/lib/oidc"
	"vraxel.io/vraxel/lib/statushub"
	ws "vraxel.io/vraxel/lib/websocket"
	"vraxel.io/vraxel/pkg/apis/agentgw"
	modstore "vraxel.io/vraxel/pkg/apis/compute/store"
	"vraxel.io/vraxel/pkg/apis/shared/scope"
)

const defaultSSHPort = 22

type hostOps struct {
	store modstore.HostStore
}

// HostsDef declares the hosts resource at all three scopes.
//
// There is no Patch and no BatchDelete: the writable surface is two
// fields, so a partial update of it is the same request as a full one,
// and a host is deleted one at a time because deleting it detaches a
// machine that is probably still running.
func HostsDef(store modstore.HostStore, agentHosts modstore.AgentHostStore, agents agentgw.AgentStore, hub *statushub.Hub, terminals *ws.SessionManager, dialer *AgentDialerHolder, metricsBackend MetricsBackend) apiserver.ResourceDef[Host] {
	o := hostOps{store: store}
	m := hostMergeOps{hosts: store, agentHosts: agentHosts, agents: agents}
	metrics := hostMetricsOps{hosts: store, backend: metricsBackend}
	return apiserver.ResourceDef[Host]{
		Group: "compute", Name: "hosts",
		Scopes: apiserver.ScopeAll,
		Ops: apiserver.Ops[Host]{
			List:   o.list,
			Get:    o.get,
			Create: o.create,
			Update: o.update,
			Delete: o.delete,
		},
		Verbs: []apiserver.VerbDef{
			// Read-only, so it inherits compute:hosts:get and declares no
			// permission of its own.
			apiserver.Verb("image-siblings", m.imageSiblings),
			// Also read-only under compute:hosts:get -- unlike the
			// terminal, which runs commands as root and carries its own
			// permission code, reading utilisation curves is exactly what
			// "may read this host's details" should cover.
			apiserver.VerbAny("metrics", metrics.series),
		},
		Actions: []apiserver.ActionDef{
			// On the collection, and borrowing the collection's own
			// permission: watching this URL shows exactly what listing it
			// shows. A separate code would only add a way to hold
			// compute:hosts:list and still watch a page that never updates.
			apiserver.WSAction("watch", []string{"compute:hosts:list"},
				NewHostWatchHandler(hub), apiserver.OnCollection()),
			// Deleting a record is what a merge does to the record it
			// absorbs, so it borrows that permission rather than inventing
			// one. It is also the heaviest thing the action does: whoever
			// may merge may already delete either side by hand.
			apiserver.Action("merge", http.MethodPost, []string{"compute:hosts:delete"}, m.merge),
			// A code of its own, NOT compute:hosts:get. watch may borrow
			// list because it shows exactly what listing shows; a terminal
			// runs arbitrary commands as root on the machine, which is not
			// something "may read this host's details" should carry.
			//
			// MarkInteractive is what puts the upgrade in the audit log --
			// a GET reaches it no other way, and an unaudited root shell is
			// the one thing here that must never be silent.
			apiserver.WSAction("terminal", []string{"compute:hosts:terminal"},
				NewTerminalHandler(store, terminals, dialer), apiserver.MarkInteractive()),
			// Its own code, like the terminal and unlike metrics: the
			// journal carries auth logs and whatever every service prints,
			// which is more than "may read this host's details" should
			// grant -- and less than a root shell, which is why it does not
			// demand compute:hosts:terminal either. Read-only, but still a
			// privileged read, so it is audited the same way.
			apiserver.WSAction("logs", []string{"compute:hosts:logs"},
				NewHostLogsHandler(store, terminals, dialer), apiserver.MarkInteractive()),
		},
	}
}

// +openapi:summary=获取主机列表
// +openapi:summary.workspaces.hosts=获取工作空间下的主机列表
// +openapi:summary.workspaces.namespaces.hosts=获取项目下的主机列表
func (o hostOps) list(ctx apiserver.Ctx, q list.Query) (*list.Result[Host], error) {
	if q.Filters == nil {
		q.Filters = map[string]any{}
	}
	applyHostScopeFilters(&q, ctx.Scope)

	result, err := o.store.List(ctx, q)
	if err != nil {
		return nil, domainErr(err)
	}
	items := make([]Host, len(result.Items))
	for i := range result.Items {
		items[i] = hostToAPI(&result.Items[i])
	}
	return &list.Result[Host]{Items: items, TotalCount: result.TotalCount}, nil
}

// +openapi:summary=获取主机详情
// +openapi:summary.workspaces.hosts=获取工作空间下的主机详情
// +openapi:summary.workspaces.namespaces.hosts=获取项目下的主机详情
func (o hostOps) get(ctx apiserver.Ctx, id int64) (*Host, error) {
	row, err := o.store.GetByID(ctx, id, scope.FromIDs(ctx.Scope.WorkspaceID, ctx.Scope.NamespaceID))
	if err != nil {
		return nil, domainErr(err)
	}
	out := hostToAPI(row)
	return &out, nil
}

// +openapi:summary=录入主机
// +openapi:summary.workspaces.hosts=在工作空间下录入主机
// +openapi:summary.workspaces.namespaces.hosts=在项目下录入主机
//
// Creates the record only. Nothing is contacted: the host may be
// unreachable, powered off, or not yet built. Giving it an agent is a
// separate operation with its own entry points.
func (o hostOps) create(ctx apiserver.Ctx, in *Host) (*Host, error) {
	if err := validateHostCreate(in); err != nil {
		return nil, err
	}
	sf := scope.FromIDs(ctx.Scope.WorkspaceID, ctx.Scope.NamespaceID)
	sc, wsID, nsID := sf.PartsForCreate()

	port := in.Spec.SSHPort
	if port == 0 {
		port = defaultSSHPort
	}
	var createdBy *int64
	if userID, ok := oidc.UserIDFromContext(ctx); ok {
		createdBy = &userID
	}

	id, err := o.store.Create(ctx, modstore.HostCreateInput{
		Name:        strings.TrimSpace(in.Metadata.Name),
		DisplayName: in.Spec.DisplayName,
		Description: in.Spec.Description,
		Scope:       sc,
		WorkspaceID: wsID,
		NamespaceID: nsID,
		SSHPort:     port,
		PrimaryIP:   strings.TrimSpace(in.Spec.IP),
		CreatedBy:   createdBy,
	})
	if err != nil {
		return nil, domainErr(err)
	}
	return o.get(ctx, id)
}

// +openapi:summary=更新主机
// +openapi:summary.workspaces.hosts=更新工作空间下的主机
// +openapi:summary.workspaces.namespaces.hosts=更新项目下的主机
func (o hostOps) update(ctx apiserver.Ctx, id int64, in *Host) (*Host, error) {
	if err := validateHostUpdate(in); err != nil {
		return nil, err
	}
	sf := scope.FromIDs(ctx.Scope.WorkspaceID, ctx.Scope.NamespaceID)
	if err := o.store.Update(ctx, id, sf, modstore.HostUpdateInput{
		DisplayName: in.Spec.DisplayName,
		Description: in.Spec.Description,
	}); err != nil {
		return nil, domainErr(err)
	}
	return o.get(ctx, id)
}

// +openapi:summary=删除主机
// +openapi:summary.workspaces.hosts=删除工作空间下的主机
// +openapi:summary.workspaces.namespaces.hosts=删除项目下的主机
//
// Removes the record. The agent binding and any join token bound to this
// host cascade with it; the machine itself is untouched and its agent
// will fail to register until an operator onboards it again.
func (o hostOps) delete(ctx apiserver.Ctx, id int64) error {
	return domainErr(o.store.Delete(ctx, id, scope.FromIDs(ctx.Scope.WorkspaceID, ctx.Scope.NamespaceID)))
}

// applyHostScopeFilters narrows a list to the scope the URL addressed.
// Platform scope lists everything, which is what a platform URL means.
func applyHostScopeFilters(q *list.Query, s apiserver.ScopeInfo) {
	switch s.Level {
	case apiserver.ScopeNamespace:
		q.Filters["namespace_id"] = s.NamespaceID
	case apiserver.ScopeWorkspace:
		q.Filters["workspace_id"] = s.WorkspaceID
	}
}

func hostToAPI(r *modstore.HostRow) Host {
	createdAt, updatedAt := r.CreatedAt, r.UpdatedAt
	h := Host{
		Metadata: apiObjectMeta(r.ID, r.Name, &createdAt, &updatedAt),
		Spec: HostSpec{
			DisplayName:           r.DisplayName,
			Description:           r.Description,
			Hostname:              r.Hostname,
			OS:                    r.OS,
			Arch:                  r.Arch,
			CPUCores:              r.CPUCores,
			MemoryMB:              r.MemoryMB,
			DiskGB:                r.DiskGB,
			ReportedPrimaryIP:     r.ReportedPrimaryIP,
			Origin:                r.Origin,
			ConnectivityMode:      r.ConnectivityMode,
			IP:                    r.PrimaryIPOverride,
			SSHPort:               r.SSHPort,
			Scope:                 r.Scope,
			WorkspaceName:         r.WorkspaceName,
			NamespaceName:         r.NamespaceName,
			CreatedByName:         r.CreatorName,
			AgentID:               r.AgentID,
			AgentConnectedAt:      r.AgentConnectedAt,
			AgentLastSeenAt:       r.AgentLastSeenAt,
			AgentConflictAt:       r.AgentConflictAt,
			AgentForeignMachineAt: r.AgentForeignMachineAt,
			ImageGroupSize:        r.ImageGroupSize,
		},
	}
	if r.WorkspaceID != nil {
		h.Spec.WorkspaceID = strconv.FormatInt(*r.WorkspaceID, 10)
	}
	if r.NamespaceID != nil {
		h.Spec.NamespaceID = strconv.FormatInt(*r.NamespaceID, 10)
	}
	// Left nil by the join when no agent has ever bound. Flattening to
	// "offline" here would erase the difference between a host waiting
	// for an install and one whose machine stopped answering.
	if r.AgentStatus != nil {
		h.Spec.AgentStatus = *r.AgentStatus
	}
	if r.AgentVersion != nil {
		h.Spec.AgentVersion = *r.AgentVersion
	}
	if r.AgentForeignMachineUuid != nil {
		h.Spec.AgentForeignMachineUuid = *r.AgentForeignMachineUuid
	}
	h.Spec.MetricsSampledAt = r.MetricsSampledAt
	h.Spec.CPUUsedPct = r.CPUUsedPct
	h.Spec.MemUsedPct = r.MemUsedPct
	h.Spec.DiskUsedPct = r.DiskUsedPct
	h.Spec.DiskUsedPath = r.DiskUsedPath
	h.Spec.DiskUsedBytes, h.Spec.DiskTotalBytes = r.DiskUsedBytes, r.DiskTotalBytes
	h.Spec.Load1, h.Spec.Load5, h.Spec.Load15 = r.Load1, r.Load5, r.Load15
	h.Spec.NetRxBps, h.Spec.NetTxBps = r.NetRxBps, r.NetTxBps
	// The trend travelled as JSON from the agent's heartbeat into jsonb
	// and comes out as JSON again; this decode exists only because the
	// API type is a concrete array for the schema generators. A mangled
	// value (impossible short of hand-edited rows) degrades to no
	// sparkline rather than to a failed host list.
	if len(r.CPUTrend) > 0 {
		_ = json.Unmarshal(r.CPUTrend, &h.Spec.CPUTrend)
	}
	h.Spec.AlertsFiring = r.AlertsFiring
	if len(r.FiringAlerts) > 0 {
		_ = json.Unmarshal(r.FiringAlerts, &h.Spec.FiringAlerts)
	}

	h.Spec.Virtualization = r.Virtualization
	h.Spec.CPUModel = r.CPUModel
	h.Spec.CPUSockets = r.CPUSockets
	h.Spec.CPUCoresPerSocket = r.CPUCoresPerSocket
	h.Spec.CPUThreadsPerCore = r.CPUThreadsPerCore
	h.Spec.KernelVersion = r.KernelVersion
	h.Spec.OSID = r.OSID
	h.Spec.OSVersionID = r.OSVersionID
	h.Spec.SystemVendor = r.SystemVendor
	h.Spec.ProductName = r.ProductName
	h.Spec.BIOSVersion = r.BIOSVersion
	h.Spec.BIOSDate = r.BIOSDate
	h.Spec.BoardName = r.BoardName
	h.Spec.BoardSerial = r.BoardSerial
	h.Spec.ChassisType = r.ChassisType
	h.Spec.SerialNumber = r.SerialNumber
	h.Spec.AssetTag = r.AssetTag
	h.Spec.Timezone = r.Timezone
	h.Spec.DefaultGateway = r.DefaultGateway
	h.Spec.BootAt = r.BootAt
	h.Spec.FactsReportedAt = r.FactsReportedAt
	// Same round trip as CPUTrend above: agent JSON into jsonb and out
	// again, decoded here only because the API type is concrete for the
	// schema generators. Nil on the list, where the query does not join
	// host_facts at all.
	if len(r.FactsNICs) > 0 {
		_ = json.Unmarshal(r.FactsNICs, &h.Spec.NICs)
	}
	if len(r.FactsFilesystems) > 0 {
		_ = json.Unmarshal(r.FactsFilesystems, &h.Spec.Filesystems)
	}
	if len(r.FactsBlockDevices) > 0 {
		_ = json.Unmarshal(r.FactsBlockDevices, &h.Spec.BlockDevices)
	}
	return h
}
