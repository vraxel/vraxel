import type { Messages } from "../../types"

const compute = {
  // nav
  "nav.compute": "计算",
  "nav.hosts": "主机",

  // host list
  "compute.host.title": "主机",
  "compute.host.subtitle": "通过 agent 纳管的主机。规格信息由 agent 上报，无需手工录入。",
  "compute.host.searchPlaceholder": "搜索名称 / IP / 操作系统",
  "compute.host.create": "创建主机",
  "compute.host.agentStatusAll": "全部状态",
  "compute.host.empty": "还没有接入任何主机",
  "compute.host.agentStatus": "Agent 状态",
  "compute.host.ip": "IP 地址",
  "compute.host.os": "操作系统",
  "compute.host.arch": "架构",
  "compute.host.spec": "规格",
  "compute.host.cores": "核",
  "compute.host.cpu": "CPU",
  "compute.host.memory": "内存",
  "compute.host.disk": "磁盘",
  "compute.host.hostname": "主机名",
  "compute.host.agentVersion": "Agent 版本",

  // host detail
  "compute.host.basicInfo": "基本信息",
  "compute.host.agentSession": "Agent 会话",
  "compute.host.agentId": "Agent ID",
  "compute.host.connectedAt": "连接时间",
  "compute.host.lastSeenAt": "最近心跳",
  "compute.host.revokeAgentToken": "吊销 Agent 令牌",
  "compute.host.reportedNote":
    "以上信息由 agent 自动上报，不支持手工修改；仅显示名称与描述可编辑。",

  // agent status
  "compute.agent.online": "在线",
  "compute.agent.offline": "离线",
  "compute.agent.conflict": "身份冲突",
  "compute.agent.conflictHint":
    "有多个 agent 进程在使用同一身份，通常是从已接入主机克隆磁盘导致的。冲突期间该主机的所有连接都会被拒绝；请在克隆机上重置 /etc/machine-id 后重新接入。",

  // join tokens
  "compute.token.pending": "待使用令牌",
  "compute.token.pendingHint":
    "已发放但尚未被使用的接入令牌。令牌明文只在创建时显示一次，若已外泄请在此吊销。",
  "compute.token.nonePending": "没有待使用的令牌",
  "compute.token.unnamed": "未命名令牌",
  "compute.token.reserves": "预留主机名",
  "compute.token.uses": "已用 {used} / {max}",
  "compute.token.expiresAt": "过期于",
  "compute.token.revoke": "吊销",

  // onboarding wizard
  "compute.onboard.title": "接入主机",
  "compute.onboard.subtitle": "选择主机的接入方式，按步骤完成纳管。",
  "compute.onboard.next": "下一步",
  "compute.onboard.prev": "上一步",
  "compute.onboard.done": "完成",

  "compute.onboard.step.identity": "主机信息",
  "compute.onboard.step.install": "安装并接入",

  "compute.onboard.scope.platform": "平台",
  "compute.onboard.scope.workspace": "工作空间 {name}",
  "compute.onboard.scope.namespace": "项目 {name}",

  "compute.onboard.identity.scope": "接入后归属：",
  "compute.onboard.identity.scopeHint":
    "由当前所在范围决定。要接入到其他范围，请切换到该范围后再操作。",
  "compute.onboard.identity.auto.title": "快速接入（推荐）",
  "compute.onboard.identity.auto.desc":
    "主机名由 agent 上报的 hostname 自动生成，接入后可随时修改显示名称。",
  "compute.onboard.identity.reserved.title": "指定主机名",
  "compute.onboard.identity.reserved.desc":
    "预先指定主机名，agent 接入时使用该名称。适用于有命名规范的场景。",
  "compute.onboard.identity.hostName": "主机名",
  "compute.onboard.identity.hostNamePlaceholder": "例如 db-primary-01",
  "compute.onboard.identity.hostNameHint":
    "3-50 个字符，可用字母、数字、下划线和连字符，首尾须为字母或数字。",
  "compute.onboard.identity.nameInvalid": "主机名格式不合法。",
  "compute.onboard.identity.noSpecForm":
    "无需填写 IP、操作系统、CPU、内存、磁盘 —— 这些信息在 agent 接入时自动上报。",

  "compute.onboard.install.onceWarning": "以下命令包含接入令牌，只显示这一次，离开本页后无法找回。",
  "compute.onboard.install.reservedFor": "该令牌已预留主机名",
  "compute.onboard.install.runOnHost": "在目标主机上以 root 执行",
  "compute.onboard.install.copy": "复制",
  "compute.onboard.install.copied": "已复制到剪贴板",
  "compute.onboard.install.copyFailed": "复制失败，请手动选择命令文本",
  "compute.onboard.install.rootHint":
    "需要 root 权限。脚本会下载 agent、校验哈希、注册为 systemd 服务并完成接入。",
  "compute.onboard.install.waiting": "等待主机接入…",
  "compute.onboard.install.waitingHint": "命令执行成功后，主机会自动出现在列表中。",
  "compute.onboard.install.canLeave": "可离开本页",
  "compute.onboard.install.joined": "主机已接入",
  "compute.onboard.install.reportedHint": "以上规格信息由 agent 上报。",
  "compute.onboard.install.viewHost": "查看主机",
  "compute.onboard.install.mintFailed": "创建接入令牌失败，请重试",
} satisfies Messages

export default compute
