import { describe, it, expect, beforeEach, afterEach, vi } from "vitest"
import { connectModuleWatch, useStatusWatch, type StatusEvent } from "../status-watch"

// Confirms the StatusEvent.deleted lifecycle flag is dispatched
// untouched and that subscribers can branch on it. Replaces the
// legacy `event.status === "deleted"` sentinel pattern.
describe("StatusWatch.deleted", () => {
  beforeEach(() => {
    useStatusWatch.getState().listeners.clear()
  })

  it("dispatches event with deleted=true to subscribers", () => {
    const received: StatusEvent[] = []
    const unsub = useStatusWatch.getState().subscribe((ev) => received.push(ev))

    const ev: StatusEvent = {
      entityType: "mysql",
      entityId: 42,
      status: "",
      phase: "",
      opKind: "",
      deleted: true,
    }
    useStatusWatch.getState().listeners.forEach((fn) => fn(ev))

    expect(received).toHaveLength(1)
    expect(received[0].deleted).toBe(true)
    expect(received[0].entityId).toBe(42)
    unsub()
  })

  it("non-delete events have undefined deleted field", () => {
    const received: StatusEvent[] = []
    const unsub = useStatusWatch.getState().subscribe((ev) => received.push(ev))

    useStatusWatch.getState().listeners.forEach((fn) => fn({
      entityType: "mysql",
      entityId: 1,
      status: "running",
    }))

    expect(received[0].deleted).toBeUndefined()
    unsub()
  })

  it("multiple listeners all receive deleted event independently", () => {
    const a: StatusEvent[] = []
    const b: StatusEvent[] = []
    const unsubA = useStatusWatch.getState().subscribe((ev) => a.push(ev))
    const unsubB = useStatusWatch.getState().subscribe((ev) => b.push(ev))

    useStatusWatch.getState().listeners.forEach((fn) => fn({
      entityType: "redis",
      entityId: 7,
      status: "",
      deleted: true,
    }))

    expect(a).toHaveLength(1)
    expect(b).toHaveLength(1)
    expect(a[0].deleted).toBe(true)
    expect(b[0].deleted).toBe(true)
    unsubA()
    unsubB()
  })
})

// Refcounted connection lifecycle: sockets close after the last holder
// releases (plus a short linger), instead of living for the whole
// session (the old leak: disconnectModuleWatch had zero callers).
// Each test uses a distinct module name because the module-level
// `connections` map is shared across tests (static import, no reset).
describe("StatusWatch connection refcount", () => {
  class FakeWebSocket {
    static instances: FakeWebSocket[] = []
    static CONNECTING = 0
    static OPEN = 1
    static CLOSING = 2
    static CLOSED = 3
    url: string
    binaryType = ""
    readyState = 1
    onmessage: ((e: unknown) => void) | null = null
    onclose: (() => void) | null = null
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
    r1(); r2()
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
    const ws = FakeWebSocket.instances[0]
    expect(ws.readyState).toBe(3)
    expect(FakeWebSocket.instances).toHaveLength(1) // no 3s auto-reconnect
  })
})
