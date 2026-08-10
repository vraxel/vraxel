package rest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	apierrors "vraxel.io/vraxel/lib/api/errors"
	"vraxel.io/vraxel/lib/logger"
	"vraxel.io/vraxel/lib/runtime"
)

// WriteObjectNegotiated serializes obj using a content-type negotiated from the
// request's Accept header, and writes it to the HTTP response
//
// This is the top-level entry point
//
//  1. Negotiate the output media type from the Accept header
//  2. If negotiation fails for an error response, fall back to raw JSON
//  3. Create the encoder
//  4. Call SerializeObject to encode and write
func WriteObjectNegotiated(
	ns runtime.NegotiatedSerializer,
	w http.ResponseWriter,
	req *http.Request,
	statusCode int,
	obj runtime.Object,
) {
	result, err := runtime.NegotiateOutputMediaType(req, ns)
	if err != nil {
		// if negotiation fails
		// and the status was already an error, fall back to raw JSON
		if isErrorStatusCode(statusCode) {
			WriteRawJSON(w, statusCode, obj)
			return
		}
		ErrorNegotiated(w, req, ns, fmt.Errorf("not acceptable: %w", err))
		return
	}

	SerializeObject(result.MediaType, result.Serializer, w, statusCode, obj)
}

// ErrorNegotiated writes an error response through the same negotiation path.
// If the error implements GetStatus() int (e.g. *apierrors.StatusError), its
// HTTP status code is used; otherwise a generic 500 is returned.
func ErrorNegotiated(
	w http.ResponseWriter,
	req *http.Request,
	ns runtime.NegotiatedSerializer,
	err error,
) {
	var errObj runtime.Object
	var code int

	if se, ok := err.(interface{ GetStatus() int }); ok {
		code = se.GetStatus()
		errObj = err.(runtime.Object)
	} else {
		code = http.StatusInternalServerError
		errObj = apierrors.NewInternalError(err)
	}

	// Log server errors (5xx) with full detail for debugging
	if code >= 500 {
		logger.Errorf("[%d] %s %s: %v", code, req.Method, req.URL.Path, err)
	}

	result, negErr := runtime.NegotiateOutputMediaType(req, ns)
	if negErr != nil {
		WriteRawJSON(w, code, errObj)
		return
	}
	SerializeObject(result.MediaType, result.Serializer, w, code, errObj)
}

// SerializeObject encodes the object and writes the HTTP response
//
//  1. Create a buffer for encoding
//  2. Encode the object via the encoder
//  3. Set Content-Type header
//  4. Write status code and body
func SerializeObject(
	mediaType string,
	encoder runtime.Encoder,
	w http.ResponseWriter,
	statusCode int,
	obj runtime.Object,
) {
	// Buffer the encoded output
	buf := &bytes.Buffer{}
	if err := encoder.Encode(obj, buf); err != nil {
		// If encoding fails, try to serialize an error status
		internalError(w, fmt.Errorf("encoding response: %w", err))
		return
	}

	// Set headers and write response
	w.Header().Set("Content-Type", mediaType)
	w.WriteHeader(statusCode)
	_, _ = w.Write(buf.Bytes())
}

// WriteRawJSON writes a non-API object in JSON
func WriteRawJSON(w http.ResponseWriter, statusCode int, object any) {
	output, err := json.MarshalIndent(object, "", "  ")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_, _ = w.Write(output)
}

func isErrorStatusCode(code int) bool {
	return code >= 400
}

func internalError(w http.ResponseWriter, err error) {
	http.Error(w, fmt.Sprintf("Internal Server Error: %v", err), http.StatusInternalServerError)
}
