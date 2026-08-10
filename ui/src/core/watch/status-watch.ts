import { create } from "zustand"
import { buildTaskWsUrl } from "@/core/watch/ws-url"

export interface StatusEvent {
  entityType: string
  entityId: number
  // status 是 P1-P3 dual-track 期间保留的旧字段，承载 hosts.status 列
  // 的字面值（含 active / provisioning 等 legacy 值）。
  status: string
  // host status redesign — 新前端只读这三个字段，phase 是单一 badge
  // 数据源。后端 PhaseStore.Apply 在 commit 后通过 statushub 推过来；
  // legacy 写入路径（provision_host.go 等）发的事件不会带这三个字段，
  // 此时 phase 为 undefined，前端不更新 host.status.phase（保留旧值）。
  phase?: string
  opKind?: string
  phaseAt?: string
  // 节点级事件（如 mysql_node）携带 status_message，订阅者可同步更新 detail
  // 页 error tooltip。空字符串明确清空消息（成功收尾）；undefined 表示该事件
  // 不带 message 字段（实例级事件等），订阅者保留缓存。
  statusMessage?: string
  // deleted 是硬删除信号：行已从数据库消失，订阅者应该把它从缓存
  // 列表里剔除。替代曾经用 status === "deleted" 做哨兵的旧约定。
  deleted?: boolean
}

type Listener = (event: StatusEvent) => void

interface StatusWatchState {
  listeners: Set<Listener>
  subscribe: (fn: Listener) => () => void
}

export const useStatusWatch = create<StatusWatchState>()((_set, get) => ({
  listeners: new Set<Listener>(),
  subscribe: (fn: Listener) => {
    get().listeners.add(fn)
    return () => { get().listeners.delete(fn) }
  },
}))

// Per-module WS connections. The map keys by module so a host page +
// a mysql page can run their watchers in parallel without one closing
// the other (covers split-pane setups, future side-panel previews,
// etc.). Each connection auto-reconnects on close.
type Connection = {
  ws: WebSocket | null
  scopeKey: string
  reconnectTimer: ReturnType<typeof setTimeout> | null
  // Refcount of mounted subscribers (pages/components). The socket used
  // to live forever once opened (disconnectModuleWatch had zero callers)
  // and kept 3s-reconnecting even after logout. Now each
  // connectModuleWatch() returns a release fn; when the count drops to
  // zero the connection closes after a short linger (so a list->detail
  // navigation within the same module doesn't bounce the socket).
  refCount: number
  idleCloseTimer: ReturnType<typeof setTimeout> | null
}
const connections = new Map<string, Connection>()

const IDLE_CLOSE_DELAY_MS = 5000

function dispatch(event: StatusEvent) {
  useStatusWatch.getState().listeners.forEach(fn => fn(event))
}

function connect(module: string, wsUrl: string, scopeKey: string) {
  const c = connections.get(module) || { ws: null, scopeKey: "", reconnectTimer: null, refCount: 0, idleCloseTimer: null }
  if (c.reconnectTimer) { clearTimeout(c.reconnectTimer); c.reconnectTimer = null }
  if (c.ws) { c.ws.close(); c.ws = null }

  c.scopeKey = scopeKey
  const ws = new WebSocket(wsUrl)
  ws.binaryType = "arraybuffer"
  ws.onmessage = (e) => {
    const arr = new Uint8Array(e.data as ArrayBuffer)
    if (arr[0] !== 0x00) return
    try {
      dispatch(JSON.parse(new TextDecoder().decode(arr.slice(1))))
    } catch { /* ignore */ }
  }
  ws.onclose = () => {
    // The browser fires close asynchronously: when connect() replaces a
    // socket (scope change) or disconnectModuleWatch() closed it, this
    // stale handler must not null out the CURRENT socket or reconnect.
    if (c.ws !== ws) return
    c.ws = null
    if (c.scopeKey === scopeKey) {
      c.reconnectTimer = setTimeout(() => connect(module, wsUrl, scopeKey), 3000)
    }
  }
  c.ws = ws
  connections.set(module, c)
}

// connectModuleWatch is the generic entry point: subscribe to one
// module's resource-collection /watch endpoint. Listeners filter by
// event.entityType to discard events from other entity types if the
// module hub is shared across resources (db's hub will fan out
// mysql/pgsql/redis events; subscribers on /db/mysql/watch only get
// mysql events because the watch handler filters server-side, but
// listeners should still defensively check entityType).
export function connectModuleWatch(module: string, resource: string, scopeWorkspaceId?: string, scopeNamespaceId?: string): () => void {
  const scopeKey = `${resource}:${scopeWorkspaceId || ""}:${scopeNamespaceId || ""}`
  let c = connections.get(module)
  if (!c) {
    c = { ws: null, scopeKey: "", reconnectTimer: null, refCount: 0, idleCloseTimer: null }
    connections.set(module, c)
  }
  c.refCount++
  if (c.idleCloseTimer) { clearTimeout(c.idleCloseTimer); c.idleCloseTimer = null }
  const sameScopeAlive = c.scopeKey === scopeKey &&
    ((c.ws && c.ws.readyState <= WebSocket.OPEN) || c.reconnectTimer !== null)
  if (!sameScopeAlive) {
    const wsUrl = buildTaskWsUrl(module, resource, scopeWorkspaceId, scopeNamespaceId)
    connect(module, wsUrl, scopeKey)
  }
  let released = false
  return () => {
    if (released) return
    released = true
    const cur = connections.get(module)
    if (!cur) return
    cur.refCount = Math.max(0, cur.refCount - 1)
    if (cur.refCount === 0 && !cur.idleCloseTimer) {
      cur.idleCloseTimer = setTimeout(() => disconnectModuleWatch(module), IDLE_CLOSE_DELAY_MS)
    }
  }
}

export function disconnectModuleWatch(module: string) {
  const c = connections.get(module)
  if (!c) return
  c.scopeKey = ""
  if (c.reconnectTimer) { clearTimeout(c.reconnectTimer); c.reconnectTimer = null }
  if (c.idleCloseTimer) { clearTimeout(c.idleCloseTimer); c.idleCloseTimer = null }
  if (c.ws) { c.ws.close(); c.ws = null }
  connections.delete(module)
}

// Backward-compat shim: existing host pages call this.
export function connectStatusWatch(scopeWorkspaceId?: string, scopeNamespaceId?: string): () => void {
  return connectModuleWatch("compute", "hosts/watch", scopeWorkspaceId, scopeNamespaceId)
}
