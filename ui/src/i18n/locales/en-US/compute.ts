const compute = {
  // nav
  "nav.compute": "Compute",
  "nav.hosts": "Hosts",

  // host list
  "compute.host.title": "Hosts",
  "compute.host.subtitle":
    "Hosts managed through an agent. Specs are reported by the agent, never typed in.",
  "compute.host.searchPlaceholder": "Search name / IP / OS",
  "compute.host.create": "Create host",
  "compute.host.agentStatusAll": "All statuses",
  "compute.host.empty": "No hosts onboarded yet",
  "compute.host.agentStatus": "Agent",
  "compute.host.ip": "IP address",
  "compute.host.os": "OS",
  "compute.host.arch": "Arch",
  "compute.host.spec": "Spec",
  "compute.host.cores": "cores",
  "compute.host.cpu": "CPU",
  "compute.host.memory": "Memory",
  "compute.host.disk": "Disk",
  "compute.host.hostname": "Hostname",
  "compute.host.agentVersion": "Agent version",

  // host detail
  "compute.host.basicInfo": "Basic information",
  "compute.host.agentSession": "Agent session",
  "compute.host.agentId": "Agent ID",
  "compute.host.connectedAt": "Connected at",
  "compute.host.lastSeenAt": "Last heartbeat",
  "compute.host.revokeAgentToken": "Revoke agent token",
  "compute.host.reportedNote":
    "Everything above is reported by the agent and cannot be edited; only the display name and description are yours to change.",

  // agent status
  "compute.agent.online": "Online",
  "compute.agent.offline": "Offline",
  "compute.agent.conflict": "Identity contended",
  "compute.agent.conflictHint":
    "More than one agent process is claiming this identity, almost always a disk cloned from an onboarded host. Every channel for this host is refused while it lasts; reset /etc/machine-id on the copies and onboard them again.",

  // join tokens
  "compute.token.pending": "Pending tokens",
  "compute.token.pendingHint":
    "Join tokens handed out but not yet redeemed. The plaintext is shown once at creation; revoke here if one leaked.",
  "compute.token.nonePending": "No pending tokens",
  "compute.token.unnamed": "Unnamed token",
  "compute.token.reserves": "Reserves host name",
  "compute.token.uses": "{used} / {max} used",
  "compute.token.expiresAt": "Expires",
  "compute.token.revoke": "Revoke",

  // onboarding wizard
  "compute.onboard.title": "Onboard host",
  "compute.onboard.subtitle": "Pick how the host joins, then follow the steps.",
  "compute.onboard.next": "Next",
  "compute.onboard.prev": "Back",
  "compute.onboard.done": "Done",

  "compute.onboard.step.identity": "Host details",
  "compute.onboard.step.install": "Install and join",

  "compute.onboard.scope.platform": "Platform",
  "compute.onboard.scope.workspace": "Workspace {name}",
  "compute.onboard.scope.namespace": "Project {name}",

  "compute.onboard.identity.scope": "Will belong to:",
  "compute.onboard.identity.scopeHint":
    "Determined by the scope you are in. To onboard into another scope, switch to it first.",
  "compute.onboard.identity.auto.title": "Quick onboarding (recommended)",
  "compute.onboard.identity.auto.desc":
    "The host name is derived from the hostname the agent reports. The display name stays editable afterwards.",
  "compute.onboard.identity.reserved.title": "Reserve a host name",
  "compute.onboard.identity.reserved.desc":
    "Fix the host name up front; the joining agent takes it. For estates with a naming convention.",
  "compute.onboard.identity.hostName": "Host name",
  "compute.onboard.identity.hostNamePlaceholder": "e.g. db-primary-01",
  "compute.onboard.identity.hostNameHint":
    "3-50 characters: letters, digits, underscore and hyphen, starting and ending alphanumeric.",
  "compute.onboard.identity.nameInvalid": "That host name is not valid.",
  "compute.onboard.identity.noSpecForm":
    "No IP, OS, CPU, memory or disk to fill in -- the agent reports all of it when it joins.",

  "compute.onboard.install.onceWarning":
    "The command below carries a join token. It is shown once and cannot be retrieved after you leave this page.",
  "compute.onboard.install.reservedFor": "This token reserves the host name",
  "compute.onboard.install.runOnHost": "Run on the target host as root",
  "compute.onboard.install.copy": "Copy",
  "compute.onboard.install.copied": "Copied to clipboard",
  "compute.onboard.install.copyFailed": "Copy failed -- select the command manually",
  "compute.onboard.install.rootHint":
    "Requires root. The script downloads the agent, verifies its digest, installs a systemd unit and joins.",
  "compute.onboard.install.waiting": "Waiting for the host to join...",
  "compute.onboard.install.waitingHint":
    "Once the command succeeds the host appears in the list on its own.",
  "compute.onboard.install.canLeave": "Safe to leave",
  "compute.onboard.install.joined": "Host joined",
  "compute.onboard.install.reportedHint": "Everything above was reported by the agent.",
  "compute.onboard.install.viewHost": "View host",
  "compute.onboard.install.mintFailed": "Could not create the join token; try again",
}

export default compute
