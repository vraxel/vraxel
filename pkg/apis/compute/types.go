package compute

import (
	"time"

	apitypes "vraxel.io/vraxel/lib/api/types"
	"vraxel.io/vraxel/lib/runtime"
)

// HostSpec is what an operator sees of a host.
//
// There is no CPU / memory / disk form anywhere: those arrive from the
// agent and a hand-typed copy would be stale within a quarter. Only
// DisplayName and Description are writable; everything else is either
// reported by the machine or fixed when the record was created.
// +openapi:description=主机属性：可编辑的仅显示名称与描述，其余为 agent 上报或创建时固定。
type HostSpec struct {
	DisplayName string `json:"displayName,omitempty"`
	Description string `json:"description,omitempty"`

	// --- reported by the agent, read-only ---
	Hostname          string `json:"hostname,omitempty"`
	OS                string `json:"os,omitempty"`
	Arch              string `json:"arch,omitempty"`
	CPUCores          int32  `json:"cpuCores,omitempty"`
	MemoryMB          int64  `json:"memoryMb,omitempty"`
	DiskGB            int64  `json:"diskGb,omitempty"`
	ReportedPrimaryIP string `json:"reportedPrimaryIp,omitempty"`

	// --- set at creation, read-only afterwards ---
	// Origin is how this record came into existence: "agent" (the machine
	// onboarded itself) or "manual" (a human entered it). It never
	// changes. ConnectivityMode is how the control plane reaches the host
	// today and does change -- an imported host that installs an agent
	// later keeps origin "manual" and flips mode to "agent". Nothing may
	// infer one from the other.
	Origin           string `json:"origin,omitempty"`
	ConnectivityMode string `json:"connectivityMode,omitempty"`

	// IP is the address an operator recorded, dialable by the control
	// plane. Optional, and absent for a host reached only through its
	// agent. Distinct from ReportedPrimaryIP, which the agent observed
	// from inside the host and which may be unroutable from here.
	IP      string `json:"ip,omitempty"`
	SSHPort int32  `json:"sshPort,omitempty"`

	Scope         string `json:"scope,omitempty"`
	WorkspaceID   string `json:"workspaceId,omitempty"`
	NamespaceID   string `json:"namespaceId,omitempty"`
	WorkspaceName string `json:"workspaceName,omitempty"`
	NamespaceName string `json:"namespaceName,omitempty"`
	CreatedByName string `json:"createdByName,omitempty"`

	// --- agent session ---
	// AgentStatus is "online", "offline", or empty when no agent has ever
	// bound to this host. The empty case is not a missing value: it is
	// the state an imported host sits in until someone installs one, and
	// it is what the list renders as "not installed".
	AgentStatus      string     `json:"agentStatus,omitempty"`
	AgentID          string     `json:"agentId,omitempty"`
	AgentVersion     string     `json:"agentVersion,omitempty"`
	AgentConnectedAt *time.Time `json:"agentConnectedAt,omitempty"`
	AgentLastSeenAt  *time.Time `json:"agentLastSeenAt,omitempty"`
	// AgentConflictAt is set while two live agent processes claim this
	// host's identity, which is what a cloned disk produces. The gateway
	// refuses every channel for the host until it clears.
	AgentConflictAt *time.Time `json:"agentConflictAt,omitempty"`
	// AgentForeignMachineAt is set while a machine keeps presenting this
	// host's credential without being the machine it was issued to, and
	// AgentForeignMachineUuid is that machine's SMBIOS UUID.
	//
	// The host reads offline the whole time, and the reason is on a
	// machine nobody thought to look at -- so it is carried here rather
	// than left in a server log. Clears the moment a legitimate session
	// gets through.
	AgentForeignMachineAt   *time.Time `json:"agentForeignMachineAt,omitempty"`
	AgentForeignMachineUuid string     `json:"agentForeignMachineUuid,omitempty"`
	// ImageGroupSize is how many hosts were built from this host's disk
	// image, this one included. Above 1 means somebody cloned a machine
	// without resetting /etc/machine-id: the hosts are distinct and
	// working, but they are indistinguishable by that id everywhere else
	// it surfaces, and one of them may be a duplicate record of another.
	// 0 or 1 is the ordinary answer and the UI says nothing.
	ImageGroupSize int64 `json:"imageGroupSize,omitempty"`

	// --- latest utilisation, read-only ---
	// From the agent's most recent heartbeat: one overwritten snapshot,
	// not a series. Pointers because absence is a state -- a host whose
	// agent has never reported shows "-", not a plausible-looking 0%.
	// MetricsSampledAt is the AGENT's clock; the UI greys values whose
	// age exceeds a couple of beats rather than trusting them fresh.
	MetricsSampledAt *time.Time `json:"metricsSampledAt,omitempty"`
	CPUUsedPct       *float64   `json:"cpuUsedPct,omitempty"`
	MemUsedPct       *float64   `json:"memUsedPct,omitempty"`
	DiskUsedPct      *float64   `json:"diskUsedPct,omitempty"`
	// DiskUsedPath names the mountpoint DiskUsedPct describes -- the
	// fullest real filesystem, not necessarily /.
	DiskUsedPath string `json:"diskUsedPath,omitempty"`
	// DiskUsedBytes / DiskTotalBytes are the host's whole disk footprint,
	// summed over the same real filesystems (each device counted once).
	// This is what the list column shows: DiskUsedPct answers "is
	// anything filling up" and cannot be paired with a size, because the
	// filesystem it describes varies from beat to beat.
	DiskUsedBytes  *int64   `json:"diskUsedBytes,omitempty"`
	DiskTotalBytes *int64   `json:"diskTotalBytes,omitempty"`
	Load1          *float64 `json:"load1,omitempty"`
	Load5          *float64 `json:"load5,omitempty"`
	Load15         *float64 `json:"load15,omitempty"`
	NetRxBps       *float64 `json:"netRxBps,omitempty"`
	NetTxBps       *float64 `json:"netTxBps,omitempty"`
	// CPUTrend is the list sparkline: CPU used % over roughly the last
	// 24h in 48 half-hour buckets, oldest first; null entries are buckets
	// the agent holds nothing for.
	CPUTrend []*float64 `json:"cpuTrend,omitempty"`

	// --- threshold alerts, read-only ---
	// AlertsFiring is how many alert rules are firing on this host.
	AlertsFiring int64 `json:"alertsFiring,omitempty"`
	// FiringAlerts names them, on the detail response only -- the list
	// carries the count and nothing else.
	FiringAlerts []HostFiringAlert `json:"firingAlerts,omitempty"`

	// --- machine inventory, read-only ---
	// Reported by the agent on its own cadence, not on every beat: these
	// describe what the machine IS. The scalars come back on the list too
	// (they are hosts columns, and the fleet questions worth asking --
	// which hypervisor, which kernel -- are asked across hosts); the three
	// lists below are detail-only.
	Virtualization    string `json:"virtualization,omitempty"`
	CPUModel          string `json:"cpuModel,omitempty"`
	CPUSockets        int32  `json:"cpuSockets,omitempty"`
	CPUCoresPerSocket int32  `json:"cpuCoresPerSocket,omitempty"`
	CPUThreadsPerCore int32  `json:"cpuThreadsPerCore,omitempty"`
	KernelVersion     string `json:"kernelVersion,omitempty"`
	// OSID and OSVersionID are /etc/os-release's ID and VERSION_ID
	// ("debian", "13"), kept apart from the display string in OS: a fleet
	// question is a comparison on the parts, not a substring match on the
	// joined name.
	OSID         string `json:"osId,omitempty"`
	OSVersionID  string `json:"osVersionId,omitempty"`
	SystemVendor string `json:"systemVendor,omitempty"`
	ProductName  string `json:"productName,omitempty"`
	BIOSVersion  string `json:"biosVersion,omitempty"`
	// BIOSDate is the firmware build date as DMI spells it (MM/DD/YYYY):
	// the closest thing to a hardware age this machine can answer alone.
	BIOSDate string `json:"biosDate,omitempty"`
	// BoardName / BoardSerial describe the motherboard, not the system.
	// A board swap changes these and leaves SerialNumber alone.
	BoardName   string `json:"boardName,omitempty"`
	BoardSerial string `json:"boardSerial,omitempty"`
	// ChassisType is the SMBIOS enclosure class collapsed to one of
	// desktop / tower / laptop / server / rack / blade. Empty on a guest,
	// where every hypervisor reports "Other".
	ChassisType string `json:"chassisType,omitempty"`
	// SerialNumber is the DMI product serial: an asset identifier on
	// physical hardware, and a restatement of the SMBIOS UUID on a guest.
	// The UI shows it only when Virtualization says "physical".
	SerialNumber string `json:"serialNumber,omitempty"`
	// AssetTag is the tag burned into SMBIOS at provisioning -- the same
	// field NetBox and bk-cmdb ask an operator to type in, read from the
	// machine instead.
	AssetTag string `json:"assetTag,omitempty"`
	// Timezone is the IANA name the machine is configured with. Worth a
	// field of its own because a host in the wrong zone produces logs
	// nobody can line up against anything else.
	Timezone string `json:"timezone,omitempty"`
	// DefaultGateway is the IPv4 next hop for 0.0.0.0/0: where on the
	// network this machine sits, without reading its addresses against a
	// subnet map kept somewhere else.
	DefaultGateway string `json:"defaultGateway,omitempty"`
	// BootAt is when the machine last booted, dated by the SERVER's clock
	// from the uptime counter the agent reports -- so it stays right on a
	// host whose own clock is hours out.
	BootAt *time.Time `json:"bootAt,omitempty"`
	// KernelCmdline is what the bootloader passed the kernel: where a
	// fleet's exceptions are actually written down (mitigations=off,
	// hugepages, an IO scheduler override). A machine behaving unlike its
	// neighbours usually differs here first.
	KernelCmdline string `json:"kernelCmdline,omitempty"`
	// ClockSync is "synced", "unsynced", or empty for an agent that could
	// not ask the kernel. A wrong clock is invisible in every other
	// reading and corrupts all of them -- log ordering across hosts,
	// certificate validation, and this platform's own judgement of
	// whether a metrics sample is fresh, which is measured against the
	// agent's clock on purpose.
	ClockSync string `json:"clockSync,omitempty"`

	// Detail-only. The list does not join the table these come from,
	// which is the whole reason they live in one.
	NICs         []HostNIC         `json:"nics,omitempty"`
	Filesystems  []HostFilesystem  `json:"filesystems,omitempty"`
	BlockDevices []HostBlockDevice `json:"blockDevices,omitempty"`
	// Swaps is swap CONFIGURATION. How much is in use belongs to the
	// metrics stream: it changes every sample, and the report these
	// arrive on is sent only when its content changes.
	Swaps []HostSwap `json:"swaps,omitempty"`
	// DNS is the resolver this machine will actually use. A host that
	// resolves differently from its neighbours is a class of outage that
	// looks like an application fault for hours.
	DNS *HostDNS `json:"dns,omitempty"`
	// CPUMitigations is the kernel's verdict on each hardware
	// vulnerability it tracks, verbatim. Not folded into a boolean: "not
	// affected", "vulnerable" and "unknown, dependent on hypervisor
	// status" are three different situations.
	CPUMitigations []HostCPUMitigation `json:"cpuMitigations,omitempty"`
	// SSHHostKeys is what an ssh client pins, by fingerprint. Public
	// halves only; no key body ever leaves the machine. A rebuilt or
	// replaced machine has new ones, which is exactly the event that
	// makes every operator's client refuse to connect.
	SSHHostKeys []HostSSHKey `json:"sshHostKeys,omitempty"`
	// FactsReportedAt is when the agent last SENT an inventory, which it
	// does only when the content changed. Stale here means "nothing has
	// changed", not "nobody is looking".
	FactsReportedAt *time.Time `json:"factsReportedAt,omitempty"`
}

