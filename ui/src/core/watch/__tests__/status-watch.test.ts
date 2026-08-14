import { describe, it, expect, beforeEach, afterEach, vi } from "vitest"
import { connectModuleWatch, useStatusWatch, type StatusEvent } from "../status-watch"

class FakeWebSocket {
  static instances: FakeWebSocket[] = []
  static CONNECTING = 0
  static OPEN = 1
  static CLOSING = 2
  static CLOSED = 3
  url: string
  binaryType = ""
  readyState = 1
  onopen: (() => void) | null = null
  onmessage: ((e: unknown) => void) | null = null
  onclose: (() => void) | null = null
  onerror: (() => void) | null = null
  constructor(url: string) {
    this.url = url
    FakeWebSocket.instances.push(this)
  }
  close() {
    this.readyState = 3
    // Browsers dispatch the close event asynchronously; mirroring that
    // is what exposes stale-onclose bugs (a replaced socket's close
    // firing after the replacement socket was opened).
    setTimeout(() => this.onclose?.(), 0)
  }
}

/** One server frame: [msgType][json]. 0x00 is MsgData. */
function frame(msgType: number, body: string): { data: ArrayBuffer } {
  const payload = new TextEncoder().encode(body)
  const buf = new Uint8Array(payload.length + 1)
  buf[0] = msgType
  buf.set(payload, 1)
  return { data: buf.buffer }
}

// The wire format is the contract between lib/websocket's framing and
// this decoder. Every test here drives it through the socket rather than
// calling listeners directly, because "the listener runs when someone
// calls it" is not a fact about the code under test.
describe("StatusWatch frame decoding", () => {
  let received: StatusEvent[]

  beforeEach(() => {
    vi.useFakeTimers()
    FakeWebSocket.instances = []
    vi.stubGlobal("WebSocket", FakeWebSocket)
    useStatusWatch.getState().listeners.clear()
    received = []
  })
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.useRealTimers()
  })

  function open(module: string) {
    useStatusWatch.getState().subscribe((ev) => received.push(ev))
    const release = connectModuleWatch(module, "hosts/watch")
    return { socket: FakeWebSocket.instances[0], release }
  }

  it("dispatches a data frame to every listener", () => {
    const second: StatusEvent[] = []
    useStatusWatch.getState().subscribe((ev) => second.push(ev))
    const { socket, release } = open("t-decode")

    socket.onmessage?.(frame(0x00, JSON.stringify({ entityType: "host", entityId: 42 })))

    expect(received).toEqual([{ entityType: "host", entityId: 42 }])
    expect(second).toHaveLength(1)
    release()
  })

  it("carries the deleted flag through", () => {
    const { socket, release } = open("t-deleted")

    socket.onmessage?.(
      frame(0x00, JSON.stringify({ entityType: "host", entityId: 7, deleted: true })),
    )

    expect(received[0].deleted).toBe(true)
    release()
  })

  it("ignores frames that are not MsgData", () => {
    const { socket, release } = open("t-msgtype")

    socket.onmessage?.(frame(0x01, JSON.stringify({ entityType: "host", entityId: 1 })))

    expect(received).toHaveLength(0)
    release()
  })

  it("survives a malformed payload", () => {
    const { socket, release } = open("t-garbage")

    expect(() => socket.onmessage?.(frame(0x00, "{not json"))).not.toThrow()
    expect(received).toHaveLength(0)
    release()
  })
})

