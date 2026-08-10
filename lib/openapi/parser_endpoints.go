package openapi

import (
	"go/ast"
	"strings"
)

// buildEndpointInfo builds an EndpointInfo from a standalone function's
// +openapi: annotations. Returns ok=false when the function is not an endpoint
// or lacks the required path/method.
func buildEndpointInfo(fd *ast.FuncDecl) (EndpointInfo, bool) {
	annotations := ParseAnnotations(fd.Doc)
	if len(annotations) == 0 || !hasEndpointAnnotation(annotations) {
		return EndpointInfo{}, false
	}
	ep := EndpointInfo{}
	var responses []EndpointResponse
	for _, ann := range annotations {
		responses = applyEndpointAnnotation(&ep, responses, ann)
	}
	ep.Responses = responses
	if ep.Path == "" || ep.Method == "" {
		return EndpointInfo{}, false
	}
	return ep, true
}

// hasEndpointAnnotation reports whether the annotations include +openapi:endpoint.
func hasEndpointAnnotation(annotations []Annotation) bool {
	for _, ann := range annotations {
		if ann.Key == "endpoint" {
			return true
		}
	}
	return false
}

// applyEndpointAnnotation folds a single annotation into the endpoint, returning
// the (possibly grown) responses slice for response.* annotations.
func applyEndpointAnnotation(ep *EndpointInfo, responses []EndpointResponse, ann Annotation) []EndpointResponse {
	switch {
	case ann.Key == "path":
		ep.Path = ann.Value
	case ann.Key == "method":
		ep.Method = strings.ToUpper(ann.Value)
	case ann.Key == "summary":
		ep.Summary = ann.Value
	case ann.Key == "description":
		ep.Description = ann.Value
	case ann.Key == "tag":
		ep.Tag = ann.Value
	case ann.Key == "operationId":
		ep.OperationID = ann.Value
	case ann.Key == "requestBody.contentType":
		if ep.RequestBody == nil {
			ep.RequestBody = &EndpointBody{}
		}
		ep.RequestBody.ContentType = ann.Value
	case ann.Key == "requestBody.schema":
		if ep.RequestBody == nil {
			ep.RequestBody = &EndpointBody{}
		}
		ep.RequestBody.SchemaRef = ann.Value
	case strings.HasPrefix(ann.Key, "response."):
		responses = applyEndpointResponseAnnotation(responses, ann)
	}
	return responses
}

// applyEndpointResponseAnnotation parses a +openapi:response.CODE.FIELD
// annotation into the matching EndpointResponse, returning the slice.
func applyEndpointResponseAnnotation(responses []EndpointResponse, ann Annotation) []EndpointResponse {
	code, field, ok := strings.Cut(strings.TrimPrefix(ann.Key, "response."), ".")
	if !ok {
		return responses
	}
	responses, resp := findOrCreateResponse(responses, code)
	switch field {
	case "description":
		resp.Description = ann.Value
	case "contentType":
		resp.ContentType = ann.Value
	case "schema":
		resp.SchemaRef = ann.Value
	}
	return responses
}

// findOrCreateResponse returns a pointer to the response entry for code,
// appending a new one if absent, along with the (possibly grown) slice.
func findOrCreateResponse(responses []EndpointResponse, code string) ([]EndpointResponse, *EndpointResponse) {
	for i := range responses {
		if responses[i].StatusCode == code {
			return responses, &responses[i]
		}
	}
	responses = append(responses, EndpointResponse{StatusCode: code})
	return responses, &responses[len(responses)-1]
}
