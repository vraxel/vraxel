import { describe, it, expect, vi, beforeEach, afterEach } from "vitest"
import {
  openReconnectingSocket,
  backoffDelay,
  DEFAULT_BASE_DELAY_MS,
  DEFAULT_MAX_DELAY_MS,
  JITTER,
} from "../reconnecting-socket"

class FakeWebSocket {
  static instances: FakeWebSocket[] = []
  static CONNECTING = 0
  static OPEN = 1
  static CLOSING = 2
  static CLOSED = 3
  url: string
  binaryType = ""
  readyState = 0
  onopen: (() => void) | null = null
  onmessage: ((e: unknown) => void) | null = null
  onclose: (() => void) | null = null
  onerror: (() => void) | null = null
  constructor(url: string) {
    this.url = url
    FakeWebSocket.instances.push(this)
  }
  /** Test helper: the server accepted the handshake. */
  open() {
    this.readyState = 1
    this.onopen?.()
  }
  /** Test helper: the connection dropped from the far end. */
  drop() {
    this.readyState = 3
    this.onclose?.()
  }
  close() {
    this.readyState = 3
    // Browsers dispatch close asynchronously; mirroring that is what exposes
    // a replaced socket's close firing after its replacement exists.
    setTimeout(() => this.onclose?.(), 0)
  }
}

const latest = () => FakeWebSocket.instances[FakeWebSocket.instances.length - 1]

describe("backoffDelay", () => {
  it("doubles per attempt and caps", () => {
    // Pin the jitter to its midpoint so the exponential part is observable.
    vi.spyOn(Math, "random").mockReturnValue(0.5)
    expect(backoffDelay(1)).toBe(1_000)
    expect(backoffDelay(2)).toBe(2_000)
    expect(backoffDelay(3)).toBe(4_000)
    expect(backoffDelay(4)).toBe(8_000)
    // 2**9 * 1000 would be 512s; the cap holds it at 30s.
    expect(backoffDelay(10)).toBe(DEFAULT_MAX_DELAY_MS)
    vi.mocked(Math.random).mockRestore()
  })

  it("keeps jitter within +/- JITTER of the base", () => {
    vi.spyOn(Math, "random").mockReturnValue(0)
    expect(backoffDelay(1)).toBe(DEFAULT_BASE_DELAY_MS * (1 - JITTER))
    vi.mocked(Math.random).mockReturnValue(0.999999)
    expect(backoffDelay(1)).toBeCloseTo(DEFAULT_BASE_DELAY_MS * (1 + JITTER), -1)
    vi.mocked(Math.random).mockRestore()
  })

  it("spreads concurrent sockets so they do not share a beat", () => {
    // The point of jitter: N sockets that dropped together must not all retry
    // in the same tick.
    const delays = new Set(Array.from({ length: 50 }, () => backoffDelay(3)))
    expect(delays.size).toBeGreaterThan(1)
  })
})