// HostSwap is one configured swap area.
// +openapi:description=交换区配置：仅配置，使用量属于监控指标。
type HostSwap struct {
	// Device is a partition path or a file path; Kind says which.
	Device    string `json:"device"`
	Kind      string `json:"kind,omitempty"`
	SizeBytes int64  `json:"sizeBytes,omitempty"`
	// Priority decides which area the kernel fills first: equal
	// priorities are striped, different ones are a fallback chain, so the
	// same list of devices means two different things without it.
	Priority int32 `json:"priority,omitempty"`
}

// HostDNS is the machine's resolver configuration.
// +openapi:description=主机 DNS 解析配置。
type HostDNS struct {
	Servers []string `json:"servers,omitempty"`
	// Search is what gets appended to a short name, "domain" and "search"
	// folded together -- they are two spellings of one setting.
	Search []string `json:"search,omitempty"`
}

// HostCPUMitigation is the kernel's verdict on one hardware vulnerability.
// +openapi:description=CPU 硬件漏洞缓解状态：内核原文，未做归一化。
type HostCPUMitigation struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

// HostNIC is one of the machine's network interfaces. Container and
// bridge plumbing (veth, docker0) is filtered out at the agent.
// +openapi:description=主机网卡：agent 上报，已过滤容器/网桥虚拟接口。
type HostNIC struct {
	Name string `json:"name"`
	MAC  string `json:"mac,omitempty"`
	// IPv4 / IPv6 are in CIDR form.
	IPv4 []string `json:"ipv4,omitempty"`
	IPv6 []string `json:"ipv6,omitempty"`
	// SpeedMbps is absent on a down link and on drivers that do not
	// report one, which includes most virtual NICs.
	SpeedMbps int32  `json:"speedMbps,omitempty"`
	MTU       int32  `json:"mtu,omitempty"`
	State     string `json:"state,omitempty"`
	// Duplex is "full" or "half", absent on a down link. Half duplex on a
	// server link is a negotiation failure that reads as unexplained
	// latency everywhere else.
	Duplex string `json:"duplex,omitempty"`
	// Driver is the kernel module bound to the hardware.
	Driver string `json:"driver,omitempty"`
	// Kind is "physical", "bond", "bridge" or "vlan". The last three have
	// no hardware behind them and are listed anyway, because on a machine
	// that uses them the ADDRESS is on them and not on the port beneath.
	Kind string `json:"kind,omitempty"`
	// Master is the aggregate this interface is enslaved to, empty when it
	// stands alone.
	Master string `json:"master,omitempty"`
}

