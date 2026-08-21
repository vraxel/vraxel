import { useEffect, useRef, useState } from "react"
import { Terminal } from "@xterm/xterm"
import { FitAddon } from "@xterm/addon-fit"
import "@xterm/xterm/css/xterm.css"
import { RotateCw } from "lucide-react"
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/shared/ui/dialog"
import { Badge } from "@/shared/ui/badge"
import { Button } from "@/shared/ui/button"
import { Input } from "@/shared/ui/input"
import { Label } from "@/shared/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select"
import { Switch } from "@/shared/ui/switch"
import { terminalTheme } from "@/shared/lib/terminal-theme"
import { xtermClipboardHandler } from "@/shared/lib/xterm-clipboard"
import { translate, useTranslation } from "@/i18n"
import type { ScopeRef } from "@/core/registry/resource"
import type { Host } from "@/modules/compute/api/types"

// Wire message types. Must match lib/websocket/message.go.
const MSG_DATA = 0x00
const MSG_STATUS = 0x03

type ConnectionStatus = "idle" | "connecting" | "connected" | "closed" | "error"

type LogSource = "journal" | "kernel" | "file"

// The priorities the server accepts, most severe first. journalctl -p N
// means "N and more severe", which is what a viewer filter wants.
const PRIORITIES = ["err", "warning", "notice", "info", "debug"] as const

const TAIL_OPTIONS = ["200", "500", "1000", "5000"] as const

function logsUrl(
  scope: ScopeRef,
  hostId: string,
  q: {
    source: LogSource
    unit: string
    priority: string
    path: string
    tail: string
    follow: boolean
  },
): string {
  const protocol = location.protocol === "https:" ? "wss:" : "ws:"
  let scopePath = ""
  if (scope.ws && scope.ns) {
    scopePath = `workspaces/${scope.ws}/namespaces/${scope.ns}/`
  } else if (scope.ws) {
    scopePath = `workspaces/${scope.ws}/`
  }
  const params = new URLSearchParams({ source: q.source, tail: q.tail, follow: String(q.follow) })
  if (q.source === "journal" && q.unit) params.set("unit", q.unit)
  if (q.source !== "file" && q.priority) params.set("priority", q.priority)
  if (q.source === "file") params.set("path", q.path)
  return `${protocol}//${location.host}/api/compute/v1/${scopePath}hosts/${hostId}/logs?${params}`
}

/**
 * A read-only live view of a host's logs -- journalctl or a file tail
 * run by the agent, streamed over the host's own outbound connection.
 *
 * Same single-effect shape as the terminal dialog, for the same reason:
 * the xterm instance and the socket are one resource, and a socket that
 * outlived the dialog would hold a follow process on a real machine.
 * Every stream parameter is an effect dependency, so changing one tears
 * the old stream down and starts a fresh one -- a different source is a
 * different stream, not more lines in the same scrollback.
 */
