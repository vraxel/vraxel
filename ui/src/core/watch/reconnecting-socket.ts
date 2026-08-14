// A WebSocket that reconnects on its own, with the retry policy in one place.
//
// Seven call sites used to hand-roll the same twenty lines -- socket variable,
// `closed` flag, reconnect timer, onerror-to-close, readyState check in the
// cleanup -- each retrying on a fixed 3s timer. Two problems with that:
//
//   * A backend restart turns every open tab into a steady drumbeat. A kube
//     detail page alone holds four sockets (the object watch, the events tab,
//     the pods card, the global status hub); three tabs is twelve sockets
//     retrying every 3s for the length of the outage. Worse, they share one
//     beat, so the moment the server comes back they all reconnect in the same
//     instant -- a thundering herd aimed at a process that just started.
//
//   * Seven copies drift. They already had: an extra re-list on close in
//     use-resource-list (to re-anchor an expired resourceVersion), and a
//     stale-socket guard in status-watch. Nobody has all the fixes.
//
// Backoff is exponential with jitter and resets on a successful open, so a
// brief blip still recovers in about a second while a long outage settles to
// one attempt per 30s per socket.
//
// Deliberately no auth in here: the socket is created synchronously inside
// connect(), which is what callers (and their tests) assume. Refreshing a
// token before connecting would push creation into a microtask and change that
// for every consumer.

export type SocketStatus = "connecting" | "open" | "closed"

export interface ReconnectingSocketHandlers {
  /** Called after the socket opens; use it to send a subscribe/connect frame. */
  onOpen?: (socket: WebSocket) => void
  onMessage: (event: MessageEvent) => void
  onStatusChange?: (status: SocketStatus) => void
  /** Runs after each close, before the retry is scheduled. Use it for work the
   *  reconnect depends on -- e.g. re-listing to re-anchor a resourceVersion the
   *  apiserver has expired. Not called after the caller closes the socket. */
  onClose?: () => void
}

export interface ReconnectingSocketOptions {
  binaryType?: BinaryType
  /** Delay before the first retry. Doubles per consecutive failure. */
  baseDelayMs?: number
  /** Ceiling for the backoff. */
  maxDelayMs?: number
}

export const DEFAULT_BASE_DELAY_MS = 1_000
export const DEFAULT_MAX_DELAY_MS = 30_000
/** Jitter spread, +/- this fraction of the computed delay. */
export const JITTER = 0.2

/**
 * Exponential backoff with jitter. `attempt` is 1-based: the first retry after
 * a failure is attempt 1.
 *
 * The jitter is what keeps sockets from sharing a beat; without it every socket
 * that dropped together retries together forever.
 */
export function backoffDelay(
  attempt: number,
  baseMs = DEFAULT_BASE_DELAY_MS,
  maxMs = DEFAULT_MAX_DELAY_MS,
): number {
  const exp = Math.min(baseMs * 2 ** Math.max(attempt - 1, 0), maxMs)
  return Math.round(exp * (1 - JITTER + Math.random() * JITTER * 2))
}

/**
 * Opens `getUrl()` and keeps it open. `getUrl` is evaluated per attempt, so a
 * reconnect picks up whatever the current parameters are.
 *
 * Returns a close function; call it from the effect cleanup. After it runs, no
 * handler fires and no reconnect is scheduled.
 */
export function openReconnectingSocket(
  getUrl: () => string,
  handlers: ReconnectingSocketHandlers,
  options: ReconnectingSocketOptions = {},
): () => void {
  const { binaryType, baseDelayMs, maxDelayMs } = options
  let socket: WebSocket | null = null
  let retryTimer: ReturnType<typeof setTimeout> | null = null
  let attempt = 0
  let closed = false

  const connect = () => {
    if (closed) return
    handlers.onStatusChange?.("connecting")

    const ws = new WebSocket(getUrl())
    if (binaryType) ws.binaryType = binaryType
    socket = ws

    ws.onopen = () => {
      // A connection that got established resets the backoff, so the next
      // outage starts from baseDelayMs again rather than wherever the last
      // one left off.
      attempt = 0
      handlers.onStatusChange?.("open")
      handlers.onOpen?.(ws)
    }
    ws.onmessage = (event) => {
      if (!closed) handlers.onMessage(event)
    }
    ws.onclose = () => {
      // Browsers dispatch close asynchronously, so a socket replaced by a
      // reconnect can deliver its close after the replacement exists. Ignore
      // anything that is not the current socket.
      if (socket !== ws) return
      socket = null
      if (closed) return
      handlers.onStatusChange?.("closed")
      handlers.onClose?.()
      if (retryTimer) return
      attempt += 1
      retryTimer = setTimeout(
        () => {
          retryTimer = null
          connect()
        },
        backoffDelay(attempt, baseDelayMs, maxDelayMs),
      )
    }
    // onerror always precedes onclose, so letting close drive the retry keeps
    // a single path.
    ws.onerror = () => {
      try {
        ws.close()
      } catch {
        /* already closing */
      }
    }
  }

  connect()

  return () => {
    closed = true
    if (retryTimer) {
      clearTimeout(retryTimer)
      retryTimer = null
    }
    if (
      socket &&
      (socket.readyState === WebSocket.OPEN || socket.readyState === WebSocket.CONNECTING)
    ) {
      socket.close()
    }
    socket = null
  }
}