// HostFilesystem is one mounted real filesystem. tmpfs and the kernel's
// bookkeeping filesystems are excluded, by the same rule that shapes the
// disk gauge -- so these rows add up to the number beside them.
// +openapi:description=主机文件系统：真实存储挂载点，已排除 tmpfs 等伪文件系统。
type HostFilesystem struct {
	Mount     string `json:"mount"`
	Device    string `json:"device,omitempty"`
	FSType    string `json:"fstype,omitempty"`
	SizeBytes int64  `json:"sizeBytes,omitempty"`
	UsedBytes int64  `json:"usedBytes,omitempty"`
	// InodesTotal / InodesUsed are absent on filesystems that allocate
	// inodes on demand (btrfs) and so have no ceiling to report. Where
	// they are present they can run out while the byte gauge still reads
	// half empty, and writes then fail on a disk that looks fine.
	InodesTotal int64 `json:"inodesTotal,omitempty"`
	InodesUsed  int64 `json:"inodesUsed,omitempty"`
	// ReadOnly is a fault indicator, not a setting: ext4 and xfs default
	// to errors=remount-ro, so a local filesystem that has gone read-only
	// is a disk the kernel gave up on with services still running on it.
	ReadOnly bool `json:"readOnly,omitempty"`
}

// HostBlockDevice is one whole disk. Partitions and removable media are
// excluded.
// +openapi:description=主机块设备：整盘，已排除分区与可移动介质。
type HostBlockDevice struct {
	Name       string `json:"name"`
	SizeBytes  int64  `json:"sizeBytes,omitempty"`
	Rotational bool   `json:"rotational,omitempty"`
	Vendor     string `json:"vendor,omitempty"`
	Model      string `json:"model,omitempty"`
	// Serial identifies the physical drive across chassis and controller
	// renumbering: "sda" is a slot, this is the disk. Absent on most
	// virtual disks, which have no identity to publish.
	Serial string `json:"serial,omitempty"`
}

