package rest

import (
	"context"
	"net/http"
	"strings"

	"vraxel.io/vraxel/lib/logger"
	"vraxel.io/vraxel/lib/websocket"
)

// WebSocketHandler handles an upgraded WebSocket connection.
// The framework performs the HTTP → WebSocket upgrade; the handler
// receives the ready-to-use connection with path/query params.
type WebSocketHandler func(ctx context.Context, params map[string]string, conn *websocket.Conn)

// HandleWebSocket returns an http.HandlerFunc that upgrades the
// connection to WebSocket, extracts path and query params, and
// delegates to the WebSocketHandler.
func HandleWebSocket(handler WebSocketHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Echo back any sub-protocol the client requests. This is safe
		// for all existing handlers (exec, logs, agent) which don't use
		// sub-protocols, and required by the console handler whose wmks
		// client mandates a negotiated "binary" protocol.
		var subprotocols []string
		if sp := r.Header.Get("Sec-WebSocket-Protocol"); sp != "" {
			for _, p := range strings.Split(sp, ",") {
				subprotocols = append(subprotocols, strings.TrimSpace(p))
			}
		}
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			InsecureSkipVerify: true,
			Subprotocols:       subprotocols,
		})
		if err != nil {
			logger.Warnf("websocket upgrade failed for %s %s: %v", r.Method, r.URL.Path, err)
			return
		}

		params := mergeQueryParams(PathParams(r), r)
		handler(r.Context(), params, conn)
	}
}
