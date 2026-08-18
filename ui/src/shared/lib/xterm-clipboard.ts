import type { Terminal } from "@xterm/xterm"

/**
 * Restore Ctrl+C / Ctrl+V on Windows and Linux.
 *
 * xterm claims both: Ctrl+C becomes \x03 (SIGINT) and Ctrl+V becomes a
 * control character, so neither reaches the browser's native clipboard.
 * That is correct for Ctrl+C with nothing selected -- interrupting a
 * running command is what the key is for -- and wrong for every other
 * case. macOS is unaffected, its Cmd+C/V arrive as metaKey and xterm
 * lets those through.
 *
 * So: Ctrl+V always defers to the browser, and Ctrl+C defers only when
 * there is a selection to copy.
 */
export function xtermClipboardHandler(terminal: Terminal) {
  return (event: KeyboardEvent): boolean => {
    if (event.type !== "keydown") return true
    if (!event.ctrlKey || event.altKey || event.metaKey) return true

    // Let the browser fire its own paste event; xterm receives the text
    // through onData like any other input.
    if (event.key === "v" || event.key === "V") return false

    if ((event.key === "c" || event.key === "C") && terminal.hasSelection()) {
      const text = terminal.getSelection()
      if (text) copy(text)
      return false
    }

    return true
  }
}

// navigator.clipboard is undefined outside a secure context, and an
// on-premise deployment reached over plain http at an IP address is
// exactly that -- which is the common case for this product, not an edge
// one. execCommand is deprecated and still the only thing that works
// there.
function copy(text: string) {
  const viaClipboardApi = navigator.clipboard?.writeText(text)
  if (viaClipboardApi) {
    viaClipboardApi.catch(() => copyViaTextarea(text))
    return
  }
  copyViaTextarea(text)
}

function copyViaTextarea(text: string) {
  try {
    const ta = document.createElement("textarea")
    ta.value = text
    ta.style.position = "fixed"
    ta.style.left = "-9999px"
    document.body.appendChild(ta)
    ta.select()
    document.execCommand("copy")
    document.body.removeChild(ta)
  } catch {
    // Nothing left to try, and failing to copy is not worth interrupting
    // a working terminal over.
  }
}
