import { useEffect, useRef, useState } from "react"
import { Terminal } from "@xterm/xterm"
import { FitAddon } from "@xterm/addon-fit"
import "@xterm/xterm/css/xterm.css"
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/shared/ui/dialog"
import { Badge } from "@/shared/ui/badge"
import { terminalTheme } from "@/shared/lib/terminal-theme"
import { xtermClipboardHandler } from "@/shared/lib/xterm-clipboard"
import { translate, useTranslation } from "@/i18n"
import type { ScopeRef } from "@/core/registry/resource"
import type { Host } from "@/modules/compute/api/types"

// Wire message types. Must match lib/websocket/message.go.
const MSG_DATA = 0x00
const MSG_RESIZE = 0x01
const MSG_STATUS = 0x03

type ConnectionStatus = "connecting" | "connected" | "closed" | "error"

// The explicit ArrayBuffer parameter is load-bearing: a bare Uint8Array
// widens to ArrayBufferLike, which WebSocket.send does not accept because
// it could be backed by a SharedArrayBuffer.
function encodeMessage(type: number, payload: Uint8Array<ArrayBuffer>): Uint8Array<ArrayBuffer> {
  const msg = new Uint8Array(1 + payload.length)
  msg[0] = type
  msg.set(payload, 1)
  return msg
}

// Builds the terminal URL for the scope the operator is browsing. The
// scope segments are part of the path because the server resolves the
// host within them: a workspace URL cannot reach a host outside it, even
// with a correct id.
function terminalUrl(scope: ScopeRef, hostId: string, cols: number, rows: number): string {
  const protocol = location.protocol === "https:" ? "wss:" : "ws:"
  let scopePath = ""
  if (scope.ws && scope.ns) {
    scopePath = `workspaces/${scope.ws}/namespaces/${scope.ns}/`
  } else if (scope.ws) {
    scopePath = `workspaces/${scope.ws}/`
  }
  return `${protocol}//${location.host}/api/compute/v1/${scopePath}hosts/${hostId}/terminal?cols=${cols}&rows=${rows}`
}

/**
 * An interactive shell on a managed host, carried by the host's own
 * outbound agent connection.
 *
 * Everything lives in one effect keyed on the dialog being open, because
 * the terminal, the socket and their listeners are one resource: xterm
 * writes into a DOM node that only exists while the dialog is mounted,
 * and a socket that outlived it would hold a PTY open on a real machine.
 *
 * Which is also why the effect translates through the module-level
 * `translate` rather than the hook's `t`. `t` is rebuilt on every render,
 * so depending on it would tear this whole resource down and stand it
 * back up each time the page rerenders -- killing a live shell on a real
 * machine to redraw a badge. The strings it needs are written when an
 * event fires, which is exactly what the non-hook translator is for.
 */
