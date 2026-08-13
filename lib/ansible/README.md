# lib/ansible — Ansible 兼容自动化引擎

轻量级 Ansible Playbook 执行引擎，兼容 Ansible 的 playbook、role、inventory 格式，无 Kubernetes 依赖。

## 包结构

```
lib/ansible/                 # 核心类型（Playbook, Play, Block, Task, Role, Inventory）
lib/ansible/connector/       # 连接器接口 + SSH/Local 实现
lib/ansible/variable/        # 分层变量系统（host > group > inventory）
lib/ansible/template/        # Go template + Sprig 模板引擎
lib/ansible/modules/         # 13 个内置模块
lib/ansible/converter/       # YAML 解析与转换
lib/ansible/project/         # Playbook 文件来源（本地 / embed.FS）
lib/ansible/executor/        # 执行引擎（Task → Block → Role → Playbook）
lib/clients/sshclient/       # 通用 SSH 客户端
```

## 快速开始

### 最简示例：执行一个 Playbook

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"

    "vraxel.io/vraxel/lib/ansible"
    "vraxel.io/vraxel/lib/ansible/converter"
    "vraxel.io/vraxel/lib/ansible/executor"
    "vraxel.io/vraxel/lib/ansible/project"

    // 注册所有内置模块（command, shell, copy, template 等）
    _ "vraxel.io/vraxel/lib/ansible/modules"
)

func main() {
    // 1. 定义 Inventory
    inv := ansible.Inventory{
        Hosts: map[string]map[string]any{
            "10.0.0.1": {
                "remote_user":         "deploy",
                "private_key_content": "...",     // PEM 私钥
                "become":              true,      // sudo 提权
            },
            "10.0.0.2": {
                "remote_user": "deploy",
                "password":    "your-password",   // SSH 密码（同时用于 sudo）
                "become":      true,
            },
        },
        Groups: map[string]ansible.InventoryGroup{
            "web": {Hosts: []string{"10.0.0.1", "10.0.0.2"}},
        },
    }

    // 2. 解析 Playbook YAML
    playbookYAML := []byte(`
- hosts: web
  gather_facts: false
  tasks:
    - name: 检查主机连通性
      shell: echo "hello from $(hostname)"
      register: result

    - name: 输出结果
      debug:
        msg: "{{ .result.stdout }}"
`)

    playbook, err := converter.ParsePlaybook(playbookYAML)
    if err != nil {
        log.Fatal(err)
    }

    // 3. 创建执行器并运行
    source := project.NewLocalSource(".")
    exec := executor.NewPlaybookExecutor(inv, source,
        executor.WithLogOutput(os.Stdout),
    )

    result, err := exec.Execute(context.Background(), playbook)
    if err != nil {
        log.Fatalf("执行失败: %v", err)
    }

    fmt.Printf("执行完成: success=%v\n", result.Success)
}
```

### 从文件加载 Playbook

```go
// playbook 和 role 都放在 /opt/playbooks/ 目录下
source := project.NewLocalSource("/opt/playbooks")

data, _ := source.ReadFile("site.yml")
playbook, _ := converter.ParsePlaybook(data)

exec := executor.NewPlaybookExecutor(inv, source)
result, err := exec.Execute(ctx, playbook)
```

### 使用 embed.FS 内嵌 Playbook

```go
import "embed"

//go:embed playbooks/*
var playbookFS embed.FS

func run() {
    source := project.NewBuiltinSource(playbookFS, "playbooks")
    data, _ := source.ReadFile("setup.yml")
    playbook, _ := converter.ParsePlaybook(data)

    exec := executor.NewPlaybookExecutor(inv, source)
    result, err := exec.Execute(ctx, playbook)
}
```

## Playbook 格式

完全兼容 Ansible YAML 格式：

```yaml
# site.yml
- hosts: web
  gather_facts: true
  become: true
  vars:
    app_version: "1.2.0"
  vars_files:
    - vars/common.yml
  pre_tasks:
    - name: 检查磁盘空间
      shell: df -h /
      register: disk_info

  roles:
    - common
    - role: nginx
      vars:
        nginx_port: 8080

  tasks:
    - name: 部署应用
      copy:
        src: app.tar.gz
        dest: /opt/app.tar.gz
      when: '{{ ne .app_version "" }}'

    - name: 解压
      shell: tar -xzf /opt/app.tar.gz -C /opt/

    - name: 记录部署结果
      result:
        deployed_version: "{{ .app_version }}"

  post_tasks:
    - name: 健康检查
      shell: curl -f http://localhost:8080/health
      retries: 5
      delay: 3
      until: '{{ eq .result.stdout "ok" }}'
