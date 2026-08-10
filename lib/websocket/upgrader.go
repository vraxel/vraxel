package websocket

import (
	"net/http"

	cws "github.com/coder/websocket"
)

// AcceptOptions configures the WebSocket upgrade.
type AcceptOptions struct {
	// InsecureSkipVerify disables origin verification. Use only for
	// development or when origin checking is handled elsewhere.
	InsecureSkipVerify bool
	// Subprotocols lists the sub-protocols the server is willing to
	// negotiate. Empty means accept any (no protocol in response).
	Subprotocols []string
}

// Accept upgrades an HTTP request to a WebSocket connection.
// If opts is nil, default options are used (origin verification enabled).
func Accept(w http.ResponseWriter, r *http.Request, opts *AcceptOptions) (*Conn, error) {
	var cwsOpts *cws.AcceptOptions
	if opts != nil {
		cwsOpts = &cws.AcceptOptions{
			InsecureSkipVerify: opts.InsecureSkipVerify,
			Subprotocols:       opts.Subprotocols,
		}
	}

	c, err := cws.Accept(w, r, cwsOpts)
	if err != nil {
		return nil, err
	}
	return NewConn(c), nil
}
