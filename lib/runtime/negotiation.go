package runtime

import (
	"fmt"
	"net/http"
	"strings"
)

// NegotiateResult holds the outcome of content-type negotiation
type NegotiateResult struct {
	// MediaType is the negotiated media type string
	MediaType string
	// Serializer is the selected serializer for the negotiated media type
	Serializer Serializer
}

// mediaTypeClause represents a single clause from the Accept header
type mediaTypeClause struct {
	Type    string // e.g. "application"
	SubType string // e.g. "json"
}

// NotAcceptableError is returned when none of the registered serializers
// match the client's Accept header
type NotAcceptableError struct {
	Accepted []string
}

func (e *NotAcceptableError) Error() string {
	return fmt.Sprintf("only the following media types are accepted: %s", strings.Join(e.Accepted, ", "))
}

// NegotiateOutputMediaType determines the best serializer for the response based
// on the request's Accept header
//
// Flow:
//  1. Parse Accept header into media type clauses
//  2. For each clause, find a matching registered serializer
//  3. Detect pretty-print preference and swap to PrettySerializer if needed
//  4. Return the result or an error if no acceptable type is found
func NegotiateOutputMediaType(req *http.Request, ns NegotiatedSerializer) (NegotiateResult, error) {
	supported := ns.SupportedMediaTypes()
	if len(supported) == 0 {
		return NegotiateResult{}, fmt.Errorf("no supported media types registered")
	}

	accept := req.Header.Get("Accept")

	// If no Accept header (or Accept: */*), default to the first registered
	// serializer (JSON)
	if accept == "" || accept == "*/*" {
		info := supported[0]
		s := chooseSerializer(info, isPrettyPrint(req))
		return NegotiateResult{
			MediaType:  info.MediaType,
			Serializer: s,
		}, nil
	}

	// Parse Accept header clauses. Real K8s uses goautoneg.ParseAccept which
	// handles quality values. We do a simplified parse.
	clauses := parseAccept(accept)
	for _, clause := range clauses {
		for _, info := range supported {
			if mediaTypeMatches(clause, info) {
				s := chooseSerializer(info, isPrettyPrint(req))
				return NegotiateResult{
					MediaType:  info.MediaType,
					Serializer: s,
				}, nil
			}
		}
	}

	return NegotiateResult{}, &NotAcceptableError{
		Accepted: supportedMediaTypes(supported),
	}
}

// parseAccept splits an Accept header value into clauses
func parseAccept(accept string) []mediaTypeClause {
	var clauses []mediaTypeClause
	for _, part := range strings.Split(accept, ",") {
		part = strings.TrimSpace(part)
		// Strip parameters (e.g. ";q=0.9", ";charset=utf-8")
		if idx := strings.IndexByte(part, ';'); idx >= 0 {
			part = strings.TrimSpace(part[:idx])
		}
		slash := strings.IndexByte(part, '/')
		if slash < 0 {
			continue
		}
		clauses = append(clauses, mediaTypeClause{
			Type:    strings.TrimSpace(part[:slash]),
			SubType: strings.TrimSpace(part[slash+1:]),
		})
	}
	return clauses
}

// chooseSerializer returns the PrettySerializer if pretty printing is requested
// and a PrettySerializer is available; otherwise returns the standard Serializer
//
//	if mediaType.Pretty || isPrettyPrint(req) { use PrettySerializer }
func chooseSerializer(info SerializerInfo, pretty bool) Serializer {
	if pretty && info.PrettySerializer != nil {
		return info.PrettySerializer
	}
	return info.Serializer
}

// isPrettyPrint detects whether the client wants pretty-printed output
// Real checks:
//   - ?pretty=true query parameter
//   - User-Agent matching curl, wget, or browser patterns
//
// We simplify to just checking the query parameter.
func isPrettyPrint(req *http.Request) bool {
	return req.URL.Query().Get("pretty") == "true"
}

// mediaTypeMatches checks whether an Accept clause matches a SerializerInfo
// Supports wildcard matching ("*/*" and "application/*")
func mediaTypeMatches(clause mediaTypeClause, info SerializerInfo) bool {
	if clause.Type == "*" && clause.SubType == "*" {
		return true
	}
	if strings.EqualFold(clause.Type, info.MediaTypeType) && clause.SubType == "*" {
		return true
	}
	return strings.EqualFold(clause.Type, info.MediaTypeType) &&
		strings.EqualFold(clause.SubType, info.MediaTypeSubType)
}

func supportedMediaTypes(infos []SerializerInfo) []string {
	var types []string
	for _, info := range infos {
		types = append(types, info.MediaType)
	}
	return types
}
