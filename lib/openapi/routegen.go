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

// generateRoutePaths emits one operation per registered route. The route
// table is authoritative for which endpoints exist; a route whose payload
// type the AST parser never annotated is still emitted, with a generic
// object schema in place of a $ref. Dropping it instead would silently
// hide real endpoints (e.g. a whole module that ships no +openapi
// annotations) -- the opposite of describing what the server serves.
func (g *Generator) generateRoutePaths(doc *Document, groups []GroupInfo) {
	tl := newTypeLookup(groups)
	for _, r := range g.routes {
		ti, ok := tl.get(r.Group, r.TypeName)
		if !ok {
			// Unparsed payload: keep the Go type name for summaries and
			// tags; the empty annotation maps just fall through to the
			// generic fallbacks below.
			ti = TypeInfo{Name: r.TypeName}
		}
		g.emitRoute(doc, r, ti, ok)
	}
}

func (g *Generator) emitRoute(doc *Document, r Route, ti TypeInfo, typed bool) {
	item := getOrCreatePathItem(doc, r.Path)
	op := &Operation{
		Tags:       []string{typeTag(ti)},
		Parameters: pathParameters(r.Path),
	}

	// A parsed type resolves to its component $ref; an unparsed one falls
	// back to a generic object so the operation stays valid (no dangling
	// $ref) while still advertising the endpoint.
	ref := g.payloadRef(r.Group, ti.Name, typed)
	listRef := g.payloadRef(r.Group, ti.Name+"List", typed)
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
	case "createAny":
		// Ops.CreateAny: the request and response are handler-defined
		// (e.g. a batch {ids,roleId} grant answering a Result envelope),
		// so a typed $ref would document a contract that does not exist.
		op.Summary = g.routeSummary(ti, r, "create", "Create "+resourceLabel(r))
		op.OperationID = "create" + suffix
		op.RequestBody = jsonBody(&Schema{Type: "object"})
		op.Responses = jsonResponse("201", "Created", &Schema{Type: "object"})
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
		g.fillActionOp(op, r, ti)
		setPathOp(item, r.Method, op)
	case "verb":
		op.Summary = summaryOr(ti.CustomVerbSummary[r.Name], "Get "+r.Name+" of "+ti.Name)
		op.OperationID = "list" + suffix
		op.Parameters = append(op.Parameters, listQueryParameters()...)
		op.Responses = jsonResponse("200", "OK", g.payloadRef(r.Group, verbResponseType(ti, r.Name), typed))
		item.Get = op
	default:
		// Not every action-registration path normalises Kind to "action";
		// some leave the raw verb (scale / exec / invalidate) in Kind with
		// an empty Name. An unrecognised but served Kind is still an
		// action endpoint, so describe it as one rather than drop it.
		g.fillActionOp(op, r, ti)
		setPathOp(item, r.Method, op)
	}
}

// fillActionOp populates op as an action: summary, id and body derived
// from the action name (r.Name when the apiserver classified it, else the
// verb that leaked into r.Kind). Read-only actions (GET) carry no body.
func (g *Generator) fillActionOp(op *Operation, r Route, ti TypeInfo) {
	name := r.Name
	if name == "" {
		name = r.Kind
	}
	op.Summary = summaryOr(ti.ActionSummary[name], toCamelCase(name)+" "+ti.Name)
	op.OperationID = toCamelCase(name) + pascalSegments(routeQualifier(r.Path), name)
	if r.Method == "POST" || r.Method == "PUT" || r.Method == "PATCH" {
		op.RequestBody = jsonBody(&Schema{Type: "object"})
	}
	op.Responses = map[string]*Response{"200": {Description: "OK"}}
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

// payloadRef returns a $ref to the parsed component schema when the type
// was annotated, and a generic object otherwise -- a route table entry
// for an unparsed type still gets a valid (if untyped) body/response
// rather than a dangling $ref.
func (g *Generator) payloadRef(group, typeName string, typed bool) *Schema {
	if !typed {
		return &Schema{Type: "object"}
	}
	return g.schemaRef(group, typeName)
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

// setPathOp assigns an operation to the PathItem slot for its HTTP method.
func setPathOp(item *PathItem, method string, op *Operation) {
	switch method {
	case "GET":
		item.Get = op
	case "POST":
		item.Post = op
	case "PUT":
		item.Put = op
	case "PATCH":
		item.Patch = op
	case "DELETE":
		item.Delete = op
	}
}

// sealMissingListSchemas fills in every "<X>List" component a path $refs
// but that no schema pass produced, using the standard paginated envelope.
// Items point at the <X> component when it exists, else a generic object
// (e.g. a read verb whose response type was singularised from its name).
func sealMissingListSchemas(doc *Document) {
	if doc.Components == nil {
		return
	}
	refs := map[string]struct{}{}
	collectSchemaRefs(doc.Paths, refs)
	for _, s := range doc.Components.Schemas {
		collectSchemaRefs(s, refs)
	}
	for key := range refs {
		if _, ok := doc.Components.Schemas[key]; ok || !strings.HasSuffix(key, "List") {
			continue
		}
		items := &Schema{Type: "object"}
		if itemKey := strings.TrimSuffix(key, "List"); doc.Components.Schemas[itemKey] != nil {
			items = &Schema{Ref: "#/components/schemas/" + itemKey}
		}
		doc.Components.Schemas[key] = &Schema{
			Type: "object",
			Properties: map[string]*Schema{
				"items":      {Type: "array", Items: items},
				"totalCount": {Type: "integer"},
			},
		}
	}
}

// collectSchemaRefs walks any spec value and records the bare schema keys
// its $refs point at (the "#/components/schemas/" prefix stripped).
func collectSchemaRefs(x any, out map[string]struct{}) {
	switch v := x.(type) {
	case *Schema:
		if v == nil {
			return
		}
		if v.Ref != "" {
			out[strings.TrimPrefix(v.Ref, "#/components/schemas/")] = struct{}{}
		}
		collectSchemaRefs(v.Items, out)
		for _, p := range v.Properties {
			collectSchemaRefs(p, out)
		}
	case map[string]*PathItem:
		for _, pi := range v {
			collectSchemaRefs(pi, out)
		}
	case *PathItem:
		if v == nil {
			return
		}
		for _, op := range []*Operation{v.Get, v.Post, v.Put, v.Patch, v.Delete} {
			collectSchemaRefs(op, out)
		}
	case *Operation:
		if v == nil {
			return
		}
		if v.RequestBody != nil {
			for _, mt := range v.RequestBody.Content {
				collectSchemaRefs(mt.Schema, out)
			}
		}
		for _, resp := range v.Responses {
			for _, mt := range resp.Content {
				collectSchemaRefs(mt.Schema, out)
			}
		}
	}
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
