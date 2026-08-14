import { create } from "zustand"
import { buildTaskWsUrl } from "@/core/watch/ws-url"
import { openReconnectingSocket } from "./reconnecting-socket"

/**
 * One entity change, as pushed by a module's /watch endpoint.
 *
 * It deliberately carries no data about the entity. Subscribers refetch
 * through the normal API on receipt, so the socket never becomes a
 * second, subtly different source of truth for what a row looks like --
 * and a dropped event (the server drops for slow readers, and pgnotify
 * drops during a reconnect) costs a stale row until the next event
 * rather than a wrong one.
 */
export interface StatusEvent {
  /** Discriminates resources when one module's hub carries several. */
  entityType: string
  entityId: number
  /** The row is gone: drop it from caches instead of refetching a 404. */
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
    return () => {
      get().listeners.delete(fn)
    }
  },
}))

// Per-module WS connections. The map keys by module so two pages from
// different modules can watch in parallel without one closing the
// other's socket. Each connection reconnects on its own.
type Connection = {
  /**
   * Close handle from openReconnectingSocket, which owns the socket and
   * the retry policy. null = nothing open for this module.
   */
  close: (() => void) | null
  scopeKey: string
  /**
   * Mounted subscribers (pages/components). When it drops to zero the
   * connection closes after a short linger, so navigating list -> detail
   * within one module does not bounce the socket.
   */
  refCount: number
  idleCloseTimer: ReturnType<typeof setTimeout> | null
}
const connections = new Map<string, Connection>()

const IDLE_CLOSE_DELAY_MS = 5000

function dispatch(event: StatusEvent) {
  useStatusWatch.getState().listeners.forEach((fn) => fn(event))
}

function connect(module: string, wsUrl: string, scopeKey: string) {
  const c = connections.get(module) || {
    close: null,
    scopeKey: "",
    refCount: 0,
    idleCloseTimer: null,
  }
  if (c.close) {
    c.close()
    c.close = null
  }

  c.scopeKey = scopeKey
  c.close = openReconnectingSocket(
    () => wsUrl,
    {
      onMessage: (e) => {
        // Frames are [type][payload]; 0x00 is MsgData (lib/websocket).
        const arr = new Uint8Array(e.data as ArrayBuffer)
        if (arr[0] !== 0x00) return
        try {
          dispatch(JSON.parse(new TextDecoder().decode(arr.slice(1))))
        } catch {
          /* a frame we cannot parse is not worth a broken page */
        }
      },
    },
    { binaryType: "arraybuffer" },
  )
  connections.set(module, c)
}

/**
 * Subscribe to one module's resource-collection /watch endpoint.
 *
 * Returns a release function; call it from the effect cleanup. Listeners
 * are registered separately (useStatusWatch.subscribe) and are shared
 * across modules, so they should check event.entityType.
 */
export function connectModuleWatch(
  module: string,
  resource: string,
  scopeWorkspaceId?: string,
  scopeNamespaceId?: string,
): () => void {
  const scopeKey = `${resource}:${scopeWorkspaceId || ""}:${scopeNamespaceId || ""}`
  let c = connections.get(module)
  if (!c) {
    c = { close: null, scopeKey: "", refCount: 0, idleCloseTimer: null }
    connections.set(module, c)
  }
  c.refCount++
  if (c.idleCloseTimer) {
    clearTimeout(c.idleCloseTimer)
    c.idleCloseTimer = null
  }
  const sameScopeAlive = c.scopeKey === scopeKey && c.close !== null
  if (!sameScopeAlive) {
    connect(module, buildTaskWsUrl(module, resource, scopeWorkspaceId, scopeNamespaceId), scopeKey)
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
  if (c.idleCloseTimer) {
    clearTimeout(c.idleCloseTimer)
    c.idleCloseTimer = null
  }
  if (c.close) {
    c.close()
    c.close = null
  }
  connections.delete(module)
}