// HostAlertRuleSpec is one threshold over the heartbeat snapshot.
// +openapi:description=主机告警规则：对心跳快照字段的阈值判定，服务端在心跳路径评估。
type HostAlertRuleSpec struct {
	Description string `json:"description,omitempty"`

	// --- set at creation, read-only afterwards ---
	Scope         string `json:"scope,omitempty"`
	WorkspaceID   string `json:"workspaceId,omitempty"`
	NamespaceID   string `json:"namespaceId,omitempty"`
	WorkspaceName string `json:"workspaceName,omitempty"`
	NamespaceName string `json:"namespaceName,omitempty"`

	// Metric is one of the heartbeat summary's numeric fields:
	// cpu_used_pct, mem_used_pct, disk_used_pct, load1, load5, load15,
	// net_rx_bps, net_tx_bps.
	Metric string `json:"metric"`
	// Op is gt / ge / lt / le.
	Op        string  `json:"op"`
	Threshold float64 `json:"threshold"`
	// ForSeconds is how long the breach must hold (wall clock) before
	// the alert fires. 0 on create means the 60s default.
	ForSeconds int32 `json:"forSeconds,omitempty"`
	// Severity is info / warning / critical.
	Severity string `json:"severity"`
	// Enabled defaults to true when omitted on create.
	Enabled *bool `json:"enabled,omitempty"`

	// --- read-only ---
	// FiringCount is how many hosts this rule is firing on right now.
	FiringCount   int64  `json:"firingCount,omitempty"`
	CreatedByName string `json:"createdByName,omitempty"`
}

