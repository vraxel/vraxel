import { clsx, type ClassValue } from "clsx"
import { twMerge } from "tailwind-merge"

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

/**
 * Format a byte count for storage UI: GiB with 1 decimal when >= 1 GiB,
 * MiB rounded to integer below 1 GiB, "0" for non-positive input.
 * Used by both the disk tab table and the basic-info disk usage strip
 * so identical filesystems render with identical text.
 */
export function fmtStorageBytes(bytes: number): string {
  if (bytes <= 0) return "0"
  const gib = bytes / (1024 * 1024 * 1024)
  if (gib >= 1) return `${gib.toFixed(1)} GiB`
  const mib = bytes / (1024 * 1024)
  return `${Math.round(mib)} MiB`
}

/**
 * Copy text to clipboard with fallback for non-secure contexts (HTTP).
 * Tries navigator.clipboard first, falls back to execCommand('copy').
 *
 * IMPORTANT: When called from inside a Radix Dialog (or any FocusScope-trapped
 * container), the temporary textarea MUST be appended inside the dialog itself.
 * Otherwise FocusScope's `focusin` listener will yank focus back to the previous
 * element on `ta.focus()`, deselecting the textarea and making
 * `execCommand("copy")` read an empty selection — silent failure.
 */
export async function copyToClipboard(text: string): Promise<void> {
  if (navigator.clipboard && window.isSecureContext) {
    try {
      await navigator.clipboard.writeText(text)
      return
    } catch {
      // clipboard API denied — fall through to execCommand fallback
    }
  }
  const ta = document.createElement("textarea")
  ta.value = text
  ta.setAttribute("readonly", "")
  ta.style.position = "fixed"
  ta.style.left = "-9999px"
  ta.style.opacity = "0"
  // If a Radix Dialog (or compatible FocusScope container) is currently open,
  // append the textarea inside it so the focus trap doesn't interfere.
  // Use the LAST open dialog to handle nested dialogs.
  const openDialogs = document.querySelectorAll<HTMLElement>('[role="dialog"][data-state="open"]')
  const container = openDialogs[openDialogs.length - 1] ?? document.body
  container.appendChild(ta)
  ta.focus()
  ta.select()
  try {
    if (!document.execCommand("copy")) {
      throw new Error("copy failed")
    }
  } finally {
    container.removeChild(ta)
  }
}
