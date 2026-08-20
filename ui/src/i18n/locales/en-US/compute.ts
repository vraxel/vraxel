const compute = {
  // nav
  "nav.compute": "Compute",
  "nav.hosts": "Hosts",

  // host list
  "compute.host.title": "Hosts",
  "compute.host.subtitle":
    "Hosts managed through an agent. Specs are reported by the agent, never typed in.",
  "compute.host.searchPlaceholder": "Search name / IP / OS / workspace / project",
  "compute.host.create": "Create host",
  "compute.host.agentStatusAll": "All statuses",
  "compute.host.empty": "No hosts onboarded yet",
  "compute.host.agentStatus": "Agent",
  "compute.host.ip": "IP address",
  "compute.host.os": "OS",
  "compute.host.arch": "Arch",
  "compute.host.spec": "Spec",
  "compute.host.organization": "Organization",
  "compute.host.scopeAll": "All scopes",
  "compute.host.scopePlatform": "Platform",
  "compute.host.scopeWorkspace": "Workspace",
  "compute.host.scopeNamespace": "Project",
  "compute.host.origin": "Origin",
  "compute.host.originAll": "All origins",
  "compute.host.originAgent": "Agent",
  "compute.host.originManual": "Manual",
  "compute.host.cores": "cores",
  "compute.host.cpu": "CPU",
  "compute.host.memory": "Memory",
  "compute.host.disk": "Disk",
  "compute.host.hostname": "Hostname",
  "compute.host.agentVersion": "Agent version",

  "compute.host.metrics.title": "Metrics",
  "compute.host.metrics.load": "Load",
  "compute.host.metrics.filesystem": "Filesystem",
  "compute.host.metrics.diskIO": "Disk IO",
  "compute.host.metrics.network": "Network",
  "compute.host.metrics.used": "used",
  "compute.host.metrics.read": "read",
  "compute.host.metrics.write": "write",
  "compute.host.metrics.noData": "No data",
  "compute.host.metrics.offline": "Host is offline; historical metrics are unavailable",
  "compute.host.metrics.lastSample": "last sample",

  "compute.alertRule.title": "Alert rules",
  "compute.alertRule.subtitle":
    "Thresholds over host heartbeat snapshots, evaluated continuously on the server",
  "compute.alertRule.create": "New rule",
  "compute.alertRule.edit": "Edit rule",
  "compute.alertRule.empty": "No alert rules",
  "compute.alertRule.searchPlaceholder": "Search name or description",
  "compute.alertRule.deleteConfirm":
    'Delete alert rule "{name}"? Its firing alerts are cleared with it.',
  "compute.alertRule.condition": "Condition",
  "compute.alertRule.metric": "Metric",
  "compute.alertRule.metricAll": "All metrics",
  "compute.alertRule.op": "Compare",
  "compute.alertRule.threshold": "Threshold",
  "compute.alertRule.forSeconds": "For (seconds)",
  "compute.alertRule.forSecondsRange": "An integer between 0 and 86400",
  "compute.alertRule.forHint": "fires after {seconds}s sustained",
  "compute.alertRule.severity": "Severity",
  "compute.alertRule.severityAll": "All severities",
  "compute.alertRule.severities.info": "Info",
  "compute.alertRule.severities.warning": "Warning",
  "compute.alertRule.severities.critical": "Critical",
  "compute.alertRule.ops.gt": ">",
  "compute.alertRule.ops.ge": ">=",
  "compute.alertRule.ops.lt": "<",
  "compute.alertRule.ops.le": "<=",
  "compute.alertRule.metrics.cpu_used_pct": "CPU used %",
  "compute.alertRule.metrics.mem_used_pct": "Memory used %",
  "compute.alertRule.metrics.disk_used_pct": "Disk used %",
  "compute.alertRule.metrics.load1": "Load (1m)",
  "compute.alertRule.metrics.load5": "Load (5m)",
  "compute.alertRule.metrics.load15": "Load (15m)",
  "compute.alertRule.metrics.net_rx_bps": "Net receive B/s",
  "compute.alertRule.metrics.net_tx_bps": "Net transmit B/s",
  "compute.alertRule.enabled": "Enabled",
  "compute.alertRule.disabled": "Disabled",
  "compute.alertRule.firing": "Firing",
  "compute.alertRule.firingBadge": "{count} alerts",
  "compute.alertRule.firingOn": "{count} alerts firing",
  "compute.alertRule.firedAt": "fired at {value}",

  "compute.host.edit": "Edit host",
  "compute.host.deleteConfirm":
    'Are you sure you want to delete host "{name}"? This action cannot be undone.',
  "compute.host.deleteAgentWarning":
    "This host has an agent. Deleting the record does not stop it: the machine keeps dialling in with a credential nothing will honour again. Uninstall it there first (systemctl disable --now vr-agent), or onboard the machine again afterwards.",

  "compute.host.imageGroup": "Cloned image",
  "compute.host.imageGroupHint":
    "{count} hosts were built from this disk image (they share /etc/machine-id). They are distinct hosts, but nothing that relies on that id can tell them apart; run systemd-machine-id-setup in the template, then on the machines already cloned from it.",
  "compute.host.merge": "Merge host records",
  "compute.host.mergeHint":
    "Fold this record into the one it duplicates: the agent moves across and this record is deleted.",
  "compute.host.mergeEvidence":
    "These records came from one disk image but report different hardware. That is either one machine whose hardware was replaced, or a clone -- only you know which. Merge only if it is the same machine.",
  "compute.host.mergeInto": "Merge into",
  "compute.host.mergeConfirm": "Merge into this",
  "compute.host.mergeNoCandidates": "No other host shares this disk image.",
  "compute.host.mergeSuccess": "Merged",

  "compute.host.installAgent": "Install agent",
  "compute.host.reinstallAgent": "Reinstall agent",
  "compute.host.installAgentHint":
    "Run the command below on this host. First install, reinstall to upgrade, and recovery after a credential stops working are all the same command.",

  // host detail
  "compute.host.basicInfo": "Basic information",
  "compute.host.agentSession": "Agent session",
  "compute.host.agentId": "Agent ID",
  "compute.host.connectedAt": "Connected at",
  "compute.host.lastSeenAt": "Last heartbeat",
  "compute.host.reportedNote":
    "Everything above is reported by the agent and cannot be edited; only the display name and description are yours to change.",

  // agent status
  "compute.agent.online": "Online",
  "compute.agent.offline": "Offline",
  "compute.agent.notInstalled": "Not installed",
  "compute.agent.notInstalledHint":
    "This host has a record but no agent, so the terminal and file manager are unavailable. Install one from the host page.",
  "compute.agent.conflict": "Identity contended",
  "compute.agent.conflictHint":
    "More than one agent process is claiming this identity, almost always a disk cloned from an onboarded host. Every channel for this host is refused while it lasts; reset /etc/machine-id on the copies and onboard them again.",

  // onboarding wizard
  "compute.onboard.title": "Add Host",
  "compute.onboard.subtitle": "Choose how the host joins, then follow the steps.",
  "compute.onboard.next": "Next",
  "compute.onboard.prev": "Back",
  "compute.onboard.create": "Create Host",
  "compute.onboard.done": "Done",

  "compute.onboard.step.method": "Method",
  "compute.onboard.step.install": "Install and Join",
  "compute.onboard.step.host": "Host Details",
  "compute.onboard.step.agent": "Install Agent",

  "compute.onboard.scope.platform": "Platform",
  "compute.onboard.scope.workspace": "Workspace {name}",
  "compute.onboard.scope.namespace": "Workspace {workspace} / current project",

  "compute.onboard.method.scope": "Placed in:",
  "compute.onboard.method.scopeHint":
    "Determined by the scope you are in. To place the host elsewhere, switch scope first.",
  "compute.onboard.method.recommended": "Recommended",
  "compute.onboard.method.agent.title": "Quick join (install agent)",
  "compute.onboard.method.agent.desc":
    "Run one command on the host; its details are reported by the agent. Nothing to type, and the platform does not need a route into the host.",
  "compute.onboard.method.import.title": "Register an existing host",
  "compute.onboard.method.import.desc":
    "Create the record first. The agent can be installed now, later, or never (SSH-managed only).",
  "compute.onboard.method.import.sourceLabel": "Where the host comes from",
  "compute.onboard.method.import.manual": "Enter manually",
  "compute.onboard.method.import.cloud": "Provision from a cloud pool",
  "compute.onboard.method.import.cloudSoon": "Not yet available",

  "compute.onboard.form.name": "Name",
  "compute.onboard.form.namePlaceholder": "e.g. db-primary-01",
  "compute.onboard.form.nameHint":
    "3-50 characters: letters, digits, underscore and hyphen, starting and ending alphanumeric.",
  "compute.onboard.form.nameInvalid": "Invalid host name.",
  "compute.onboard.form.ip": "IP Address",
  "compute.onboard.form.ipHint": "Optional. Leave empty for a host managed only through an agent.",
  "compute.onboard.form.ipRequired":
    "Installing the agent requires the platform to reach this address over SSH.",
  "compute.onboard.form.ipRequiredError":
    "An IP address is required to install the agent automatically",
  "compute.onboard.form.sshPort": "SSH Port",
  "compute.onboard.form.autoInstall": "Install the agent after creating",
  "compute.onboard.form.autoInstallDesc":
    "The platform pushes and installs the agent over SSH. If it cannot connect you get the manual command instead, and the host record is unaffected.",
  "compute.onboard.form.noSpecForm":
    "No OS, CPU, memory or disk to fill in -- the agent reports those once it joins.",
  "compute.onboard.form.createFailed": "Could not create the host; try again",

  "compute.onboard.agent.hostCreated": "Host created:",
  "compute.onboard.agent.hostCreatedHint":
    "What follows is optional and can be finished later from the host page.",
  "compute.onboard.agent.skipped": "No agent installed",
  "compute.onboard.agent.skippedHint":
    "This host has a record but no control channel, so the terminal and file manager are unavailable. Install an agent or attach an SSH credential from the host page whenever you like.",
  "compute.onboard.agent.sshFailed": "Could not install automatically, switched to manual",
  "compute.onboard.agent.noCredential":
    "This host has no SSH credential attached, so the platform cannot log in to push the agent.",

  "compute.onboard.install.onceWarning":
    "The command below contains a join token. It is shown once and cannot be recovered after you leave.",
  "compute.onboard.install.boundTo": "This token is bound to host",
  "compute.onboard.install.runOnHost": "Run as root on the target host",
  "compute.onboard.install.copy": "Copy",
  "compute.onboard.install.copied": "Copied to clipboard",
  "compute.onboard.install.copyFailed": "Copy failed; select the command manually",
  "compute.onboard.install.rootHint":
    "Requires root. The script downloads the agent, verifies its hash, registers a systemd service and joins.",
  "compute.onboard.install.waiting": "Waiting for the host to join...",
  "compute.onboard.install.waitingHint": "Once the command succeeds the host appears in the list.",
  "compute.onboard.install.waitingAgent": "Waiting for the agent to come online...",
  "compute.onboard.install.waitingAgentHint":
    "Once the command succeeds the agent takes over the host you just created.",
  "compute.onboard.install.canLeave": "Safe to leave",
  "compute.onboard.install.joined": "Host joined",
  "compute.onboard.install.agentOnline": "Agent online",
  "compute.onboard.install.reportedHint": "The details above are reported by the agent.",
  "compute.onboard.install.viewHost": "View Host",
  "compute.onboard.install.mintFailed": "Could not create the join token; try again",

  // host terminal
  "compute.host.terminal.open": "Open Terminal",
  "compute.host.terminal.connecting": "Connecting...",
  "compute.host.terminal.connected": "Connected",
  "compute.host.terminal.closed": "Session ended",
  "compute.host.terminal.failed": "Could not open the terminal",
  "compute.host.terminal.idle": "Session closed after being idle",
  "compute.host.terminal.needsAgent": "This host has no agent, so it has no terminal",

  // agent identity refusal
  "compute.agent.foreignMachine": "Credential belongs to another machine",
  "compute.agent.foreignMachineHint":
    "A machine is presenting this host's credential but is not the one it was issued to, which is why this host stays offline. If this machine's hardware identity changed (VM recreated, board replaced), re-run the install command on it to re-onboard. If it is a clone, onboard it as its own host.",
}

export default compute
