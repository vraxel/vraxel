package rest

import (
	"context"
	"net/http"

	"vraxel.io/vraxel/lib/runtime"
)

// HandlerFunc is the unified function signature for all request handlers.
type HandlerFunc func(ctx context.Context, params map[string]string, body []byte) (runtime.Object, error)

// RawHandlerFunc handles requests that need direct access to the HTTP
// request/response, bypassing the framework's body reading and JSON serialization.
// Used for binary uploads (large body) and binary downloads (streaming response).
type RawHandlerFunc func(w http.ResponseWriter, r *http.Request, params map[string]string)

// DefaultMaxRequestBodySize is the default request body size limit
// for the JSON Create/Update/Patch auto-handlers when a resource
// does not opt in to a larger limit via ResourceInfo.MaxRequestBodyBytes.
// Most Vraxel routes carry small payloads (a few KiB at most); resources
// that legitimately need larger bodies (e.g. dev/issues with base64
// image attachments) override this on a per-resource basis.
const DefaultMaxRequestBodySize = 1 << 20

// mergeQueryParams copies path params and adds query params that don't
// conflict with existing path params. This allows HandlerFunc to access
// query parameters (e.g. ?file=cert.pem) alongside path params.
func mergeQueryParams(pathParams map[string]string, req *http.Request) map[string]string {
	query := req.URL.Query()
	if len(query) == 0 {
		return pathParams
	}
	merged := make(map[string]string, len(pathParams)+len(query))
	for k, v := range pathParams {
		merged[k] = v
	}
	for k, vals := range query {
		if _, exists := merged[k]; !exists && len(vals) > 0 {
			merged[k] = vals[0]
		}
	}
	return merged
}
