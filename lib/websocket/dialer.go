package websocket

import (
	"context"
	"net/http"

	cws "github.com/coder/websocket"
)

// DialOptions configures a client Dial. Kept minimal on purpose: the only
// caller (lcp-agent's control channel) needs an Authorization header, and
// exposing coder/websocket's full option surface would defeat the point of
// this wrapper.
type DialOptions struct {
	// HTTPHeader is sent on the upgrade request.
	HTTPHeader http.Header
	// HTTPClient performs the upgrade request. Callers set it to carry a
	// custom TLS trust store; nil uses the library default. It is the
	// only way a WebSocket dial can be told about a private CA, since
	// the handshake is an ordinary HTTPS request.
	HTTPClient *http.Client
}

// Dial opens a client WebSocket connection and wraps it in a Conn, so the
// client half gets the same keepalive + third-party isolation the server
// half already has via Accept. The HTTP handshake response is returned so
// callers can read StatusCode on failure.
//
// Callers must NOT close resp.Body: on a WebSocket handshake the underlying
// library takes it over on success and returns a NopCloser on failure, so
// there is never a body for the caller to close (any IDE resource-leak
// warning on the returned resp is a false positive).
func Dial(ctx context.Context, url string, opts *DialOptions) (*Conn, *http.Response, error) {
	var dopts *cws.DialOptions
	if opts != nil {
		dopts = &cws.DialOptions{HTTPHeader: opts.HTTPHeader, HTTPClient: opts.HTTPClient}
	}
	c, resp, err := cws.Dial(ctx, url, dopts)
	if err != nil {
		return nil, resp, err
	}
	return NewConn(c), resp, nil
}
