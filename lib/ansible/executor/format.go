package executor

import (
	"fmt"
	"strings"
)

// FormatEventText renders an executor.Event as Ansible-style terminal
// text (CRLF line endings, xterm-ready). Returns "" for events that
// should not produce output.
//
// Shared across every PaaS progress WebSocket handler (db, mq, mw, dev)
// so they all show the same log format, including EventLog lines from
// non-Ansible orchestration paths (e.g. cloud VM creation).
func FormatEventText(ev Event) string {
	switch ev.Type {
	case EventPlayStart:
		return fmt.Sprintf("\r\nPLAY [%s] ***\r\n\r\n", ev.Play)
	case EventTaskStart:
		return fmt.Sprintf("TASK [%s] ...\r\n", ev.Task)
	case EventTaskEnd:
		dur := ""
		if ev.Duration > 0 {
			dur = fmt.Sprintf(" (%dms)", ev.Duration)
		}
		switch ev.Status {
		case "failed":
			return fmt.Sprintf("  => FAILED%s: %s\r\n", dur, ev.Error)
		case "skipped":
			return "  => SKIPPED\r\n"
		default:
			return fmt.Sprintf("  => %s%s\r\n", strings.ToUpper(ev.Status), dur)
		}
	case EventLog:
		// Non-Ansible log line from async orchestration (e.g. cloud
		// VM creation). Normalize newlines to CRLF so xterm renders
		// them correctly.
		msg := ev.Message
		if msg == "" {
			return ""
		}
		msg = strings.ReplaceAll(msg, "\r\n", "\n")
		msg = strings.ReplaceAll(msg, "\n", "\r\n")
		return msg
	default:
		return ""
	}
}
