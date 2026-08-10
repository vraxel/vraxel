package rest

import (
	"context"
	"fmt"
	"net/http"

	"vraxel.io/vraxel/lib/runtime"
)

type key int

const pathParamsKey key = iota

// WithPathParams returns a request whose context carries the matched
// path parameters. The apiserver route wrappers inject them so bridged
// WebSocket / raw handlers keep reading the v1 params map.
func WithPathParams(r *http.Request, pathParams map[string]string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), pathParamsKey, pathParams))
}

// PathParams returns the path parameters stored in the request context.
func PathParams(r *http.Request) map[string]string {
	if params, ok := r.Context().Value(pathParamsKey).(map[string]string); ok {
		return params
	}
	return map[string]string{}
}

// DecodeBody decodes the request body into a runtime.Object using the
// Content-Type header to select the appropriate serializer. Returns an
// error when the Content-Type is unsupported or deserialization fails.
func DecodeBody(
	ns runtime.NegotiatedSerializer,
	req *http.Request,
	body []byte,
	into runtime.Object,
) (runtime.Object, error) {
	contentType := req.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}
	info, ok := runtime.SerializerInfoForMediaType(ns.SupportedMediaTypes(), contentType)
	if !ok {
		return nil, fmt.Errorf("unsupported Content-Type: %s", contentType)
	}
	obj, err := info.Serializer.Decode(body, into)
	if err != nil {
		return nil, fmt.Errorf("failed to decode request body: %w", err)
	}
	return obj, nil
}