describe("openReconnectingSocket", () => {
  beforeEach(() => {
    vi.useFakeTimers()
    FakeWebSocket.instances = []
    vi.stubGlobal("WebSocket", FakeWebSocket)
    // Midpoint jitter keeps the timings below exact.
    vi.spyOn(Math, "random").mockReturnValue(0.5)
  })
  afterEach(() => {
    vi.mocked(Math.random).mockRestore()
    vi.unstubAllGlobals()
    vi.useRealTimers()
  })

  it("connects synchronously, so callers can assert right after the call", () => {
    // Regression guard: an earlier draft awaited a token refresh before
    // creating the socket, which pushed creation into a microtask and broke
    // every consumer that assumed it was already open.
    openReconnectingSocket(() => "ws://x/1", { onMessage: () => {} })
    expect(FakeWebSocket.instances).toHaveLength(1)
    expect(latest().url).toBe("ws://x/1")
  })

  it("backs off exponentially instead of retrying on a fixed beat", () => {
    openReconnectingSocket(() => "ws://x/1", { onMessage: () => {} })
    latest().drop()

    // 1s, then 2s, then 4s -- the old code retried at 3s, 3s, 3s.
    vi.advanceTimersByTime(999)
    expect(FakeWebSocket.instances).toHaveLength(1)
    vi.advanceTimersByTime(1)
    expect(FakeWebSocket.instances).toHaveLength(2)

    latest().drop()
    vi.advanceTimersByTime(1_999)
    expect(FakeWebSocket.instances).toHaveLength(2)
    vi.advanceTimersByTime(1)
    expect(FakeWebSocket.instances).toHaveLength(3)

    latest().drop()
    vi.advanceTimersByTime(4_000)
    expect(FakeWebSocket.instances).toHaveLength(4)
  })

  it("resets the backoff after a successful open", () => {
    openReconnectingSocket(() => "ws://x/1", { onMessage: () => {} })
    latest().drop()
    vi.advanceTimersByTime(1_000)
    latest().drop()
    vi.advanceTimersByTime(2_000)
    expect(FakeWebSocket.instances).toHaveLength(3)

    // Third attempt succeeds; a later drop must start from 1s again, not 4s.
    latest().open()
    latest().drop()
    vi.advanceTimersByTime(1_000)
    expect(FakeWebSocket.instances).toHaveLength(4)
  })

  it("re-evaluates the url on every attempt", () => {
    let n = 0
    openReconnectingSocket(() => `ws://x/${++n}`, { onMessage: () => {} })
    expect(latest().url).toBe("ws://x/1")
    latest().drop()
    vi.advanceTimersByTime(1_000)
    expect(latest().url).toBe("ws://x/2")
  })

  it("stops reconnecting once the caller closes", () => {
    const close = openReconnectingSocket(() => "ws://x/1", { onMessage: () => {} })
    latest().drop()
    close()
    vi.advanceTimersByTime(60_000)
    expect(FakeWebSocket.instances).toHaveLength(1)
  })

  it("delivers no handlers after close", () => {
    const onMessage = vi.fn()
    const onClose = vi.fn()
    const close = openReconnectingSocket(() => "ws://x/1", { onMessage, onClose })
    const ws = latest()
    ws.open()
    close()
    // The socket's own close event lands a tick later, as in a browser.
    vi.advanceTimersByTime(10)
    ws.onmessage?.({ data: "late" })
    expect(onMessage).not.toHaveBeenCalled()
    expect(onClose).not.toHaveBeenCalled()
  })

  it("runs onClose before scheduling the retry", () => {
    const order: string[] = []
    openReconnectingSocket(() => "ws://x/1", {
      onMessage: () => {},
      onClose: () => {
        order.push(`close:${FakeWebSocket.instances.length}`)
      },
    })
    latest().drop()
    // onClose saw the world before the reconnect created socket #2.
    expect(order).toEqual(["close:1"])
    vi.advanceTimersByTime(1_000)
    expect(FakeWebSocket.instances).toHaveLength(2)
  })

  it("ignores a replaced socket's late close event", () => {
    const onClose = vi.fn()
    openReconnectingSocket(() => "ws://x/1", { onMessage: () => {}, onClose })
    const first = latest()
    first.drop()
    vi.advanceTimersByTime(1_000)
    expect(FakeWebSocket.instances).toHaveLength(2)

    // The retired socket fires close again; it must not schedule a second
    // reconnect on top of the live one.
    onClose.mockClear()
    first.onclose?.()
    expect(onClose).not.toHaveBeenCalled()
    vi.advanceTimersByTime(60_000)
    expect(FakeWebSocket.instances).toHaveLength(2)
  })

  it("reports status transitions", () => {
    const seen: string[] = []
    openReconnectingSocket(() => "ws://x/1", {
      onMessage: () => {},
      onStatusChange: (s) => seen.push(s),
    })
    latest().open()
    latest().drop()
    vi.advanceTimersByTime(1_000)
    expect(seen).toEqual(["connecting", "open", "closed", "connecting"])
  })

  it("routes onerror through close so there is one retry path", () => {
    openReconnectingSocket(() => "ws://x/1", { onMessage: () => {} })
    latest().onerror?.()
    // close() defers its event by a tick, then the 1s backoff runs.
    vi.advanceTimersByTime(1_001)
    expect(FakeWebSocket.instances).toHaveLength(2)
  })
})
