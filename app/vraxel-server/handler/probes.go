package handler

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"vraxel.io/vraxel/lib/logger"
)

const readinessTimeout = 2 * time.Second

type ReadinessCheck struct {
	Name string
	Fn   func(ctx context.Context) error
}

func writeLivez(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	_, _ = w.Write([]byte("OK"))
}

func writeReadiness(w http.ResponseWriter, r *http.Request, checks []ReadinessCheck) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")

	ctx, cancel := context.WithTimeout(r.Context(), readinessTimeout)
	defer cancel()

	var b strings.Builder
	failed := 0
	for _, c := range checks {
		if err := c.Fn(ctx); err != nil {
			failed++
			logger.Warnf("readiness check %q failed: %v", c.Name, err)
			fmt.Fprintf(&b, "[-]%s failed\n", c.Name)
			continue
		}
		fmt.Fprintf(&b, "[+]%s ok\n", c.Name)
	}

	if failed > 0 {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(b.String()))
		return
	}
	if b.Len() == 0 {
		_, _ = w.Write([]byte("OK"))
		return
	}
	_, _ = w.Write([]byte(b.String()))
}