// Refcounted connection lifecycle: sockets close after the last holder
// releases (plus a short linger), instead of living for the whole
// session (the old leak: disconnectModuleWatch had zero callers).
// Each test uses a distinct module name because the module-level
// `connections` map is shared across tests (static import, no reset).
describe("StatusWatch connection refcount", () => {
  beforeEach(() => {
    vi.useFakeTimers()
    FakeWebSocket.instances = []
    vi.stubGlobal("WebSocket", FakeWebSocket)
  })
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.useRealTimers()
  })

  it("closes the socket only after the last release + linger", () => {
    const r1 = connectModuleWatch("t-refcount", "x/watch")
    const r2 = connectModuleWatch("t-refcount", "x/watch")
    expect(FakeWebSocket.instances).toHaveLength(1)

    r1()
    vi.advanceTimersByTime(10_000)
    expect(FakeWebSocket.instances[0].readyState).toBe(1) // r2 still holds

    r2()
    vi.advanceTimersByTime(4_000)
    expect(FakeWebSocket.instances[0].readyState).toBe(1) // linger not elapsed
    vi.advanceTimersByTime(2_000)
    expect(FakeWebSocket.instances[0].readyState).toBe(3) // closed after linger
  })

  it("re-acquire within the linger keeps the same socket", () => {
    const r1 = connectModuleWatch("t-linger", "x/watch")
    r1()
    vi.advanceTimersByTime(2_000)
    const r2 = connectModuleWatch("t-linger", "x/watch")
    vi.advanceTimersByTime(60_000)
    expect(FakeWebSocket.instances).toHaveLength(1)
    expect(FakeWebSocket.instances[0].readyState).toBe(1)
    r2()
    vi.advanceTimersByTime(6_000)
    expect(FakeWebSocket.instances[0].readyState).toBe(3)
  })

  it("scope change replaces the socket and the replacement still closes on release", () => {
    const r1 = connectModuleWatch("t-scope", "x/watch")
    const r2 = connectModuleWatch("t-scope", "x/watch", "2")
    expect(FakeWebSocket.instances).toHaveLength(2)
    expect(FakeWebSocket.instances[0].readyState).toBe(3)
    expect(FakeWebSocket.instances[1].readyState).toBe(1)
    // The old socket's async close event must not null out / respawn
    // the replacement (stale-onclose guard).
    vi.advanceTimersByTime(1_000)
    expect(FakeWebSocket.instances).toHaveLength(2)
    expect(FakeWebSocket.instances[1].readyState).toBe(1)
    r1()
    r2()
    vi.advanceTimersByTime(6_000)
    expect(FakeWebSocket.instances[1].readyState).toBe(3) // replacement closed after linger
    expect(FakeWebSocket.instances).toHaveLength(2) // no zombie respawn
  })

  it("double release of one handle cannot steal another holder's ref", () => {
    const r1 = connectModuleWatch("t-double", "x/watch")
    const r2 = connectModuleWatch("t-double", "x/watch")
    r1()
    r1() // idempotent
    vi.advanceTimersByTime(60_000)
    expect(FakeWebSocket.instances[0].readyState).toBe(1)
    r2()
    vi.advanceTimersByTime(6_000)
    expect(FakeWebSocket.instances[0].readyState).toBe(3)
  })

  it("does not auto-reconnect after an idle close", () => {
    const r1 = connectModuleWatch("t-idle", "x/watch")
    r1()
    vi.advanceTimersByTime(60_000) // linger close + async close event
    expect(FakeWebSocket.instances[0].readyState).toBe(3)
    expect(FakeWebSocket.instances).toHaveLength(1) // no reconnect after a deliberate close
  })

  // A server restart drops every open socket. The retry has to come back
  // on its own, and it has to back off: the pre-port version retried on a
  // fixed 3s timer, so every tab in the building knocked in unison.
  it("reconnects with backoff after the server drops the connection", () => {
    const release = connectModuleWatch("t-drop", "x/watch")
    FakeWebSocket.instances[0].readyState = 3
    FakeWebSocket.instances[0].onclose?.()

    expect(FakeWebSocket.instances).toHaveLength(1) // not immediately
    vi.advanceTimersByTime(2_000) // first retry is ~1s +/- jitter
    expect(FakeWebSocket.instances).toHaveLength(2)

    release()
    vi.advanceTimersByTime(60_000)
    expect(FakeWebSocket.instances[1].readyState).toBe(3)
  })
})
