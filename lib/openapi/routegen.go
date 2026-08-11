package openapi

import (
	"fmt"
	"sort"
	"strings"
)

// Route is one endpoint the apiserver actually serves. The generator
// emits paths from these, never from the shape of the source code:
// resource names, nesting and id-parameter spellings are decided by
// registration, and re-deriving them from type names produced a spec
// that both invented endpoints (405 on GET/PUT/PATCH of every item) and
// missed real ones (/audit/v1/logs was documented as /audit/v1/auditlogs).
type Route struct {
	Method string
	Path   string
	// Kind: list | get | create | update | patch | delete |
	// deleteCollection | action | verb.
	Kind     string
	Group    string
	Resource string
	// TypeName is the Go name of the payload type.
	TypeName string
	// Name is the action / read-verb segment; empty for CRUD routes.
	Name string
}

// typeLookup resolves a route's payload type to its parsed TypeInfo.
type typeLookup map[string]map[string]TypeInfo // group → type name → info

func newTypeLookup(groups []GroupInfo) typeLookup {
	tl := make(typeLookup, len(groups))
	for _, g := range groups {
		byName := make(map[string]TypeInfo, len(g.Types))
		for _, t := range g.Types {
			byName[t.Name] = t
		}
		tl[g.GroupName] = byName
	}
	return tl
}

func (tl typeLookup) get(group, typeName string) (TypeInfo, bool) {
	t, ok := tl[group][typeName]
	return t, ok
}

// generateRoutePaths emits one operation per registered route.
func (g *Generator) generateRoutePaths(doc *Document, groups []GroupInfo) {
	tl := newTypeLookup(groups)
	for _, r := range g.routes {
		ti, ok := tl.get(r.Group, r.TypeName)
		if !ok {
			// A route whose payload type was not parsed cannot be
			// described; skipping beats emitting a dangling $ref.
			continue
		}
		g.emitRoute(doc, r, ti)
	}
}

func (g *Generator) emitRoute(doc *Document, r Route, ti TypeInfo) {
	item := getOrCreatePathItem(doc, r.Path)
	op := &Operation{
		Tags:       []string{typeTag(ti)},
		Parameters: pathParameters(r.Path),
	}

	ref := g.schemaRef(r.Group, ti.Name)
	listRef := g.schemaRef(r.Group, ti.Name+"List")
	suffix := operationIDSuffix(r)

	switch r.Kind {
	case "list":
		op.Summary = g.routeSummary(ti, r, "list", "List "+resourceLabel(r))
		op.OperationID = "list" + suffix
		op.Parameters = append(op.Parameters, listQueryParameters()...)
		op.Responses = jsonResponse("200", "OK", listRef)
		item.Get = op
	case "get":
		op.Summary = g.routeSummary(ti, r, "get", "Get a "+ti.Name)
		op.OperationID = "get" + suffix
		op.Responses = jsonResponse("200", "OK", ref)
		item.Get = op
	case "create":
		op.Summary = g.routeSummary(ti, r, "create", "Create a "+ti.Name)
		op.OperationID = "create" + suffix
		op.RequestBody = jsonBody(ref)
		op.Responses = jsonResponse("201", "Created", ref)
		item.Post = op
	case "update":
		op.Summary = g.routeSummary(ti, r, "update", "Update a "+ti.Name)
		op.OperationID = "update" + suffix
		op.RequestBody = jsonBody(ref)
		op.Responses = jsonResponse("200", "OK", ref)
		item.Put = op
	case "patch":
		op.Summary = g.routeSummary(ti, r, "patch", "Patch a "+ti.Name)
		op.OperationID = "patch" + suffix
		op.RequestBody = jsonBody(ref)
		op.Responses = jsonResponse("200", "OK", ref)
		item.Patch = op
	case "delete":
		op.Summary = g.routeSummary(ti, r, "delete", "Delete a "+ti.Name)
		op.OperationID = "delete" + suffix
		op.Responses = map[string]*Response{"204": {Description: "No Content"}}
		item.Delete = op
	case "deleteCollection":
		op.Summary = g.routeSummary(ti, r, "deleteCollection", "Batch delete "+resourceLabel(r))
		op.OperationID = "deleteCollection" + suffix
		op.RequestBody = jsonBody(&Schema{
			Type:       "object",
			Properties: map[string]*Schema{"ids": {Type: "array", Items: &Schema{Type: "string"}}},
		})
		op.Responses = map[string]*Response{"200": {Description: "OK"}}
		item.Delete = op
	case "action":
		op.Summary = summaryOr(ti.ActionSummary[r.Name], toCamelCase(r.Name)+" "+ti.Name)
		op.OperationID = toCamelCase(r.Name) + pascalSegments(routeQualifier(r.Path), r.Name)
		op.RequestBody = jsonBody(&Schema{Type: "object"})
		op.Responses = map[string]*Response{"200": {Description: "OK"}}
		item.Post = op
	case "verb":
		op.Summary = summaryOr(ti.CustomVerbSummary[r.Name], "Get "+r.Name+" of "+ti.Name)
		op.OperationID = "list" + suffix
		op.Parameters = append(op.Parameters, listQueryParameters()...)
		op.Responses = jsonResponse("200", "OK", g.schemaRef(r.Group, verbResponseType(ti, r.Name)))
		item.Get = op
	}
}

