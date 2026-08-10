# lib/websocket

通用 WebSocket 基础设施包，封装 `github.com/coder/websocket`，为业务模块提供连接管理、消息协议和会话控制能力。

## 包结构

```
lib/websocket/
├── conn.go        Conn 连接封装（隔离第三方依赖）
├── upgrader.go    HTTP → WebSocket 升级
├── message.go     二进制消息协议（类型前缀帧）
└── session.go     SessionManager（并发限制 + 会话追踪）
```

## 快速开始

### 注册 WebSocket Action

在 `install.go` 中通过 `rest.ActionInfo` 注册：

```go
import "vraxel.io/vraxel/lib/websocket"

// 创建 SessionManager（通常在模块初始化时）
sessionMgr := websocket.NewSessionManager(5, 30*time.Minute)

// 注册到资源路由
{
    Name:    "hosts",
    Storage: hostStorage,
    Actions: []rest.ActionInfo{
        {
            Name:             "exec",
            Method:           "GET",
            WebSocketHandler: NewExecHandler(sessionMgr, hostStore),
        },
    },
}
// 生成路由: GET /api/infra/v1/hosts/{hostId}/exec
// 生成权限: infra:hosts:exec
```

### 实现 WebSocketHandler

```go
func NewExecHandler(sm *websocket.SessionManager, hostStore HostStore) rest.WebSocketHandler {
    return func(ctx context.Context, params map[string]string, conn *websocket.Conn) {
        defer conn.Close(websocket.StatusNormalClosure, "")

        // 1. 读取 connect 消息
        _, raw, err := conn.ReadMessage(ctx)
        if err != nil {
            return
        }
        msgType, payload, _ := websocket.DecodeMessage(raw)
        if msgType != websocket.MsgConnect {
            return
        }
        cp, _ := websocket.DecodeConnectPayload(payload)

        // 2. 获取会话
        userID := oidc.UserIDFromContext(ctx)
        sess, err := sm.Acquire(userID, "host", params["hostId"], cp.User+"@host", cancel)
        if err != nil {
            // 发送错误状态
            msg, _ := websocket.EncodeStatusMessage(&websocket.StatusPayload{
                Status: "error", Message: err.Error(),
            })
            conn.WriteBinary(ctx, msg)
            return
        }
        defer sm.Release(sess.ID)

        // 3. 建立 SSH 连接、分配 PTY ...
        // 4. 双向桥接 ...
    }
}
```

## 消息协议

每条 WebSocket 二进制消息的第一个字节标识消息类型：

```
[1 byte type] [payload]
```

| 类型 | 值 | 方向 | Payload |
|------|-----|------|---------|
| `MsgData` | `0x00` | 双向 | 原始终端数据（stdin/stdout） |
| `MsgResize` | `0x01` | 客户端→服务端 | `{"cols": 120, "rows": 40}` |
| `MsgConnect` | `0x02` | 客户端→服务端 | `{"cols":80,"rows":24,"user":"root","password":"..."}` |
| `MsgStatus` | `0x03` | 服务端→客户端 | `{"status":"connected","message":"..."}` |

### 编码

```go
// 数据消息
msg := websocket.EncodeMessage(websocket.MsgData, []byte("ls -la\n"))
conn.WriteBinary(ctx, msg)

// 状态消息
msg, _ := websocket.EncodeStatusMessage(&websocket.StatusPayload{
    Status: "connected", Message: "Connected to 10.0.1.5",
})
conn.WriteBinary(ctx, msg)

// Resize 消息
msg, _ := websocket.EncodeResizeMessage(&websocket.ResizePayload{Cols: 150, Rows: 50})
conn.WriteBinary(ctx, msg)

// Connect 消息
msg, _ := websocket.EncodeConnectMessage(&websocket.ConnectPayload{
    Cols: 120, Rows: 40, User: "root", Password: "secret",
})
conn.WriteBinary(ctx, msg)
```

### 解码

```go
_, raw, err := conn.ReadMessage(ctx)
msgType, payload, err := websocket.DecodeMessage(raw)

switch msgType {
case websocket.MsgData:
    // payload 是原始终端数据
    stdin.Write(payload)

case websocket.MsgResize:
    resize, _ := websocket.DecodeResizePayload(payload)
    session.WindowChange(resize.Rows, resize.Cols)

case websocket.MsgConnect:
    cp, _ := websocket.DecodeConnectPayload(payload)
    // cp.User, cp.Password, cp.PrivateKey, cp.Port
    // cp.Cols, cp.Rows（零值默认 80x24）
}
```

## SessionManager

管理 WebSocket 会话的并发限制和生命周期追踪。

```go
// 创建：每用户最多 5 个会话，空闲超时 30 分钟
sm := websocket.NewSessionManager(5, 30*time.Minute)

// 获取会话（超限返回错误）
sess, err := sm.Acquire(userID, "host", hostID, "root@10.0.1.5", cancelFunc)

// 释放会话
sm.Release(sess.ID)

// 查询
count := sm.Count(userID)           // 当前会话数
sessions := sm.List(userID)         // 会话列表（安全拷贝）

// 强制断开（调用 cancel，会话仍需 Release）
sm.Cancel(sess.ID)

// 读取配置
timeout := sm.IdleTimeout()
```

## Conn

封装 `github.com/coder/websocket`，隔离第三方依赖：

```go
// 服务端升级
conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
    InsecureSkipVerify: true,  // 开发环境
})

// 读写
msgType, data, err := conn.ReadMessage(ctx)
err = conn.WriteBinary(ctx, data)
err = conn.WriteMessage(ctx, msgType, data)

// 关闭
conn.Close(websocket.StatusNormalClosure, "bye")
```

## REST 框架集成

`lib/rest/` 中的 `WebSocketHandler` 类型与 `HandlerFunc` 平级：

```go
// lib/rest/apigroup.go
type ActionInfo struct {
    Name             string
    Method           string
    StatusCode       int              // 仅用于 Handler
    Handler          HandlerFunc      // JSON 请求/响应（与 WebSocketHandler 互斥）
    WebSocketHandler WebSocketHandler // WebSocket 连接（与 Handler 互斥）
}
```

框架自动处理：
- HTTP → WebSocket 升级
- Path params + Query params 提取和合并
- 认证/鉴权 filter chain 在升级前执行
- RBAC 权限自动注册（如 `infra:hosts:exec`）

设置两者会在启动时 panic。

## 依赖

- `github.com/coder/websocket` — 封装在本包内部，其他模块无需直接导入