export function HostLogsDialog({
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
  const { t } = useTranslation()

  const [source, setSource] = useState<LogSource>("journal")
  const [priority, setPriority] = useState("")
  const [tail, setTail] = useState<string>("500")
  const [follow, setFollow] = useState(true)
  // The text fields commit on Enter or the refresh button, not per
  // keystroke: every committed change restarts the stream on the host.
  const [unitInput, setUnitInput] = useState("")
  const [pathInput, setPathInput] = useState("")
  // The committed values live in one object because its identity is the
  // reconnect signal: committing always makes a fresh object, so the
  // stream effect reruns even when the text is unchanged -- which is
  // also how the refresh button reopens a closed session.
  const [committed, setCommitted] = useState({ unit: "", path: "" })

  const applyAndReconnect = () => {
    setCommitted({ unit: unitInput.trim(), path: pathInput.trim() })
  }

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

    // Deferred to the next frame for the same reason as the terminal:
    // the container exists but has no layout yet, and the first fit
    // decides the width the first lines wrap at.
    const frame = requestAnimationFrame(() => {
      const container = containerRef.current
      if (!container) return

      terminal = new Terminal({
        disableStdin: true,
        cursorBlink: false,
        // Log lines end in a bare \n; without this every line would
        // staircase rightwards.
        convertEol: true,
        fontSize: 13,
        fontFamily: "Menlo, Monaco, 'Courier New', monospace",
        scrollback: 10000,
        theme: terminalTheme,
      })
      fitAddon = new FitAddon()
      terminal.loadAddon(fitAddon)
      terminal.attachCustomKeyEventHandler(xtermClipboardHandler(terminal))
      terminal.open(container)
      fit(fitAddon)

      onWindowResize = () => fit(fitAddon)
      window.addEventListener("resize", onWindowResize)
      resizeObserver = new ResizeObserver(() => fit(fitAddon))
      resizeObserver.observe(container)

      // The file source has nothing to stream until a path is committed;
      // say so instead of opening a socket the server would reject.
      if (source === "file" && !committed.path) {
        setStatus("idle")
        terminal.write(`\x1b[90m${translate("compute.host.logs.needPath")}\x1b[0m\r\n`)
        return
      }

      socket = new WebSocket(
        logsUrl({ ws: scopeWs, ns: scopeNs }, hostId, {
          source,
          unit: committed.unit,
          priority,
          path: committed.path,
          tail,
          follow,
        }),
      )
      socket.binaryType = "arraybuffer"

      socket.onmessage = (event) => {
        const data = new Uint8Array(event.data as ArrayBuffer)
        const type = data[0]
        const payload = data.slice(1)
        if (type === MSG_DATA) {
          // Raw bytes, not a decoded string: xterm's stateful UTF-8
          // decoder is what keeps a multibyte character split across two
          // chunks intact. xterm pins the viewport to the bottom only
          // while it is already there, so a reader who scrolled up is
          // not yanked back down by new lines.
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
            // Into the terminal, not the badge, so the full sentence
            // survives the badge's nowrap clipping.
            terminal?.write(
              `\r\n\x1b[31m${parsed.message ?? translate("compute.host.logs.failed")}\x1b[0m\r\n`,
            )
            break
          case "timeout":
            setStatus("closed")
            terminal?.write(`\r\n\x1b[33m${translate("compute.host.logs.wall")}\x1b[0m\r\n`)
            break
          case "exited":
            setStatus("closed")
            terminal?.write(
              `\r\n\x1b[90m${parsed.message ?? translate("compute.host.logs.ended")}\x1b[0m\r\n`,
            )
            break
        }
      }

      socket.onclose = () => {
        setStatus((prev) =>
          prev === "connecting" ? "error" : prev === "connected" ? "closed" : prev,
        )
        setErrorMessage((prev) => prev || translate("compute.host.logs.closed"))
      }
      socket.onerror = () => {
        setStatus("error")
        setErrorMessage(translate("compute.host.logs.failed"))
      }
    })

    return () => {
      cancelAnimationFrame(frame)
      if (onWindowResize) window.removeEventListener("resize", onWindowResize)
      resizeObserver?.disconnect()
      if (socket) {
        socket.onmessage = null
        socket.onclose = null
        socket.onerror = null
        if (socket.readyState !== WebSocket.CLOSED) socket.close()
      }
      terminal?.dispose()
      setStatus("connecting")
      setErrorMessage("")
    }
  }, [open, hostId, scopeWs, scopeNs, source, priority, tail, follow, committed])

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="flex h-[80vh] w-[80vw] flex-col gap-3 sm:max-w-[80vw]"
        aria-describedby={undefined}
        onOpenAutoFocus={(e) => e.preventDefault()}
      >
        <DialogHeader className="flex-row items-center gap-3 space-y-0">
          <DialogTitle className="truncate">
            {t("compute.host.logs.title")} - {host.metadata.name}
          </DialogTitle>
          <StatusBadge status={status} message={errorMessage} />
        </DialogHeader>

        <div className="flex flex-wrap items-center gap-2">
          <Select value={source} onValueChange={(v) => setSource(v as LogSource)}>
            <SelectTrigger className="w-36">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="journal">{t("compute.host.logs.sourceJournal")}</SelectItem>
              <SelectItem value="kernel">{t("compute.host.logs.sourceKernel")}</SelectItem>
              <SelectItem value="file">{t("compute.host.logs.sourceFile")}</SelectItem>
            </SelectContent>
          </Select>

          {source === "journal" && (
            <Input
              className="w-56"
              value={unitInput}
              onChange={(e) => setUnitInput(e.target.value)}
              onKeyDown={(e) => e.key === "Enter" && applyAndReconnect()}
              placeholder={t("compute.host.logs.unitPlaceholder")}
            />
          )}

          {source === "file" && (
            <Input
              className="w-72 font-mono"
              value={pathInput}
              onChange={(e) => setPathInput(e.target.value)}
              onKeyDown={(e) => e.key === "Enter" && applyAndReconnect()}
              placeholder={t("compute.host.logs.pathPlaceholder")}
            />
          )}

          {source !== "file" && (
            <Select
              // Radix reserves "" for "nothing selected", so "all" is the
              // sentinel and is stripped before it reaches the URL.
              value={priority || "all"}
              onValueChange={(v) => setPriority(v === "all" ? "" : v)}
            >
              <SelectTrigger className="w-32">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">{t("compute.host.logs.priorityAll")}</SelectItem>
                {PRIORITIES.map((p) => (
                  <SelectItem key={p} value={p}>
                    {p + "+"}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}

          <Select value={tail} onValueChange={setTail}>
            <SelectTrigger className="w-24">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {TAIL_OPTIONS.map((n) => (
                <SelectItem key={n} value={n}>
                  {n}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>

          <Label className="flex items-center gap-2 text-sm font-normal">
            <Switch checked={follow} onCheckedChange={setFollow} />
            {t("compute.host.logs.follow")}
          </Label>

          <Button variant="outline" size="sm" onClick={applyAndReconnect}>
            <RotateCw className="size-4" />
            {t("compute.host.logs.refresh")}
          </Button>
        </div>

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
  if (status === "idle") return null
  if (status === "connected") {
    return <Badge variant="outline">{t("compute.host.logs.connected")}</Badge>
  }
  if (status === "connecting") {
    return <Badge variant="outline">{t("compute.host.logs.connecting")}</Badge>
  }
  if (status === "error") {
    return <Badge variant="destructive">{message || t("compute.host.logs.failed")}</Badge>
  }
  return <Badge variant="secondary">{t("compute.host.logs.closed")}</Badge>
}

// fit throws when the container has no layout yet; the ResizeObserver
// retries once it does.
function fit(addon: FitAddon | null) {
  try {
    addon?.fit()
  } catch {
    // retried on the next resize
  }
}