```

## Role 目录结构

```
roles/
  nginx/
    tasks/main.yml       # 任务列表（必须）
    handlers/main.yml    # 处理器
    vars/main.yml        # 角色变量（高优先级）
    defaults/main.yml    # 默认变量（低优先级）
    templates/           # 模板文件
    files/               # 静态文件
```

## Inventory 定义

```go
inv := ansible.Inventory{
    // 主机及其变量
    Hosts: map[string]map[string]any{
        "web1": {
            "remote_user":         "deploy",
            "private_key_content": pemBytes,
            "become":              true,
        },
        "web2": {
            "remote_user": "deploy",
            "password":    "deploy-password",
            "become":      true,
        },
        "db1": {
            "remote_user": "root",
            "password":    "root-password",
        },
        "local": {"connection": "local"},
    },
    // 全局变量
    Vars: map[string]any{
        "env": "production",
    },
    // 主机组
    Groups: map[string]ansible.InventoryGroup{
        "web":     {Hosts: []string{"web1", "web2"}},
        "db":      {Hosts: []string{"db1"}},
        "backend": {Groups: []string{"web", "db"}}, // 嵌套组
    },
}
```

### 连接变量

| 变量 | 类型 | 说明 | 默认值 |
|------|------|------|--------|
| `connection` | string | 连接方式：`ssh` 或 `local` | 自动检测 |
| `remote_user` | string | SSH 用户名 | `root` |
| `port` | int | SSH 端口 | `22` |
| `password` | string | SSH 密码（同时用于 sudo） | — |
| `private_key` | string | 私钥文件路径 | — |
| `private_key_content` | string | 私钥 PEM 内容 | — |
| `become` | bool | 是否 sudo 提权 | `false` |
| `become_user` | string | sudo 目标用户 | `root` |

> **推荐非 root 用户 + become**：使用普通用户 SSH 登录，需要 root 权限的操作通过 `become: true` 自动 sudo。要求目标主机 sudoers 配置允许该用户执行 sudo（可带密码或 NOPASSWD）。

变量优先级（从低到高）：`Inventory.Vars` → 组变量 → `gather_facts` → 运行时变量 → 主机变量

## 内置模块

| 模块 | 用途 | 支持 become | 示例 |
|------|------|-------------|------|
| `command` | 直接执行命令 | ✅ (connector) | `command: ls -la /opt` |
| `shell` | 通过 shell 执行 | ✅ (connector) | `shell: echo $HOME` |
| `copy` | 上传文件 | ✅ | `copy: {src: app.conf, dest: /etc/app.conf, mode: "0644", owner: app, group: app}` |
| `fetch` | 下载远程文件 | ✅ | `fetch: {src: /var/log/app.log, dest: ./logs/}` |
| `template` | 渲染模板后上传 | ✅ | `template: {src: nginx.conf.j2, dest: /etc/nginx/nginx.conf, owner: nginx, group: nginx}` |
| `setup` | 采集主机信息 | ✅ (connector) | 由 `gather_facts: true` 触发 |
| `set_fact` | 设置运行时变量 | — | `set_fact: {app_url: "http://{{ .host }}:8080"}` |
| `include_vars` | 从文件加载变量 | — | `include_vars: {file: vars/secrets.yml}` |
| `add_hostvars` | 添加主机变量 | — | `add_hostvars: {host: web1, role: primary}` |
| `debug` | 打印调试信息 | — | `debug: {msg: "部署到 {{ .target }}"}` |
| `assert` | 条件断言 | — | `assert: {that: ['{{ eq .env "prod" }}'], fail_msg: "非生产环境"}` |
| `result` | 存储全局执行结果 | — | `result: {version: "{{ .app_version }}"}` |
| `http_get_file` | HTTP 下载文件 | — | `http_get_file: {url: "https://...", dest: /tmp/pkg.tar.gz}` |

> `copy` 和 `template` 支持 `owner`/`group` 参数，上传后自动 `chown`。`fetch` 在 become 模式下通过 sudo cp 到临时文件再下载。

## 权限提升 (become)

### 非 root 用户执行

推荐使用普通用户 SSH 登录 + `become: true` 提权：

```go
inv := ansible.Inventory{
    Hosts: map[string]map[string]any{
        "10.0.0.1": {
            "remote_user": "deploy",       // 普通用户
            "password":    "deploy-pwd",   // SSH + sudo 密码
            "become":      true,           // 自动 sudo
        },
    },
}
```

可在 play 级别或 task 级别控制：

```yaml
# play 级别 — 所有 task 默认 sudo
- hosts: web
  become: true
  tasks:
    - shell: systemctl restart nginx    # 自动 sudo
    - shell: whoami                      # 也会 sudo，输出 root

