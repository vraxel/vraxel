package nodemetrics

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
)

// slogTo adapts the agent's Logger to the *slog.Logger the collector
// package wants.
//
// Only warnings and errors pass. The collectors log a debug line per
// collector per round -- forty-odd lines every fifteen seconds -- and an
// agent on somebody else's machine must not chat like that. Failures are
// worth a line; they are also independently visible in the data, as
// node_scrape_collector_success 0.
func slogTo(log Logger) *slog.Logger {
	return slog.New(logHandler{log: log})
}

type logHandler struct {
	log   Logger
	attrs string
}

func (h logHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= slog.LevelWarn
}

func (h logHandler) Handle(_ context.Context, r slog.Record) error {
	var b strings.Builder
	b.WriteString(r.Message)
	b.WriteString(h.attrs)
	r.Attrs(func(a slog.Attr) bool {
		fmt.Fprintf(&b, " %s=%v", a.Key, a.Value.Any())
		return true
	})
	h.log.Warnf("node metrics: %s", b.String())
	return nil
}

func (h logHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	var b strings.Builder
	b.WriteString(h.attrs)
	for _, a := range attrs {
		fmt.Fprintf(&b, " %s=%v", a.Key, a.Value.Any())
	}
	return logHandler{log: h.log, attrs: b.String()}
}

func (h logHandler) WithGroup(string) slog.Handler { return h }
