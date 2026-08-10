package openapi

import (
	"path/filepath"
	"testing"
)

// TestParseGroupTypeCollection characterizes parseGroup's type-collection pass:
// direct +openapi: annotations on exported structs (path, summary.METHOD,
// action.NAME.summary, description, schema, List suffix) and type aliases
// (+openapi:schema on `type X = Y`). Safety net for decomposing parseGroup.
func TestParseGroupTypeCollection(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "demo", "doc.go"), `// +openapi:groupName=demo
// +openapi:groupVersion=v1
package demo
`)
	mustWrite(t, filepath.Join(root, "demo", "types.go"), `package demo

// +openapi:path=/things
// +openapi:summary.list=List things
// +openapi:action.archive.summary=Archive a thing
// +openapi:description=A thing resource
type Thing struct{}

// +openapi:schema
type ThingList struct{}

// +openapi:schema
type ThingRef = Thing
`)
	groups, err := NewParser(root).Parse()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	thing := findType(t, groups, "Thing")
	thingList := findType(t, groups, "ThingList")
	thingRef := findType(t, groups, "ThingRef")

	for _, c := range []struct{ name, got, want string }{
		{"Thing description", thing.Description, "A thing resource"},
		{"Thing direct list summary", thing.OperationSummary["list"], "List things"},
		{"Thing action summary", thing.ActionSummary["archive"], "Archive a thing"},
		{"ThingRef alias target", thingRef.AliasTarget, "Thing"},
	} {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.name, c.got, c.want)
		}
	}
	if !containsStr(thing.Paths, "/things") {
		t.Errorf("Thing.Paths = %v, want to contain /things", thing.Paths)
	}
	if thing.SchemaOnly {
		t.Errorf("Thing.SchemaOnly = true, want false (no +openapi:schema)")
	}
	if !thingList.IsListType {
		t.Errorf("ThingList.IsListType = false, want true")
	}
	if !thingRef.SchemaOnly {
		t.Errorf("ThingRef.SchemaOnly = false, want true")
	}
}

// TestParseEndpointAnnotations characterizes parseEndpointAnnotations: a
// standalone func with +openapi:endpoint yields an EndpointInfo (method
// upper-cased, request body and per-status responses parsed); funcs without
// +openapi:endpoint or missing path/method are dropped.
func TestParseEndpointAnnotations(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "demo", "doc.go"), `// +openapi:groupName=demo
// +openapi:groupVersion=v1
package demo
`)
	mustWrite(t, filepath.Join(root, "demo", "endpoints.go"), `package demo

// +openapi:schema
type Keep struct{}

// +openapi:endpoint
// +openapi:path=/auth/login
// +openapi:method=post
// +openapi:summary=Log in
// +openapi:description=Authenticate
// +openapi:tag=Auth
// +openapi:operationId=login
// +openapi:requestBody.contentType=application/json
// +openapi:requestBody.schema=LoginRequest
// +openapi:response.200.description=OK
// +openapi:response.200.contentType=application/json
// +openapi:response.200.schema=LoginResponse
func loginEndpoint() {}

// +openapi:path=/ignored
// +openapi:method=get
func notAnEndpoint() {}

// +openapi:endpoint
// +openapi:path=/incomplete
func incompleteEndpoint() {}
`)
	groups, err := NewParser(root).Parse()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	eps := findGroup(t, groups, "demo").Endpoints
	if len(eps) != 1 {
		t.Fatalf("Endpoints = %d, want 1 (login only; no-endpoint and missing-method dropped)", len(eps))
	}
	ep := eps[0]
	if ep.RequestBody == nil {
		t.Fatalf("RequestBody = nil, want non-nil")
	}
	if len(ep.Responses) != 1 {
		t.Fatalf("Responses = %d, want 1", len(ep.Responses))
	}
	r := ep.Responses[0]
	for _, c := range []struct{ name, got, want string }{
		{"path", ep.Path, "/auth/login"},
		{"method (upper-cased)", ep.Method, "POST"},
		{"summary", ep.Summary, "Log in"},
		{"description", ep.Description, "Authenticate"},
		{"tag", ep.Tag, "Auth"},
		{"operationId", ep.OperationID, "login"},
		{"requestBody contentType", ep.RequestBody.ContentType, "application/json"},
		{"requestBody schema", ep.RequestBody.SchemaRef, "LoginRequest"},
		{"response status", r.StatusCode, "200"},
		{"response description", r.Description, "OK"},
		{"response contentType", r.ContentType, "application/json"},
		{"response schema", r.SchemaRef, "LoginResponse"},
	} {
		if c.got != c.want {
			t.Errorf("endpoint %s = %q, want %q", c.name, c.got, c.want)
		}
	}
}

func findGroup(t *testing.T, groups []GroupInfo, name string) GroupInfo {
	t.Helper()
	for _, g := range groups {
		if g.GroupName == name {
			return g
		}
	}
	t.Fatalf("group %q not found", name)
	return GroupInfo{}
}