// HostAlertRule is a threshold alert rule over host utilisation.
type HostAlertRule struct {
	runtime.TypeMeta `json:",inline"`
	Metadata         apitypes.ObjectMeta `json:"metadata"`
	Spec             HostAlertRuleSpec   `json:"spec"`
}

func (r *HostAlertRule) GetTypeMeta() *runtime.TypeMeta { return &r.TypeMeta }

// HostFiringAlert is one firing alert on a host, carried on the host
// detail so the page can say WHICH thresholds are breached, not just
// how many.
type HostFiringAlert struct {
	RuleID    string     `json:"ruleId"`
	RuleName  string     `json:"ruleName"`
	Metric    string     `json:"metric"`
	Op        string     `json:"op"`
	Threshold float64    `json:"threshold"`
	Severity  string     `json:"severity"`
	Value     float64    `json:"value"`
	Since     *time.Time `json:"since,omitempty"`
}

// HostMetrics is one windowed read of a host's chart series, the answer
// of GET /hosts/{id}:metrics. A fixed grid: FromMs plus i*StepSec
// locates bucket i, and every series carries exactly Count values.
//
// The names are the chart vocabulary (cpu.used_pct, net.rx_bps, ...),
// NOT raw metric names: values arrive derived -- percentages and
// per-second rates -- so the frontend plots what it is handed and never
// computes a rate. Which backend answered (the agent's in-memory ring,
// or VictoriaMetrics on the full tier) is invisible on purpose.
// +openapi:description=主机监控曲线：固定网格 + 派生后的图表序列，null 为该桶无数据。
type HostMetrics struct {
	FromMs  int64               `json:"fromMs"`
	StepSec int                 `json:"stepSec"`
	Count   int                 `json:"count"`
	Series  []HostMetricsSeries `json:"series"`
}

// HostMetricsSeries is one line on a chart; a null value is a bucket the
// agent holds nothing for and must render as a gap, not a zero.
type HostMetricsSeries struct {
	Name   string            `json:"name"`
	Labels map[string]string `json:"labels,omitempty"`
	Values []*float64        `json:"values"`
}

// Host is a managed machine.
// +openapi:description=主机：通过 agent 纳管或手工录入的机器
type Host struct {
	runtime.TypeMeta `json:",inline"`
	Metadata         apitypes.ObjectMeta `json:"metadata"`
	Spec             HostSpec            `json:"spec"`
}

func (h *Host) GetTypeMeta() *runtime.TypeMeta { return &h.TypeMeta }

// AgentJoinTokenSpec is a one-shot registration credential.
//
// The plaintext is returned once, by create, and never again: only its
// SHA-256 hash is stored. The resource is marked Sensitive so the audit
// log does not capture the create response body.
// +openapi:description=接入令牌属性：明文与 serverUrl 仅在创建响应中出现一次。
type AgentJoinTokenSpec struct {
	Scope       string `json:"scope,omitempty"`
	WorkspaceID string `json:"workspaceId,omitempty"`
	NamespaceID string `json:"namespaceId,omitempty"`

	// TargetHostID binds the token to a host that already exists: the
	// agent redeeming it adopts that row instead of creating one. Empty
	// for the onboarding path, where the record does not exist until the
	// agent brings it into being. A bound token is always single-use --
	// one host, one machine.
	TargetHostID   string `json:"targetHostId,omitempty"`
	TargetHostName string `json:"targetHostName,omitempty"`

	MaxUses   int32      `json:"maxUses,omitempty"`
	UsedCount int32      `json:"usedCount,omitempty"`
	TTLHours  int32      `json:"ttlHours,omitempty"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`

	// Token is the plaintext, present only in a create response.
	Token string `json:"token,omitempty"`
	// ServerURL is the address the AGENT should call home to, taken from
	// server.externalUrl. Present only in a create response.
	//
	// It is not the address the operator reached this API at. Those two
	// coincide in a plain production deployment and nowhere else: a
	// browser on a dev machine sees the vite port, a browser behind an
	// SSH tunnel sees localhost, a browser on an admin VLAN sees a name
	// the fleet cannot resolve. Pasting any of them into a host produces
	// a command that quietly points the agent at itself.
	ServerURL     string `json:"serverUrl,omitempty"`
	CreatedByName string `json:"createdByName,omitempty"`
}

