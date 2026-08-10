// Package rest holds the HTTP wire-layer primitives the apiserver
// framework composes per route: request decoding (DecodeBody, ParseID,
// ParseListOptions, PathParams), negotiated response writing
// (WriteObjectNegotiated, ErrorNegotiated, WriteRawJSON, WriteFile),
// deferred response headers, and the WebSocket upgrade contract.
//
// It sits between lib/runtime (codecs) and lib/apiserver (routing +
// registration): rest knows HTTP but not resources; apiserver knows
// resources and delegates every byte on the wire to rest. Handlers in
// pkg/apis may use the request-parsing helpers directly.
package rest
