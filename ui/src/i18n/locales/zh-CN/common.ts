import type { Messages } from "../../types"

const common = {
  // common
  "common.name": "名称",
  "common.displayName": "显示名称",
  "common.status": "状态",
  "common.created": "创建时间",
  "common.createdBy": "创建人",
  "common.total": "共 {count} 个",
  "common.delete": "删除",
  "common.description": "描述",
  "common.edit": "编辑",
  "common.cancel": "取消",
  "common.confirm": "确认",
  "common.confirmByName.label": '请输入名称 "{name}" 以确认此操作。',
  "common.confirmByName.mismatch": "名称不匹配。",
  "common.save": "保存",
  "common.search": "搜索",
  "common.noData": "暂无数据",
  "common.noOptions": "暂无可选项",
  "common.actions": "操作",
  "common.active": "活跃",
  "common.inactive": "停用",
  "common.all": "全部",
  "common.phone": "手机号",
  "common.password": "密码",
  "common.previous": "上一页",
  "common.next": "下一页",
  "common.pageSize": "每页",
  "common.firstPage": "首页",
  "common.lastPage": "末页",
  "common.gotoPage": "第 {page} 页",
  "common.jumpTo": "跳至",
  "common.pageUnit": "页",
  "common.jumpToPageInput": "跳至指定页",
  "common.noSearchResults": "未找到匹配结果",
  "common.reset": "重置",
  "common.current": "当前",
  "common.loadError": "加载失败",
  "common.retry": "重试",
  "common.updated": "更新时间",

  // auth
  "auth.authenticating": "登录中...",
  "auth.missingCode": "缺少授权码",

  // login
  "login.title": "Vraxel Console",
  "login.username": "用户名",
  "login.password": "密码",
  "login.usernamePlaceholder": "请输入用户名",
  "login.passwordPlaceholder": "请输入密码",
  "login.signIn": "登录",
  "login.noAccount": "还没有账号？",
  "login.createAccount": "注册",
  "login.orContinueWith": "或使用以下方式",
  "login.social.github": "使用 GitHub 登录",
  "login.social.google": "使用 Google 登录",

  // register
  "register.title": "注册账号",
  "register.email": "邮箱",
  "register.emailPlaceholder": "请输入邮箱",
  "register.displayName": "显示名称",
  "register.displayNamePlaceholder": "请输入显示名称（可选）",
  "register.confirmPassword": "确认密码",
  "register.confirmPasswordPlaceholder": "请再次输入密码",
  "register.submit": "注册",
  "register.haveAccount": "已有账号？",
  "register.signIn": "去登录",

  // nav
  "nav.iam": "组织",
  "nav.workspaces": "租户",
  "nav.namespaces": "项目",
  "nav.users": "用户",
  "nav.roles": "角色管理",
  "nav.audit": "审计",
  "nav.auditLogs": "审计日志",
  "nav.kube": "Kubernetes",
  "nav.hosts": "主机",
  "nav.apiDocs": "API 文档",
  "nav.searchPlaceholder": "搜索菜单 ({key})",
  "nav.searchNoMatch": "无匹配",

  // overview

  // scope selector
  "scope.allWorkspaces": "所有租户",
  "scope.allNamespaces": "所有项目",
  "scope.selectWorkspace": "选择租户",
  "scope.selectNamespace": "选择项目",

  // permission verb wildcards
  "perm.group.all": "全部权限",
  "perm.verb.list": "所有列表 (*:list)",
  "perm.verb.get": "所有详情 (*:get)",
  "perm.verb.create": "所有创建 (*:create)",
  "perm.verb.update": "所有更新 (*:update)",
  "perm.verb.patch": "所有修改 (*:patch)",
  "perm.verb.delete": "所有删除 (*:delete)",
  "perm.verb.deleteCollection": "所有批量删除 (*:deleteCollection)",

  // permission verb groups
  "perm.verbGroup.read": "查询",
  "perm.verbGroup.create": "创建",
  "perm.verbGroup.update": "更新",
  "perm.verbGroup.delete": "删除",

  // error
  "error.400.title": "请求错误",
  "error.400.desc": "请求无法处理，请重试。",
  "error.401.title": "未授权",
  "error.401.desc": "请登录后继续。",
  "error.403.title": "禁止访问",
  "error.403.desc": "您没有权限访问此页面。",
  "error.404.title": "页面不存在",
  "error.404.desc": "您访问的页面不存在。",
  "error.500.title": "服务器错误",
  "error.500.desc": "系统出现问题，请稍后再试。",
  "error.backHome": "返回首页",
  "error.switchAccount": "切换账号",

  // login errors
  "login.error.invalidCredentials": "用户名或密码错误",
  "login.error.accountInactive": "账号已被停用",
  "login.error.tooManyAttempts": "失败次数过多，账号已临时锁定，请稍后再试",
  "login.error.sessionExpired": "登录会话已过期，正在重新跳转...",
  "login.error.failed": "登录失败，请重试",

  // register errors
  "register.error.passwordMismatch": "两次输入的密码不一致",
  "register.error.conflict": "用户名或邮箱已存在",
  "register.error.tooManyAttempts": "注册尝试过于频繁，请稍后再试",
  "register.error.failed": "注册失败，请重试",

  // api errors
  "api.error.badRequest": "请求错误",
  "api.error.invalidJSONBody": "请求体格式错误",
  "api.error.notFound": "未找到",
  "api.error.conflict": "操作冲突",
  "api.error.badGateway": "上游服务异常，请稍后重试",
  "api.error.gatewayTimeout": "上游服务响应超时，请稍后重试",
  "api.error.networkError": "无法连接到服务器，请检查网络或稍后重试",
  "api.error.sessionExpired": "登录已过期，正在跳转登录页",
  "api.error.memberLimitExceeded": "项目成员数已达上限",
  "api.error.cannotDeleteWorkspace": "无法删除租户：租户下仍有未释放的资源，请先删除后再操作",
  "api.error.cannotDeleteNamespace": "无法删除项目：项目下仍有未释放的资源，请先删除后再操作",
  "blockingResource.host": "主机",
  // placement scheduler errors (后端 reason 来自 placement.ReasonFor)
  "api.error.cannotRemoveOwner": "无法移除所有者",
  "api.error.oldPasswordIncorrect": "当前密码不正确",
  "api.error.forbidden": "您没有权限执行此操作",
  "api.error.internalError": "服务器内部错误，请稍后重试",
  "api.error.timeout": "请求超时，请稍后重试",
  "api.error.cannotDeleteBuiltinRole": "无法删除内置角色",
  "api.error.cannotDeleteRoleWithBindings": "无法删除有绑定关系的角色",
  "api.error.valueTooLong": "输入值过长",

  // validation errors
  "api.validation.formHasErrors": "表单存在 {count} 个未通过校验的字段：",
  "api.validation.required": "{field}不能为空",
  "api.validation.username.format": "用户名需为3-50位字母、数字、下划线或连字符",
  "api.validation.email.format": "请输入有效的邮箱地址",
  "api.validation.phone.format": "请输入有效的手机号（如 13800138000）",
  "api.validation.password.length": "密码长度需为 8-72 位",
  "api.validation.password.uppercase": "密码需包含至少一个大写字母",
  "api.validation.password.lowercase": "密码需包含至少一个小写字母",
  "api.validation.password.digit": "密码需包含至少一个数字",
  "api.validation.name.format": "名称需为3-50位字母、数字、连字符或下划线，以字母或数字开头和结尾",
  "api.validation.rackCapacity.min": "机柜容量不能小于 0",
  "api.validation.rackCapacity.range": "机柜容量必须为 0-10000 之间的整数",
  "api.validation.uHeight.range": "U 高度必须为 0-100 之间的整数",
  "api.validation.status.format": "状态必须为「活跃」或「停用」",
  "api.validation.username.taken": "该用户名已被使用",
  "api.validation.email.taken": "该邮箱已被使用",
  "api.validation.phone.taken": "该手机号已被使用",
  "api.validation.password.hint": "8-72 位，需包含大写字母、小写字母和数字",
  "api.validation.name.networkFormat": "需为3-50位小写字母、数字或连字符，以字母或数字开头和结尾",
  "api.validation.cidr.format": "请输入有效的 CIDR（如 10.0.0.0/24）",
  "api.validation.ip.format": "请输入有效的 IP 地址",
  "api.validation.image.format": "镜像引用格式无效（如 nginx:1.25、registry.example.com/repo:tag）",
  "api.validation.gateway.notInRange": "网关不在 CIDR 范围内",
  "api.validation.cidr.overlap": "CIDR 与已有子网重叠",
  "api.validation.cidr.notWithinNetwork": "子网 CIDR 不在网络 CIDR 范围内",
  "api.validation.description.tooLong": "描述内容过长",
  "api.validation.minLength": "至少需要 {min} 个字符",
  "api.validation.maxLength": "不能超过 {max} 个字符",
  "api.validation.intRange": "必须在 {min} 到 {max} 之间",
  "api.validation.memberRange": "必须在 0 到 1,000,000 之间",
  "api.validation.subnetRange": "必须在 1 到 50 之间",
  "api.validation.nonNegativeInt": "必须为非负整数",
  "api.validation.integer.format": '必须为非负整数，不能以 0 开头（"0" 单独可以）',
  "api.validation.positive": "必须为正数",
  "api.validation.port.range": "端口必须在 1 到 65535 之间",
  "api.validation.lat.range": "纬度必须在 -90 到 90 之间",
  "api.validation.lng.range": "经度必须在 -180 到 180 之间",
  "api.validation.tooLarge": "值太大",
  "api.validation.path.absolute": "必须为绝对路径（以 / 开头）",
  "api.validation.bufferPool.format": "格式须为数字加 M 或 G（如 256M、4G）",
  "api.validation.resources.cpu.format":
    "数量须为正整数；输入小数（如 0.5 Cores）离开输入框后会自动换算为更小单位（500 m）。",
  "api.validation.resources.memory.format":
    "数量须为正整数；输入小数（如 0.5 Gi）离开输入框后会自动换算为更小单位（512 Mi）。",
  "api.validation.resources.volumeSize.format": "格式须为数字加 Mi/Gi/Ti（如 1Gi、500Mi、2Ti）",
  "api.validation.connRange": "必须在 1 到 100,000 之间",
  "api.validation.binaryChoice": "必须为 0 或 1",
  "api.validation.hostnameOrIpRequired": "主机名或 IP 地址不能都为空",

  // batch delete

  // action feedback
  "action.createSuccess": "创建成功",
  "action.updateSuccess": "更新成功",
  "action.deleteSuccess": "删除成功",
  "action.resetPasswordSuccess":
    "密码已重置。该用户的刷新令牌已吊销，现有 access token 在最长 1 小时内自动失效。",

  // user menu
  "userMenu.changePassword": "修改密码",
  "userMenu.logout": "退出登录",
  "userMenu.oldPassword": "当前密码",
  "userMenu.newPassword": "新密码",
  "userMenu.confirmPassword": "确认新密码",
  "userMenu.passwordMismatch": "两次输入的密码不一致",
  "userMenu.passwordSameAsOld": "新密码不能与当前密码相同",

  // task terminal

  // Shared probe / handler editor
} satisfies Messages

export default common
