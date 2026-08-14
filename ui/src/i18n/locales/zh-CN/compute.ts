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

  "compute.host.edit": "编辑主机",
  "compute.host.deleteConfirm": "确定要删除主机「{name}」吗？此操作不可撤销。",
  "compute.host.deleteAgentWarning":
    "这台主机已接入 Agent。删除档案不会停止机器上的 Agent，它会一直用已失效的凭证重连。请先在机器上卸载（systemctl disable --now vr-agent），或删除后重新接入。",

  "compute.host.imageGroup": "同源镜像",
  "compute.host.imageGroupHint":
    "有 {count} 台主机由同一份磁盘镜像克隆而来（/etc/machine-id 相同）。它们是各自独立的主机，但在依赖该 id 的地方无法区分；请在模板里执行 systemd-machine-id-setup，再在已克隆的机器上重置。",
  "compute.host.merge": "合并主机记录",
  "compute.host.mergeHint": "把这条记录并入它重复的那条：agent 迁过去，本条删除。",
  "compute.host.mergeEvidence":
    "这两条记录来自同一份磁盘镜像，但硬件标识不同。可能是同一台机器换了主板/整机迁移，也可能是一台克隆机——只有你知道是哪种。确认是同一台再合并。",
  "compute.host.mergeInto": "并入哪条记录",
  "compute.host.mergeConfirm": "并入这条",
  "compute.host.mergeNoCandidates": "没有找到同源的其他主机记录。",
  "compute.host.mergeSuccess": "已合并",

  "compute.host.installAgent": "安装 Agent",
  "compute.host.reinstallAgent": "重装 Agent",
  "compute.host.installAgentHint":
    "在这台主机上执行下面的命令。首次安装、重装升级、以及凭证失效后的重新接入，都是同一条命令。",

  // host detail
  "compute.host.basicInfo": "基本信息",
  "compute.host.agentSession": "Agent 会话",
  "compute.host.agentId": "Agent ID",
  "compute.host.connectedAt": "连接时间",
  "compute.host.lastSeenAt": "最近心跳",
  "compute.host.reportedNote":
    "以上信息由 agent 自动上报，不支持手工修改；仅显示名称与描述可编辑。",

  // agent status
  "compute.agent.online": "在线",
  "compute.agent.offline": "离线",
  "compute.agent.notInstalled": "未安装",
  "compute.agent.notInstalledHint":
    "这台主机只有档案，还没有安装 Agent，终端、文件管理等功能不可用。可在主机详情页安装。",
  "compute.agent.conflict": "身份冲突",
  "compute.agent.conflictHint":
    "有多个 agent 进程在使用同一身份，通常是从已接入主机克隆磁盘导致的。冲突期间该主机的所有连接都会被拒绝；请在克隆机上重置 /etc/machine-id 后重新接入。",

  // onboarding wizard
  "compute.onboard.title": "创建主机",
  "compute.onboard.subtitle": "选择主机的接入方式，按步骤完成纳管。",
  "compute.onboard.next": "下一步",
  "compute.onboard.prev": "上一步",
  "compute.onboard.create": "创建主机",
  "compute.onboard.done": "完成",

  "compute.onboard.step.method": "接入方式",
  "compute.onboard.step.install": "安装并接入",
  "compute.onboard.step.host": "主机信息",
  "compute.onboard.step.agent": "安装 Agent",

  "compute.onboard.scope.platform": "平台",
  "compute.onboard.scope.workspace": "工作空间 {name}",
  "compute.onboard.scope.namespace": "工作空间 {workspace} / 当前项目",

  "compute.onboard.method.scope": "接入后归属：",
  "compute.onboard.method.scopeHint":
    "由当前所在范围决定。要接入到其他范围，请切换到该范围后再操作。",
  "compute.onboard.method.recommended": "推荐",
  "compute.onboard.method.agent.title": "快速接入（安装 Agent）",
  "compute.onboard.method.agent.desc":
    "在主机上执行一条命令完成接入，主机信息由 agent 自动上报。无需填写地址，也不要求平台能访问到这台主机。",
  "compute.onboard.method.import.title": "录入已有主机",
  "compute.onboard.method.import.desc":
    "先建立主机档案，Agent 可以现在装、稍后装，或者不装（仅通过 SSH 管理）。",
  "compute.onboard.method.import.sourceLabel": "主机来源",
  "compute.onboard.method.import.manual": "手动导入",
  "compute.onboard.method.import.cloud": "从云资源池创建",
  "compute.onboard.method.import.cloudSoon": "暂未开放",

  "compute.onboard.form.name": "主机名",
  "compute.onboard.form.namePlaceholder": "例如 db-primary-01",
  "compute.onboard.form.nameHint":
    "3-50 个字符，可用字母、数字、下划线和连字符，首尾须为字母或数字。",
  "compute.onboard.form.nameInvalid": "主机名格式不合法。",
  "compute.onboard.form.ip": "IP 地址",
  "compute.onboard.form.ipHint": "选填。只通过 Agent 纳管时可以留空。",
  "compute.onboard.form.ipRequired": "自动安装 Agent 需要平台能通过 SSH 连到这个地址。",
  "compute.onboard.form.ipRequiredError": "勾选了自动安装 Agent，需要填写 IP 地址",
  "compute.onboard.form.sshPort": "SSH 端口",
  "compute.onboard.form.autoInstall": "创建后自动安装 Agent",
  "compute.onboard.form.autoInstallDesc":
    "平台通过 SSH 下发并安装 agent。连不通时会给出手动安装命令，主机档案不受影响。",
  "compute.onboard.form.noSpecForm":
    "无需填写操作系统、CPU、内存、磁盘 —— 这些信息在 agent 接入后自动上报。",
  "compute.onboard.form.createFailed": "创建主机失败，请重试",

  "compute.onboard.agent.hostCreated": "主机已创建：",
  "compute.onboard.agent.hostCreatedHint": "以下步骤是可选的，随时可以在主机详情页继续。",
  "compute.onboard.agent.skipped": "暂不安装 Agent",
  "compute.onboard.agent.skippedHint":
    "这台主机目前只有档案，没有可用的控制通道，终端、文件管理等功能不可用。可随时在主机详情页安装 Agent 或绑定 SSH 凭证。",
  "compute.onboard.agent.sshFailed": "无法自动安装，已切换为手动安装",
  "compute.onboard.agent.noCredential": "这台主机还没有绑定 SSH 凭证，平台无法登录它下发 agent。",

  "compute.onboard.install.onceWarning": "以下命令包含接入令牌，只显示这一次，离开本页后无法找回。",
  "compute.onboard.install.boundTo": "该令牌已绑定主机",
  "compute.onboard.install.runOnHost": "在目标主机上以 root 执行",
  "compute.onboard.install.copy": "复制",
  "compute.onboard.install.copied": "已复制到剪贴板",
  "compute.onboard.install.copyFailed": "复制失败，请手动选择命令文本",
  "compute.onboard.install.rootHint":
    "需要 root 权限。脚本会下载 agent、校验哈希、注册为 systemd 服务并完成接入。",
  "compute.onboard.install.waiting": "等待主机接入…",
  "compute.onboard.install.waitingHint": "命令执行成功后，主机会自动出现在列表中。",
  "compute.onboard.install.waitingAgent": "等待 Agent 上线…",
  "compute.onboard.install.waitingAgentHint": "命令执行成功后，agent 会接管这台已创建的主机。",
  "compute.onboard.install.canLeave": "可离开本页",
  "compute.onboard.install.joined": "主机已接入",
  "compute.onboard.install.agentOnline": "Agent 已上线",
  "compute.onboard.install.reportedHint": "以上规格信息由 agent 上报。",
  "compute.onboard.install.viewHost": "查看主机",
  "compute.onboard.install.mintFailed": "创建接入令牌失败，请重试",
} satisfies Messages

export default compute