// routeSummary resolves an operation summary, preferring the annotation
// qualified by this route's own path ("workspaces.namespaces.rolebindings.list")
// over the unqualified one -- the same precedence the annotations were
// written against, now keyed on the served URL rather than a guess.
func (g *Generator) routeSummary(ti TypeInfo, r Route, op, fallback string) string {
	if s, ok := ti.OperationSummary[routeQualifier(r.Path)+"."+op]; ok {
		return s
	}
	if s, ok := ti.OperationSummary[op]; ok {
		return s
	}
	return fallback
}

// routeQualifier reduces "/api/iam/v1/workspaces/{workspaceId}/rolebindings"
// to "workspaces.rolebindings".
func routeQualifier(path string) string {
	var segments []string
	for _, p := range strings.Split(strings.Trim(path, "/"), "/") {
		if p == "" || strings.HasPrefix(p, "{") {
			continue
		}
		segments = append(segments, p)
	}
	if len(segments) > 3 { // drop "api", group, version
		segments = segments[3:]
	} else {
		segments = nil
	}
	return strings.Join(segments, ".")
}

// operationIDSuffix builds a unique PascalCase suffix from the resource
// segments of the path. Two routes cannot share a method and path, and
// the suffix keeps every segment, so operation IDs stay unique.
func operationIDSuffix(r Route) string {
	return pascalSegments(routeQualifier(r.Path), "")
}

// pascalSegments joins a dotted qualifier into PascalCase, optionally
// dropping one segment (an action name carried by the ID's prefix).
func pascalSegments(qualifier, skip string) string {
	var b strings.Builder
	for _, seg := range strings.Split(qualifier, ".") {
		if seg == "" || seg == skip {
			continue
		}
		b.WriteString(upperFirst(toCamelCase(seg)))
	}
	return b.String()
}

// resourceLabel is the plural resource segment used in default summaries.
func resourceLabel(r Route) string {
	if r.Resource != "" {
		return r.Resource
	}
	return r.TypeName
}

// verbResponseType resolves a read verb's response schema: the annotated
// type when present, otherwise <Verb singular>List.
func verbResponseType(ti TypeInfo, verbName string) string {
	if rt, ok := ti.CustomVerbResponse[verbName]; ok && rt != "" {
		return rt
	}
	return upperFirst(strings.TrimSuffix(verbName, "s")) + "List"
}

func typeTag(ti TypeInfo) string {
	if ti.Tag != "" {
		return ti.Tag
	}
	return ti.Name
}

func (g *Generator) schemaRef(group, typeName string) *Schema {
	return &Schema{Ref: fmt.Sprintf("#/components/schemas/%s", g.schemaKey(group, typeName))}
}

func jsonBody(s *Schema) *RequestBody {
	return &RequestBody{Required: true, Content: map[string]MediaType{"application/json": {Schema: s}}}
}

func jsonResponse(code, description string, s *Schema) map[string]*Response {
	return map[string]*Response{
		code: {Description: description, Content: map[string]MediaType{"application/json": {Schema: s}}},
	}
}

func listQueryParameters() []Parameter {
	return []Parameter{
		{Name: "page", In: "query", Schema: &Schema{Type: "integer"}},
		{Name: "pageSize", In: "query", Schema: &Schema{Type: "integer"}},
		{Name: "sortBy", In: "query", Schema: &Schema{Type: "string"}},
		{Name: "sortOrder", In: "query", Schema: &Schema{Type: "string", Enum: []string{"asc", "desc"}}},
	}
}

func pathParameters(path string) []Parameter {
	names := extractPathParams(path)
	params := make([]Parameter, 0, len(names))
	for _, n := range names {
		params = append(params, Parameter{
			Name: n, In: "path", Required: true, Schema: &Schema{Type: "string"},
		})
	}
	return params
}

func summaryOr(s, fallback string) string {
	if s != "" {
		return s
	}
	return fallback
}

func upperFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// sortRoutes gives the emitted spec a deterministic operation order.
func sortRoutes(routes []Route) {
	sort.SliceStable(routes, func(i, j int) bool {
		if routes[i].Path != routes[j].Path {
			return routes[i].Path < routes[j].Path
		}
		return routes[i].Method < routes[j].Method
	})
}