export function HostTerminalDialog({
  host,
  scope,
  open,
  onOpenChange,
}: {
  host: Host
  scope: ScopeRef
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const containerRef = useRef<HTMLDivElement>(null)
  const [status, setStatus] = useState<ConnectionStatus>("connecting")
  const [errorMessage, setErrorMessage] = useState("")

  const hostId = host.metadata.id
  const scopeWs = scope.ws
  const scopeNs = scope.ns

  useEffect(() => {
    if (!open) return

    let terminal: Terminal | null = null
    let socket: WebSocket | null = null
    let fitAddon: FitAddon | null = null
    let resizeObserver: ResizeObserver | null = null
    let onWindowResize: (() => void) | null = null
    const disposables: { dispose: () => void }[] = []

    // Deferred to the next frame rather than run inline: the container is
    // in the DOM by now but has not been laid out, so fitting here would
    // size the PTY against a zero-width box. That first size is the one
    // that matters -- the shell draws its first prompt at the size the PTY
    // was created with, and a later resize does not redraw a line already
    // on screen.
    const frame = requestAnimationFrame(() => {
      const container = containerRef.current
      if (!container) return

      terminal = new Terminal({
        cursorBlink: true,
        fontSize: 14,
        fontFamily: "Menlo, Monaco, 'Courier New', monospace",
        scrollback: 5000,
        theme: terminalTheme,
      })
      fitAddon = new FitAddon()
      terminal.loadAddon(fitAddon)
      terminal.attachCustomKeyEventHandler(xtermClipboardHandler(terminal))
      terminal.open(container)
      fit(fitAddon)

      socket = new WebSocket(
        terminalUrl({ ws: scopeWs, ns: scopeNs }, hostId, terminal.cols, terminal.rows),
      )
      socket.binaryType = "arraybuffer"

      socket.onmessage = (event) => {
        const data = new Uint8Array(event.data as ArrayBuffer)
        const type = data[0]
        const payload = data.slice(1)
        if (type === MSG_DATA) {
          terminal?.write(payload)
          return
        }
        if (type !== MSG_STATUS) return
        let parsed: { status?: string; message?: string }
        try {
          parsed = JSON.parse(new TextDecoder().decode(payload))
        } catch {
          return
        }
        switch (parsed.status) {
          case "connected":
            setStatus("connected")
            terminal?.focus()
            break
          case "error":
            setStatus("error")
            // Into the terminal, not the badge: a Badge is
            // whitespace-nowrap + overflow-hidden, so "could not open a
            // terminal on this host" would be clipped to a few words and
            // the operator would never learn what went wrong.
            terminal?.write(
              `\r\n\x1b[31m${parsed.message ?? translate("compute.host.terminal.failed")}\x1b[0m\r\n`,
            )
            break
          case "timeout":
            setStatus("closed")
            terminal?.write(`\r\n\x1b[33m${translate("compute.host.terminal.idle")}\x1b[0m\r\n`)
            break
          case "exited":
            setStatus("closed")
            terminal?.write(`\r\n\x1b[90m${parsed.message ?? ""}\x1b[0m\r\n`)
            break
        }
      }

      // A close before any status frame means the upgrade itself failed
      // (no permission, host gone). After "connected" it is the ordinary
      // end of a session and says nothing worth showing.
      socket.onclose = () => {
        setStatus((prev) =>
          prev === "connecting" ? "error" : prev === "connected" ? "closed" : prev,
        )
        setErrorMessage((prev) => prev || translate("compute.host.terminal.closed"))
      }
      socket.onerror = () => {
        setStatus("error")
        setErrorMessage(translate("compute.host.terminal.failed"))
      }

      disposables.push(
        terminal.onData((input) => {
          if (socket?.readyState !== WebSocket.OPEN) return
          socket.send(encodeMessage(MSG_DATA, new TextEncoder().encode(input)))
        }),
      )
      disposables.push(
        terminal.onResize(({ cols, rows }) => {
          if (socket?.readyState !== WebSocket.OPEN) return
          socket.send(
            encodeMessage(MSG_RESIZE, new TextEncoder().encode(JSON.stringify({ cols, rows }))),
          )
        }),
      )

      onWindowResize = () => fit(fitAddon)
      window.addEventListener("resize", onWindowResize)
      // The container also changes size without the window doing so, when
      // the status line above it appears or wraps. Left unobserved, xterm
      // keeps a stale column count and long commands wrap early.
      resizeObserver = new ResizeObserver(() => fit(fitAddon))
      resizeObserver.observe(container)
    })

    return () => {
      cancelAnimationFrame(frame)
      if (onWindowResize) window.removeEventListener("resize", onWindowResize)
      resizeObserver?.disconnect()
      disposables.forEach((d) => d.dispose())
      if (socket) {
        // Detach before closing: our own close would otherwise come back
        // through onclose and be recorded as the session ending, which is
        // the state the next open would start from.
        socket.onmessage = null
        socket.onclose = null
        socket.onerror = null
        if (socket.readyState !== WebSocket.CLOSED) socket.close()
      }
      terminal?.dispose()
      // This component stays mounted between openings, so its state
      // outlives the session it describes. Left alone, reopening shows the
      // previous session's badge -- "Session ended", or last time's error --
      // over a terminal that is in fact connecting, for as long as it takes
      // the agent to bring its data channel up.
      setStatus("connecting")
      setErrorMessage("")
    }
  }, [open, hostId, scopeWs, scopeNs])

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="flex h-[80vh] w-[80vw] flex-col gap-3 sm:max-w-[80vw]"
        // Every other dialog here has a DialogDescription; a terminal has
        // nothing to say that the title and the status badge do not. Told
        // explicitly so Radix stops warning about the missing element on
        // every open, rather than pointing at one that is not there.
        aria-describedby={undefined}
        // Escape belongs to whatever is running in the shell -- leaving vim's
        // insert mode, quitting less. Closing the dialog on it would kill the
        // session instead. The X button is the way out.
        onEscapeKeyDown={(e) => e.preventDefault()}
        // Nothing in here is focusable when the dialog opens except the close
        // button (xterm's textarea does not exist until the next frame), so
        // the default autofocus lands there -- and Enter, the most natural
        // key to press at a terminal, would close the dialog. The terminal
        // takes focus itself once the session connects.
        onOpenAutoFocus={(e) => e.preventDefault()}
      >
        <DialogHeader className="flex-row items-center gap-3 space-y-0">
          <DialogTitle className="truncate">{host.metadata.name}</DialogTitle>
          <StatusBadge status={status} message={errorMessage} />
        </DialogHeader>
        <div
          ref={containerRef}
          className="min-h-0 flex-1 overflow-hidden rounded-md bg-[#1e1e2e] p-2"
        />
      </DialogContent>
    </Dialog>
  )
}

function StatusBadge({ status, message }: { status: ConnectionStatus; message: string }) {
  const { t } = useTranslation()
  if (status === "connected") {
    return <Badge variant="outline">{t("compute.host.terminal.connected")}</Badge>
  }
  if (status === "connecting") {
    return <Badge variant="outline">{t("compute.host.terminal.connecting")}</Badge>
  }
  if (status === "error") {
    return <Badge variant="destructive">{message || t("compute.host.terminal.failed")}</Badge>
  }
  return <Badge variant="secondary">{t("compute.host.terminal.closed")}</Badge>
}

// fit throws when the container has no layout yet. The ResizeObserver
// calls back once it does, so there is nothing to handle here.
function fit(addon: FitAddon | null) {
  try {
    addon?.fit()
  } catch {
    // retried on the next resize
  }
}
