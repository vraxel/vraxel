# sshclient

SSH 客户端封装，提供连接管理、远程命令执行和 SFTP 文件传输能力。

依赖 `golang.org/x/crypto/ssh` 和 `github.com/pkg/sftp`。

## 连接

```go
import "vraxel.io/vraxel/lib/clients/sshclient"

client := sshclient.New(sshclient.Config{
    Host:     "192.168.1.10",
    Port:     22,        // 默认 22
    User:     "root",    // 默认 root
    Password: "secret",
    Timeout:  10 * time.Second, // 默认 30s
})

if err := client.Connect(ctx); err != nil {
    log.Fatal(err)
}
defer client.Close()
```

### 认证方式

三种认证方式按优先级排列，密钥认证互斥（只用最高优先级的一种），密码认证独立叠加：

| 优先级 | 方式 | Config 字段 |
|--------|------|------------|
| 1 | 密钥内容（PEM 原文） | `PrivateKeyContent` |
| 2 | 密钥文件路径 | `PrivateKey` |
| 3 | 默认 `~/.ssh/id_rsa`（不存在则跳过） | — |
| — | 密码认证（与密钥认证叠加） | `Password` |

```go
// 密钥文件认证
client := sshclient.New(sshclient.Config{
    Host:       "192.168.1.10",
    PrivateKey: "/path/to/id_rsa",
})

// 密钥内容认证（适用于从数据库读取）
client := sshclient.New(sshclient.Config{
    Host:              "192.168.1.10",
    PrivateKeyContent: pemContent,
})
```

## 堡垒机跳转（ProxyJump）

通过堡垒机/跳板机访问受保护网络中的目标主机，等价于 `ssh -J bastion target`：

```go
client := sshclient.New(sshclient.Config{
    Host:     "10.0.0.5",
    Password: "target-pass",
    ProxyJump: &sshclient.Config{
        Host:     "bastion.example.com",
        Password: "bastion-pass",
    },
})

if err := client.Connect(ctx); err != nil {
    log.Fatal(err)
}
defer client.Close() // 自动级联关闭：目标连接 → 隧道 → 堡垒机
```

支持多级跳转（递归嵌套）：

```go
client := sshclient.New(sshclient.Config{
    Host:     "10.0.0.5",
    Password: "target-pass",
    ProxyJump: &sshclient.Config{
        Host:     "bastion2",
        Password: "pass2",
        ProxyJump: &sshclient.Config{
            Host:     "bastion1",
            Password: "pass1",
        },
    },
})
```

连接建立后，`Exec`、`ExecWithSudo`、`PutFile`、`FetchFile` 等操作与直连完全一致。

## 命令执行

### 普通执行

```go
result, err := client.Exec(ctx, "uname -r")
if err != nil {
    log.Fatal(err)
}
fmt.Println(string(result.Stdout)) // "5.15.0-91-generic"
```

命令以非零状态退出时返回 error，但 `ExecResult` 仍包含已捕获的 stdout/stderr。

### sudo 执行

```go
// 需要密码的 sudo
result, err := client.ExecWithSudo(ctx, "systemctl restart nginx", "password")

// 免密 sudo（NOPASSWD）
result, err := client.ExecWithSudo(ctx, "systemctl restart nginx", "")
```

### 流式输出

```go
err := client.ExecStream(ctx, "tail -f /var/log/syslog", os.Stdout, os.Stderr)
```

实时将 stdout/stderr 写入提供的 `io.Writer`，适用于长时间运行的命令。

所有执行方法均支持 context 取消，取消时向远程进程发送 SIGKILL。

## 文件传输（SFTP）

### 上传文件

```go
content := []byte("server { listen 80; }")
err := client.PutFile(content, "/etc/nginx/conf.d/app.conf", 0644)
```

自动创建远程目标目录（如不存在）。

### 下载文件

```go
var buf bytes.Buffer
err := client.FetchFile("/etc/os-release", &buf)
fmt.Println(buf.String())
```

## 与 probe 配合使用

建议在 SSH 连接前先用 `lib/probe` 做 TCP 端口探测，快速失败：

```go
import "vraxel.io/vraxel/lib/probe"

// 1. 先测端口连通性
result := probe.TCP(ctx, "192.168.1.10:22", nil)
if !result.Success {
    return fmt.Errorf("SSH 端口不通: %s", result.Message)
}

// 2. 端口通了再建立 SSH 连接
client := sshclient.New(sshclient.Config{
    Host:     "192.168.1.10",
    Password: "secret",
})
if err := client.Connect(ctx); err != nil {
    return fmt.Errorf("SSH 认证失败: %w", err)
}
defer client.Close()
```