// AgentJoinToken is a pending onboarding.
// +openapi:description=Agent 接入令牌：主机侧执行 install-agent.sh 时携带，用于换取长期 agent token。明文只在创建响应中返回一次，库内仅存 SHA-256 哈希。
type AgentJoinToken struct {
	runtime.TypeMeta `json:",inline"`
	Metadata         apitypes.ObjectMeta `json:"metadata"`
	Spec             AgentJoinTokenSpec  `json:"spec"`
}

func (t *AgentJoinToken) GetTypeMeta() *runtime.TypeMeta { return &t.TypeMeta }

// HostMergeRequest folds one host record into another.
type HostMergeRequest struct {
	// SourceHostID is the record to absorb and delete. Its agent, if it
	// has one, moves to the host named in the URL.
	SourceHostID string `json:"sourceHostId"`
}

// HostMergeResponse reports what the merge did.
type HostMergeResponse struct {
	// HostID is the surviving record.
	HostID string `json:"hostId"`
	// AgentMoved is true when the surviving record gained an agent from
	// the one that was absorbed.
	AgentMoved bool `json:"agentMoved"`
}

// HostProcesses is what a host is running: the response of
// GET /hosts/{id}/processes.
//
// One object rather than a paginated list, because it is not a
// collection anybody pages through -- a real machine reports a couple of
// dozen workloads -- and because the report's timestamp belongs to the
// report, not to a row inside it.
// +openapi:description=主机进程：agent 上报的工作负载快照
type HostProcesses struct {
	runtime.TypeMeta `json:",inline"`
	Groups           []HostProcessGroup `json:"groups,omitempty"`
	// Units is the supervisor's view of the same question. The process
	// list can only show what is alive, so the one thing it can never
	// show is the service that should be there and is not.
	Units []HostSystemdUnit `json:"units,omitempty"`
	// ReportedAt is when this answer was produced: the moment of the live
	// read when Live is true, and when the agent last SENT its inventory
	// when it is false. The agent stays silent while the workload sits
	// still, so an old timestamp on the stored path means "nothing has
	// changed", not "nobody is looking".
	ReportedAt *time.Time `json:"reportedAt,omitempty"`
	// Live says the host answered just now, which is also the only case
	// where cpuPct and rssBytes are populated. False means the agent is
	// unreachable and this is the last inventory it pushed -- still the
	// right answer to "what did this box run", which is the question
	// somebody asks about a host that stopped responding.
	Live bool `json:"live,omitempty"`
}

func (h *HostProcesses) GetTypeMeta() *runtime.TypeMeta { return &h.TypeMeta }