# task 级别 — 仅特定 task sudo
- hosts: web
  tasks:
    - shell: whoami                      # 以 deploy 用户执行
    - shell: systemctl restart nginx     # sudo 执行
      become: true
```

### sudo 到指定用户

```yaml
- hosts: web
  become: true
  become_user: www-data
  tasks:
    - shell: whoami    # 输出 www-data
```

### 文件操作权限

`template` 和 `copy` 模块在非 root 用户下自动处理：

1. SFTP 上传到 `/tmp` 临时文件（非 root 可写）
2. sudo mv 到目标路径
3. sudo chmod 设置权限
4. sudo chown 设置属主（如果指定了 owner/group）

```yaml
- template:
    src: app.conf.j2
    dest: /etc/myapp/app.conf
    mode: "0640"
    owner: myapp
    group: myapp
  become: true
```

## 流程控制

### 条件执行

```yaml
- name: 仅在 CentOS 上执行
  shell: yum install nginx
  when: '{{ eq .os_release.ID "centos" }}'
```

### 循环

```yaml
- name: 创建目录
  shell: "mkdir -p {{ .item }}"
  loop:
    - /opt/app
    - /opt/logs
    - /opt/data
```

### 重试

```yaml
- name: 等待服务就绪
  shell: curl -sf http://localhost:8080/health
  retries: 10
  delay: 5
  until: '{{ eq .result.failed false }}'
```

### Block / Rescue / Always

```yaml
- block:
    - name: 部署新版本
      shell: deploy.sh
  rescue:
    - name: 回滚
      shell: rollback.sh
  always:
    - name: 清理临时文件
      shell: rm -rf /tmp/deploy-*
```

### Serial 分批执行

```yaml
- hosts: web
  serial:
    - 1        # 第一批 1 台
    - "50%"    # 之后每批 50%
  tasks:
    - name: 滚动更新
      shell: restart-app.sh
```

### Tags 过滤

```yaml
- name: 安装依赖
  shell: apt install -y nginx
  tags: [install]

- name: 配置服务
  template:
    src: nginx.conf.j2
    dest: /etc/nginx/nginx.conf
  tags: [config]
```

```go
// 只执行带 "config" 标签的任务
exec := executor.NewPlaybookExecutor(inv, source,
    executor.WithTags([]string{"config"}),
)
```

## 模板语法

使用 Go `text/template` 语法 + [Sprig](https://masterminds.github.io/sprig/) 函数库：

```yaml
# 变量引用
msg: "{{ .app_version }}"

# Sprig 函数
msg: "{{ .name | upper }}"
msg: "{{ default \"fallback\" .optional_var }}"
msg: "{{ join \",\" .servers }}"

# 自定义函数
msg: "{{ toYaml .config }}"          # 转 YAML 字符串
msg: "{{ ipFamily .listen_addr }}"   # 返回 "IPv4" 或 "IPv6"
```

## 注册自定义模块

```go
import "vraxel.io/vraxel/lib/ansible/modules"

func init() {
    modules.RegisterModule("my_module", func(ctx context.Context, opts modules.ExecOptions) (string, string, error) {
        name, _ := opts.Args["name"].(string)

        // 获取主机变量
        vars := opts.GetAllVariables()

        // 通过连接器执行远程命令
        stdout, stderr, err := opts.Connector.ExecuteCommand(ctx, "echo "+name)

        return string(stdout), string(stderr), err
    })
}
```

在 Playbook 中使用：

```yaml
- name: 调用自定义模块
  my_module:
    name: hello
```

## 连接器

自动根据主机地址选择连接方式：

| 地址 | 连接方式 |
|------|---------|
| `localhost` / `127.0.0.1` | 本地执行 (`os/exec`) |
| 其他地址 | SSH 连接 |
| `connection: local` | 强制本地执行 |

SSH 认证优先级：`private_key_content` → `private_key` 文件 → `~/.ssh/id_rsa` → `password`

## 执行结果

```go
result, err := exec.Execute(ctx, playbook)

// result.Success    — 是否全部成功
// result.Error      — 错误信息（如有）
// result.StartTime  — 开始时间
// result.EndTime    — 结束时间
// result.Stats      — 执行统计

// 获取 result 模块存储的全局结果
detail := exec.Variable().Get(variable.GetResultVariable())
```

## 依赖

仅依赖 3 个外部库：

| 依赖 | 用途 |
|------|------|
| `golang.org/x/crypto/ssh` | SSH 连接（已有模块子包） |
| `github.com/pkg/sftp` | SFTP 文件传输 |
| `github.com/Masterminds/sprig/v3` | 模板函数库 |