// HostProcessGroup is one workload: every process sharing an identity,
// counted. There is deliberately no PID -- see lib/agent/types.
type HostProcessGroup struct {
	// Name is the executable name as the kernel reports it, truncated to
	// 15 characters ("victoria-metric").
	Name string `json:"name"`
	// User is the effective user's name, or a bare uid when the machine
	// has no passwd entry for it -- which is every container process
	// running as a uid that exists only inside its image.
	User  string `json:"user,omitempty"`
	Count int32  `json:"count"`
	// Unit is the systemd unit this workload belongs to. Identity comes
	// from the supervisor rather than the command line, which is not
	// collected: command lines carry credentials.
	Unit string `json:"unit,omitempty"`
	// Container is true when the workload runs in a container. Its NAME
	// is not here: the cgroup carries only the container's hex id.
	Container bool `json:"container,omitempty"`
	// Ports are the sockets this workload listens on, empty for the
	// workloads that only make outbound connections.
	Ports []HostListenPort `json:"ports,omitempty"`
	// Exe is the resolved path of the running binary. It carries no
	// credentials -- unlike the command line, which is why that is not
	// collected -- and answers what Name cannot: /usr/sbin/nginx and
	// /tmp/nginx report the same name.
	Exe string `json:"exe,omitempty"`
	// StartedAtMs is when the oldest member of this group started, in
	// unix milliseconds. The oldest because a prefork server replaces
	// workers continuously while the service has been up for months.
	StartedAtMs int64 `json:"startedAtMs,omitempty"`
	// CPUPct and RSSBytes are present only when HostProcesses.Live is
	// true. They are measured on demand and never stored: both change
	// every time they are read, and the stored inventory is sent only
	// when its content changes.
	//
	// RSSBytes is the group's footprint, not the sum of its members'
	// resident sizes: processes that share a mapping -- every postgres
	// backend maps the same shared_buffers -- would otherwise have it
	// counted once each.
	CPUPct   float64 `json:"cpuPct,omitempty"`
	RSSBytes int64   `json:"rssBytes,omitempty"`
}

// HostSystemdUnit is one service unit: what the supervisor was told to
// run, and whether it is running.
// +openapi:description=systemd 服务单元：开机自启配置与当前运行状态。
type HostSystemdUnit struct {
	Name string `json:"name"`
	// Enabled is the unit file's install state ("enabled", "static",
	// "masked"). A unit is listed either because it is enabled -- so it
	// is meant to be running -- or because it failed, which matters
	// whatever its install state says.
	Enabled string `json:"enabled,omitempty"`
	// Active is the runtime state ("active", "inactive", "failed").
	// Enabled plus inactive is the finding this list exists for.
	Active string `json:"active,omitempty"`
	// Sub is systemd's finer state ("running", "exited", "dead"):
	// active/exited is normal for a oneshot and alarming for a daemon,
	// and Active alone cannot tell them apart.
	Sub         string `json:"sub,omitempty"`
	Description string `json:"description,omitempty"`
}

// HostListenPort is one listening socket.
type HostListenPort struct {
	// Proto is "tcp" or "udp". The v4/v6 split lives in Addr, where
	// 0.0.0.0 and :: are visibly different bindings.
	Proto string `json:"proto"`
	Addr  string `json:"addr,omitempty"`
	Port  int32  `json:"port"`
}

// HostAccounts is who can use a host and what they can do on it: the
// response of GET /hosts/{id}/accounts.
//
// Behind its own permission code rather than the host's get, unlike the
// process list. The reasoning is the one already applied to the journal
// endpoint: "may read this host's details" should cover how loaded it is
// and what it runs, and should not automatically cover which accounts
// exist, which of them can log in and which of them are root.
// +openapi:description=主机账号：可登录的用户、用户组与提权面
type HostAccounts struct {
	runtime.TypeMeta `json:",inline"`
	Users            []HostAccount   `json:"users,omitempty"`
	Groups           []HostUserGroup `json:"groups,omitempty"`
	// SudoRules are the verbatim grant lines of /etc/sudoers and its
	// includes. Unparsed on purpose: sudo's grammar is real, and a parser
	// that half-understands it gives a confident wrong answer about the
	// one account that matters.
	SudoRules []string `json:"sudoRules,omitempty"`
	// SSHD is the daemon's side of the same question. The lists above say
	// which accounts exist and what they hold; this says which of them
	// can actually come in over ssh, and how. Absent when sshd is not
	// installed or would not answer.
	SSHD       *HostSSHDConfig `json:"sshd,omitempty"`
	ReportedAt *time.Time      `json:"reportedAt,omitempty"`
}

// HostSSHDConfig is the ssh daemon's effective configuration.
// +openapi:description=sshd 生效配置：由 sshd -T 计算，不含 Match 块的条件覆盖。
type HostSSHDConfig struct {
	// Ports is every port the daemon listens on.
	Ports []int32 `json:"ports,omitempty"`
	// PermitRootLogin has four values, not two: "prohibit-password" is
	// the common hardened setting and is neither yes nor no.
	PermitRootLogin string `json:"permitRootLogin,omitempty"`
	PasswordAuth    bool   `json:"passwordAuth"`
	// KbdInteractiveAuth is the other password path, through PAM.
	// Reported beside PasswordAuth because turning that one off and
	// leaving this one on is a machine that still takes passwords while
	// its configuration reads as though it does not.
	KbdInteractiveAuth   bool  `json:"kbdInteractiveAuth"`
	PubkeyAuth           bool  `json:"pubkeyAuth"`
	PermitEmptyPasswords bool  `json:"permitEmptyPasswords"`
	MaxAuthTries         int32 `json:"maxAuthTries,omitempty"`
	// The four access lists, empty when unset -- which means no
	// restriction, not an empty allowlist.
	AllowUsers  []string `json:"allowUsers,omitempty"`
	AllowGroups []string `json:"allowGroups,omitempty"`
	DenyUsers   []string `json:"denyUsers,omitempty"`
	DenyGroups  []string `json:"denyGroups,omitempty"`
	// MatchBlocks counts the conditional blocks in the configuration.
	// Everything above is the GLOBAL answer: sshd -T does not evaluate
	// Match, so a host with conditional overrides has exceptions this
	// summary does not describe. Counting them says so without inventing
	// a wrong one.
	MatchBlocks int32 `json:"matchBlocks,omitempty"`
}

func (h *HostAccounts) GetTypeMeta() *runtime.TypeMeta { return &h.TypeMeta }

// HostAccount is one local account.
type HostAccount struct {
	Name string `json:"name"`
	// int64 because a uid is unsigned 32-bit: the conventional "nobody"
	// on several systems is 4294967294.
	UID   int64  `json:"uid"`
	GID   int64  `json:"gid"`
	Group string `json:"group,omitempty"`
	Home  string `json:"home,omitempty"`
	Shell string `json:"shell,omitempty"`
	// CanLogin is false for the nologin / false shells most of a passwd
	// file carries -- what separates the two or three accounts a person
	// could log into from the twenty a package manager created.
	CanLogin bool `json:"canLogin,omitempty"`
	// Password is the SHAPE of the shadow field: "set", "locked",
	// "disabled" or "empty". The hash is reduced to this word on the host
	// and never leaves it. "empty" is the one value that is a finding: an
	// account that can be logged into with no password at all.
	Password string `json:"password,omitempty"`
	// Groups are the supplementary groups naming this account.
	Groups []string `json:"groups,omitempty"`
	// Privileges names every route this account has to root: "root" (uid
	// 0), "sudo" (a sudoers rule names it), "docker-group" (can mount the
	// host filesystem into a container, which appears nowhere in sudoers)
	// or "disk-group". Empty for an ordinary account.
	Privileges []string `json:"privileges,omitempty"`
	// SSHKeys are the keys that let somebody in as this account, by
	// fingerprint. Key bodies are not collected.
	SSHKeys []HostSSHKey `json:"sshKeys,omitempty"`
	// FullName is the first GECOS field -- who this account is for.
	FullName string `json:"fullName,omitempty"`
	// PasswordChangedAtMs and ExpiresAtMs come from the shadow entry's
	// day counters, so they land on midnight UTC and no finer. Reported
	// raw: how old is too old is a policy this platform does not hold.
	PasswordChangedAtMs int64 `json:"passwordChangedAtMs,omitempty"`
	ExpiresAtMs         int64 `json:"expiresAtMs,omitempty"`
	// LastLoginAtMs is truncated to the day, because the exact value
	// changes on every login and this rides a report sent only when its
	// content changes. Zero means never logged in -- expected for a
	// service account, a finding for a person's.
	LastLoginAtMs int64 `json:"lastLoginAtMs,omitempty"`
}

// HostUserGroup is one local group.
type HostUserGroup struct {
	Name string `json:"name"`
	GID  int64  `json:"gid"`
	// Members are the SUPPLEMENTARY members from /etc/group. A user whose
	// PRIMARY group this is does not appear here, so this is not the full
	// answer to "who is in this group" -- HostAccount.Groups is.
	Members []string `json:"members,omitempty"`
}

// HostSSHKey is one authorized key, reduced to what identifies it.
type HostSSHKey struct {
	Type string `json:"type"`
	// Fingerprint is the OpenSSH SHA256 form, the same value
	// ssh-keygen -l prints.
	Fingerprint string `json:"fingerprint"`
	Comment     string `json:"comment,omitempty"`
}
