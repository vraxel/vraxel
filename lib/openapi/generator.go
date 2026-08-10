package openapi

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Generator builds an OpenAPI 3.0 specification from parsed API groups.
//
// conflictTypes / pkgToGroup are derived from the parsed groups at the
// start of Generate so schema emission and $ref resolution can
// disambiguate same-name types declared in different modules
// (e.g. app.HostSpec vs compute.HostSpec). Conflict-affected schemas
// are emitted as `<group>_<TypeName>`; non-conflicting types keep the
// bare name so the spec stays stable for callers that already pinned
// against it. See schemaKey / fieldRefName.
type Generator struct {
	info          Info
	conflictTypes map[string]bool
	pkgToGroup    map[string]string
}

// NewGenerator creates a new OpenAPI generator with the given API info.
func NewGenerator(title, description, version string) *Generator {
	return &Generator{
		info: Info{
			Title:       title,
			Description: description,
			Version:     version,
		},
	}
}

// Generate builds a complete OpenAPI document from the parsed groups.
func (g *Generator) Generate(groups []GroupInfo) *Document {
	doc := &Document{
		OpenAPI: "3.0.3",
		Info:    g.info,
		Paths:   make(map[string]*PathItem),
		Components: &Components{
			Schemas: make(map[string]*Schema),
		},
	}

	// Pass 0 — index types across groups so we can detect cross-module
	// name collisions and resolve cross-package $ref by package name.
	g.conflictTypes = collectConflictTypes(groups)
	g.pkgToGroup = collectPkgToGroup(groups)

	// Collect tags per module for x-tagGroups
	moduleTags := make(map[string][]Tag)

	for _, group := range groups {
		tags := g.processGroup(doc, group)
		moduleTags[group.ModuleName] = append(moduleTags[group.ModuleName], tags...)
	}

	// Build document-level Tags and XTagGroups
	for _, moduleName := range sortedKeys(moduleTags) {
		tags := moduleTags[moduleName]
		var tagNames []string
		for _, t := range tags {
			doc.Tags = append(doc.Tags, t)
			tagNames = append(tagNames, t.Name)
		}
		doc.XTagGroups = append(doc.XTagGroups, TagGroup{
			Name: moduleName,
			Tags: tagNames,
		})
	}

	return doc
}

// collectConflictTypes walks every group's Types once and returns the set of
// type names that appear in more than one group. Helper packages (paasdeploy,
// shared/...) declare no GroupName, so their types collapse under the same
// "" bucket — they do not collide with module-scoped types.
func collectConflictTypes(groups []GroupInfo) map[string]bool {
	occurrences := make(map[string]map[string]bool)
	for _, group := range groups {
		for _, t := range group.Types {
			if occurrences[t.Name] == nil {
				occurrences[t.Name] = map[string]bool{}
			}
			occurrences[t.Name][group.GroupName] = true
		}
	}
	conflict := map[string]bool{}
	for name, gset := range occurrences {
		if len(gset) > 1 {
			conflict[name] = true
		}
	}
	return conflict
}

// collectPkgToGroup maps each Go package name to its group name so the
// generator can resolve `pkg.X` cross-package references (only the last
// `.` segment survives in field.GoType, but we still need to know which
// group X belongs to when X is conflict-affected). Helper packages
// without a GroupName map to "" and inherit the bare-name path.
func collectPkgToGroup(groups []GroupInfo) map[string]string {
	out := map[string]string{}
	for _, group := range groups {
		for _, t := range group.Types {
			if t.Package == "" {
				continue
			}
			out[t.Package] = group.GroupName
		}
	}
	return out
}

// schemaKey returns the schema name to register / reference for the
// (group, typeName) pair. Non-conflict types keep the bare typeName;
// conflict types are prefixed with the group name so two modules can
// each emit their own version of the same struct (e.g. app_HostSpec /
// compute_HostSpec). Types declared in a helper package without a
// group name (paasdeploy, shared/*) always keep the bare typeName —
// they cannot conflict with module schemas under this rule because
// occurrences[typeName] would only show one group.
func (g *Generator) schemaKey(groupName, typeName string) string {
	if g.conflictTypes[typeName] && groupName != "" {
		return groupName + "_" + typeName
	}
	return typeName
}

// fieldRefName resolves a Go field type string (e.g. "*HostSpec",
// "[]K8sPort", "paasdeploy.PaasCloudSource") to the OpenAPI schema
// name it should reference. currentGroup is the group name of the
// type that owns the field — used as the implicit package for
// unqualified references. Conflict-affected names are routed through
// schemaKey to pick up the group prefix; everything else passes
// through untouched so non-conflicting cross-package references (the
// common case) stay stable.
func (g *Generator) fieldRefName(currentGroup, goType string) string {
	// Strip *, [], map[K]V wrappers down to the bare type expression.
	bare := goType
	for {
		switch {
		case strings.HasPrefix(bare, "*"):
			bare = strings.TrimPrefix(bare, "*")
		case strings.HasPrefix(bare, "[]"):
			bare = strings.TrimPrefix(bare, "[]")
		case strings.HasPrefix(bare, "map["):
			// map[K]V — drop key, keep value
			end := strings.Index(bare, "]")
			if end < 0 {
				return goType
			}
			bare = bare[end+1:]
		default:
			goto done
		}
	}
done:
	// Resolve package qualifier `pkg.Type` to the owning group, if any.
	groupForRef := currentGroup
	cleanType := bare
	if idx := strings.LastIndex(bare, "."); idx >= 0 {
		pkgPart := bare[:idx]
		cleanType = bare[idx+1:]
		if grp, ok := g.pkgToGroup[pkgPart]; ok {
			groupForRef = grp
		} else {
			// Unknown package — fall back to bare name; non-conflict
			// types resolve correctly, conflict types will dangle
			// (caller's problem to wire up the package mapping).
			groupForRef = ""
		}
	}
	return g.schemaKey(groupForRef, cleanType)
}

func (g *Generator) processGroup(doc *Document, group GroupInfo) []Tag {
	// Process standalone endpoints first
	endpointTags := g.processEndpoints(doc, group.Endpoints)

	// Build base path
	var basePath string
	if group.GroupName == "" {
		basePath = "/api/" + group.GroupVersion
	} else {
		basePath = "/api/" + group.GroupName + "/" + group.GroupVersion
	}

	// Collect all type names to identify which are Spec types (not standalone resources)
	specTypes := map[string]bool{}
	for _, t := range group.Types {
		if strings.HasSuffix(t.Name, "Spec") || strings.HasSuffix(t.Name, "Meta") {
			specTypes[t.Name] = true
		}
	}

	// Phase 1: Register all schemas in components
	for _, t := range group.Types {
		schema := g.typeToSchema(t, group.GroupName)
		doc.Components.Schemas[g.schemaKey(group.GroupName, t.Name)] = schema
	}

	// Phase 2: Generate paths from +openapi:path annotations (or default)
	tags := g.generateGroupPaths(doc, group, basePath, specTypes)

	// Merge endpoint tags (deduplicated)
	return mergeEndpointTags(tags, endpointTags)
}

// generateGroupPaths runs Phase 2 for a group: for every non-spec, non-list
// resource type it emits paths (default or annotated) and collects the
// standalone tags for types that did not override their tag.
func (g *Generator) generateGroupPaths(doc *Document, group GroupInfo, basePath string, specTypes map[string]bool) []Tag {
	var tags []Tag
	for _, t := range group.Types {
		if t.IsListType || t.SchemaOnly || specTypes[t.Name] {
			continue
		}

		paths := t.Paths
		if len(paths) == 0 {
			// No annotation: derive default path from type name
			paths = []string{"/" + strings.ToLower(t.Name) + "s"}
		}

		tag := t.Name
		if t.Tag != "" {
			tag = t.Tag
		}
		for _, p := range paths {
			g.generatePathsForResource(doc, basePath, p, t, group.GroupName, group.GroupVersion, tag)
		}
		if t.Tag == "" {
			// Only add as a standalone tag if not overridden (overridden tags merge into existing parent tags)
			tags = append(tags, Tag{Name: t.Name, Description: t.Description})
		}
	}
	return tags
}

// mergeEndpointTags appends endpoint tags onto the resource tags, skipping
// any whose name already appears.
func mergeEndpointTags(tags, endpointTags []Tag) []Tag {
	existingTags := make(map[string]bool)
	for _, t := range tags {
		existingTags[t.Name] = true
	}
	for _, t := range endpointTags {
		if !existingTags[t.Name] {
			tags = append(tags, t)
			existingTags[t.Name] = true
		}
	}

	return tags
}

// processEndpoints generates OpenAPI paths for standalone endpoint annotations.
// These endpoints use their path as-is (no basePath prefix).
func (g *Generator) processEndpoints(doc *Document, endpoints []EndpointInfo) []Tag {
	tagSet := make(map[string]bool)
	var tags []Tag

	for _, ep := range endpoints {
		pathItem := getOrCreatePathItem(doc, ep.Path)

		op := &Operation{
			Summary:     ep.Summary,
			Description: ep.Description,
			OperationID: ep.OperationID,
		}
		if ep.Tag != "" {
			op.Tags = []string{ep.Tag}
			if !tagSet[ep.Tag] {
				tagSet[ep.Tag] = true
				tags = append(tags, Tag{Name: ep.Tag})
			}
		}

		endpointRequestBody(op, ep)
		endpointResponses(op, ep)
		endpointPathParams(op, ep)
		assignEndpointOperation(pathItem, ep.Method, op)
	}

	return tags
}

// endpointRequestBody sets op.RequestBody from the endpoint's RequestBody annotation.
func endpointRequestBody(op *Operation, ep EndpointInfo) {
	if ep.RequestBody != nil {
		contentType := ep.RequestBody.ContentType
		if contentType == "" {
			contentType = "application/json"
		}
		var schema *Schema
		if ep.RequestBody.SchemaRef != "" {
			schema = &Schema{Ref: "#/components/schemas/" + ep.RequestBody.SchemaRef}
		} else {
			schema = &Schema{Type: "object"}
		}
		op.RequestBody = &RequestBody{
			Required: true,
			Content: map[string]MediaType{
				contentType: {Schema: schema},
			},
		}
	}
}

// endpointResponses sets op.Responses from the endpoint's Responses annotations,
// falling back to a bare 200 OK when none are declared.
func endpointResponses(op *Operation, ep EndpointInfo) {
	if len(ep.Responses) > 0 {
		op.Responses = make(map[string]*Response)
		for _, r := range ep.Responses {
			op.Responses[r.StatusCode] = endpointResponse(r)
		}
	} else {
		op.Responses = map[string]*Response{
			"200": {Description: "OK"},
		}
	}
}

// endpointResponse builds a single Response from a response annotation, attaching
// content only when a content type or schema ref is declared.
func endpointResponse(r EndpointResponse) *Response {
	resp := &Response{Description: r.Description}
	if r.ContentType != "" || r.SchemaRef != "" {
		ct := r.ContentType
		if ct == "" {
			ct = "application/json"
		}
		var schema *Schema
		if r.SchemaRef != "" {
			schema = &Schema{Ref: "#/components/schemas/" + r.SchemaRef}
		} else {
			schema = &Schema{Type: "object"}
		}
		resp.Content = map[string]MediaType{
			ct: {Schema: schema},
		}
	}
	return resp
}

// endpointPathParams appends path parameters extracted from the endpoint path.
func endpointPathParams(op *Operation, ep EndpointInfo) {
	params := extractPathParams(ep.Path)
	for _, p := range params {
		op.Parameters = append(op.Parameters, Parameter{
			Name: p, In: "path", Required: true, Schema: &Schema{Type: "string"},
		})
	}
}

// assignEndpointOperation attaches op to the path item slot for the HTTP method.
func assignEndpointOperation(pathItem *PathItem, method string, op *Operation) {
	switch method {
	case "GET":
		pathItem.Get = op
	case "POST":
		pathItem.Post = op
	case "PUT":
		pathItem.Put = op
	case "PATCH":
		pathItem.Patch = op
	case "DELETE":
		pathItem.Delete = op
	}
}

func (g *Generator) typeToSchema(t TypeInfo, currentGroup string) *Schema {
	// Alias types (e.g. type MySQLCloudSource = paasdeploy.CloudSource)
	// emit as a thin $ref to the target schema so the underlying
	// struct's properties are reused without duplication. The alias's
	// AliasTarget is the unqualified target name, so route it through
	// fieldRefName with the alias's own group as the "current" group —
	// helper-package targets without a group then keep the bare name.
	if t.AliasTarget != "" {
		schema := &Schema{Ref: "#/components/schemas/" + g.fieldRefName(currentGroup, t.AliasTarget)}
		if t.Description != "" {
			schema.Description = t.Description
		}
		return schema
	}
	schema := &Schema{
		Type:       "object",
		Properties: make(map[string]*Schema),
	}

	var required []string
	for _, field := range t.Fields {
		fieldSchema := g.goTypeToSchema(field.GoType, currentGroup)
		if field.Annotations.Description != "" {
			fieldSchema.Description = field.Annotations.Description
		}
		if field.Annotations.Format != "" {
			fieldSchema.Format = field.Annotations.Format
		}
		if len(field.Annotations.Enum) > 0 {
			fieldSchema.Enum = field.Annotations.Enum
		}
		if field.Annotations.Required {
			required = append(required, field.JSONName)
		}

		schema.Properties[field.JSONName] = fieldSchema
	}

	if len(required) > 0 {
		schema.Required = required
	}

	return schema
}

func (g *Generator) goTypeToSchema(goType, currentGroup string) *Schema {
	switch goType {
	case "string":
		return &Schema{Type: "string"}
	case "int", "int32", "int64":
		return &Schema{Type: "integer", Format: goType}
	case "float32":
		return &Schema{Type: "number", Format: "float"}
	case "float64":
		return &Schema{Type: "number", Format: "double"}
	case "bool":
		return &Schema{Type: "boolean"}
	default:
		if strings.HasPrefix(goType, "[]") {
			elemType := strings.TrimPrefix(goType, "[]")
			return &Schema{
				Type:  "array",
				Items: g.goTypeToSchema(elemType, currentGroup),
			}
		}
		if strings.HasPrefix(goType, "map[") {
			return &Schema{Type: "object"}
		}
		if strings.HasPrefix(goType, "*") {
			return g.goTypeToSchema(strings.TrimPrefix(goType, "*"), currentGroup)
		}
		// Reference to another type — route through fieldRefName so
		// conflict-affected types pick up the correct group prefix.
		return &Schema{Ref: "#/components/schemas/" + g.fieldRefName(currentGroup, goType)}
	}
}

// pathGenContext bundles the per-resource values shared across the path-emission
// helpers of generatePathsForResource so each helper takes a single receiver
// instead of a long parameter list. It carries no behavior beyond hasOp /
// qualifiedSummary, which mirror the closures the original function used.
type pathGenContext struct {
	g            *Generator
	doc          *Document
	typeInfo     TypeInfo
	resourcePath string
	groupName    string
	tag          string
	typeName     string
	itemPath     string
	opSuffix     string
	pathParams   []string
	itemParams   []Parameter
	ref          *Schema
	listRef      *Schema
}

// hasOp reports whether the operation op should be generated for this path.
// If PathOperations is defined for this path, only generate those operations.
// If not defined (backward compat), generate all operations.
func (c *pathGenContext) hasOp(op string) bool {
	if c.typeInfo.PathOperations == nil {
		return true
	}
	ops, ok := c.typeInfo.PathOperations[c.resourcePath]
	if !ok {
		return true
	}
	return ops[op]
}

// summary resolves an operation summary: use annotation if present, otherwise default.
func (c *pathGenContext) summary(op, defaultSummary string) string {
	if s, ok := c.typeInfo.OperationSummary[op]; ok {
		return s
	}
	return defaultSummary
}

// qualifiedSummary resolves a summary using a qualified operation key for nested
// resources (e.g., "workspaces.namespaces.list"), falling back to the plain key.
func (c *pathGenContext) qualifiedSummary(op, defaultSummary string) string {
	// Try qualified key first (e.g., "workspaces.users.list"), then plain key
	qualifiedKey := c.resourcePath + "." + op
	// Normalize: strip leading /, replace {param}/ segments
	parts := strings.Split(strings.Trim(qualifiedKey, "/"), "/")
	var segments []string
	for _, p := range parts {
		if !strings.HasPrefix(p, "{") {
			segments = append(segments, p)
		}
	}
	qualified := strings.Join(segments, ".") // e.g. "workspaces.namespaces.users.list"
	if s, ok := c.typeInfo.OperationSummary[qualified]; ok {
		return s
	}
	return c.summary(op, defaultSummary)
}

// generatePathsForResource generates collection and item paths for a resource
// at the given resourcePath. It extracts path parameters from the path template
// and uses a single tag for all operations.
func (g *Generator) generatePathsForResource(doc *Document, basePath, resourcePath string, typeInfo TypeInfo, groupName, version, tag string) {
	typeName := typeInfo.Name
	ref := &Schema{Ref: fmt.Sprintf("#/components/schemas/%s", g.schemaKey(groupName, typeName))}
	listRef := &Schema{Ref: fmt.Sprintf("#/components/schemas/%s", g.schemaKey(groupName, typeName+"List"))}

	collectionPath := basePath + resourcePath
	idParam := deriveIDParam(resourcePath)
	itemPath := collectionPath + "/{" + idParam + "}"

	// Extract all {param} from the resource path as path parameters
	pathParams := extractPathParams(resourcePath)

	// Build a unique operation ID suffix from path segments to avoid collisions
	opSuffix := operationSuffix(resourcePath, typeName)

	// Build item-level path parameters (used by item operations, actions, and custom verbs)
	itemParams := make([]Parameter, 0, len(pathParams)+1)
	for _, pp := range pathParams {
		itemParams = append(itemParams, Parameter{
			Name: pp, In: "path", Required: true, Schema: &Schema{Type: "string"},
		})
	}
	itemParams = append(itemParams, Parameter{
		Name: idParam, In: "path", Required: true, Schema: &Schema{Type: "string"},
	})

	c := &pathGenContext{
		g:            g,
		doc:          doc,
		typeInfo:     typeInfo,
		resourcePath: resourcePath,
		groupName:    groupName,
		tag:          tag,
		typeName:     typeName,
		itemPath:     itemPath,
		opSuffix:     opSuffix,
		pathParams:   pathParams,
		itemParams:   itemParams,
		ref:          ref,
		listRef:      listRef,
	}

	c.generateCollectionOps(collectionPath)
	c.generateItemOps()
	c.generateActionOps()
	c.generateCustomVerbOps()
}

// generateCollectionOps emits the list / create / deleteCollection operations on
// the collection path, creating that path only when at least one of them exists.
func (c *pathGenContext) generateCollectionOps(collectionPath string) {
	// Collection operations — only create path if at least one collection operation exists
	needCollectionPath := c.hasOp("list") || c.hasOp("create") || c.hasOp("deleteCollection")
	var pathItem *PathItem
	if needCollectionPath {
		pathItem = getOrCreatePathItem(c.doc, collectionPath)
	}

	if c.hasOp("list") {
		listParams := make([]Parameter, 0, len(c.pathParams)+4)
		for _, pp := range c.pathParams {
			listParams = append(listParams, Parameter{
				Name: pp, In: "path", Required: true, Schema: &Schema{Type: "string"},
			})
		}
		listParams = append(listParams,
			Parameter{Name: "page", In: "query", Schema: &Schema{Type: "integer"}},
			Parameter{Name: "pageSize", In: "query", Schema: &Schema{Type: "integer"}},
			Parameter{Name: "sortBy", In: "query", Schema: &Schema{Type: "string"}},
			Parameter{Name: "sortOrder", In: "query", Schema: &Schema{Type: "string", Enum: []string{"asc", "desc"}}},
		)
		pathItem.Get = &Operation{
			Summary:     c.qualifiedSummary("list", fmt.Sprintf("List %s", lastSegment(c.resourcePath))),
			OperationID: fmt.Sprintf("list%s", c.opSuffix),
			Tags:        []string{c.tag},
			Parameters:  listParams,
			Responses: map[string]*Response{
				"200": {
					Description: "OK",
					Content: map[string]MediaType{
						"application/json": {Schema: c.listRef},
					},
				},
			},
		}
	}

	if c.hasOp("create") {
		createParams := make([]Parameter, 0, len(c.pathParams))
		for _, pp := range c.pathParams {
			createParams = append(createParams, Parameter{
				Name: pp, In: "path", Required: true, Schema: &Schema{Type: "string"},
			})
		}
		pathItem.Post = &Operation{
			Summary:     c.qualifiedSummary("create", fmt.Sprintf("Create a %s", c.typeName)),
			OperationID: fmt.Sprintf("create%s", c.opSuffix),
			Tags:        []string{c.tag},
			Parameters:  createParams,
			RequestBody: &RequestBody{
				Required: true,
				Content: map[string]MediaType{
					"application/json": {Schema: c.ref},
				},
			},
			Responses: map[string]*Response{
				"201": {
					Description: "Created",
					Content: map[string]MediaType{
						"application/json": {Schema: c.ref},
					},
				},
			},
		}
	}

	// Delete collection
	if c.hasOp("deleteCollection") {
		dcParams := make([]Parameter, 0, len(c.pathParams))
		for _, pp := range c.pathParams {
			dcParams = append(dcParams, Parameter{
				Name: pp, In: "path", Required: true, Schema: &Schema{Type: "string"},
			})
		}
		pathItem.Delete = &Operation{
			Summary:     c.qualifiedSummary("deleteCollection", fmt.Sprintf("Batch delete %s", lastSegment(c.resourcePath))),
			OperationID: fmt.Sprintf("deleteCollection%s", c.opSuffix),
			Tags:        []string{c.tag},
			Parameters:  dcParams,
			RequestBody: &RequestBody{
				Required: true,
				Content: map[string]MediaType{
					"application/json": {Schema: &Schema{
						Type: "object",
						Properties: map[string]*Schema{
							"ids": {Type: "array", Items: &Schema{Type: "string"}},
						},
					}},
				},
			},
			Responses: map[string]*Response{
				"200": {Description: "OK"},
			},
		}
	}
}

// generateItemOps emits the get / update / patch / delete operations on the item
// path, creating that path only when at least one of them exists.
func (c *pathGenContext) generateItemOps() {
	// Item operations — only create the item path if at least one item operation exists
	needItemPath := c.hasOp("get") || c.hasOp("update") || c.hasOp("patch") || c.hasOp("delete")
	if needItemPath {
		itemPathItem := getOrCreatePathItem(c.doc, c.itemPath)

		if c.hasOp("get") {
			itemPathItem.Get = &Operation{
				Summary:     c.qualifiedSummary("get", fmt.Sprintf("Get a %s", c.typeName)),
				OperationID: fmt.Sprintf("get%s", c.opSuffix),
				Tags:        []string{c.tag},
				Parameters:  c.itemParams,
				Responses: map[string]*Response{
					"200": {
						Description: "OK",
						Content: map[string]MediaType{
							"application/json": {Schema: c.ref},
						},
					},
				},
			}
		}
		if c.hasOp("update") {
			itemPathItem.Put = &Operation{
				Summary:     c.qualifiedSummary("update", fmt.Sprintf("Update a %s", c.typeName)),
				OperationID: fmt.Sprintf("update%s", c.opSuffix),
				Tags:        []string{c.tag},
				Parameters:  c.itemParams,
				RequestBody: &RequestBody{
					Required: true,
					Content: map[string]MediaType{
						"application/json": {Schema: c.ref},
					},
				},
				Responses: map[string]*Response{
					"200": {
						Description: "OK",
						Content: map[string]MediaType{
							"application/json": {Schema: c.ref},
						},
					},
				},
			}
		}
		if c.hasOp("patch") {
			itemPathItem.Patch = &Operation{
				Summary:     c.qualifiedSummary("patch", fmt.Sprintf("Patch a %s", c.typeName)),
				OperationID: fmt.Sprintf("patch%s", c.opSuffix),
				Tags:        []string{c.tag},
				Parameters:  c.itemParams,
				RequestBody: &RequestBody{
					Required: true,
					Content: map[string]MediaType{
						"application/json": {Schema: c.ref},
					},
				},
				Responses: map[string]*Response{
					"200": {
						Description: "OK",
						Content: map[string]MediaType{
							"application/json": {Schema: c.ref},
						},
					},
				},
			}
		}
		if c.hasOp("delete") {
			itemPathItem.Delete = &Operation{
				Summary:     c.qualifiedSummary("delete", fmt.Sprintf("Delete a %s", c.typeName)),
				OperationID: fmt.Sprintf("delete%s", c.opSuffix),
				Tags:        []string{c.tag},
				Parameters:  c.itemParams,
				Responses: map[string]*Response{
					"204": {Description: "No Content"},
				},
			}
		}
	}
}

// generateActionOps emits a POST operation per declared action under the item path.
func (c *pathGenContext) generateActionOps() {
	// Action operations
	for actionName, actionSummary := range c.typeInfo.ActionSummary {
		actionPath := c.itemPath + "/" + actionName
		actionPathItem := getOrCreatePathItem(c.doc, actionPath)
		actionPathItem.Post = &Operation{
			Summary:     actionSummary,
			OperationID: fmt.Sprintf("%s%s", toCamelCase(actionName), c.opSuffix),
			Tags:        []string{c.tag},
			Parameters:  c.itemParams,
			RequestBody: &RequestBody{
				Required: true,
				Content: map[string]MediaType{
					"application/json": {Schema: &Schema{Type: "object"}},
				},
			},
			Responses: map[string]*Response{
				"200": {Description: "OK"},
			},
		}
	}
}

// generateCustomVerbOps emits GET {itemPath}:{verbName} operations for custom verbs.
func (c *pathGenContext) generateCustomVerbOps() {
	// Custom verb operations (GET {itemPath}:{verbName})
	// Only generate on the primary resource path (single-segment, e.g. "/users"),
	// not on sub-resource paths (e.g. "/workspaces/{workspaceId}/users").
	isPrimaryPath := len(extractPathParams(c.resourcePath)) == 0
	for verbName, verbSummary := range c.typeInfo.CustomVerbSummary {
		if !isPrimaryPath {
			continue
		}
		verbPath := c.itemPath + ":" + verbName
		verbPathItem := getOrCreatePathItem(c.doc, verbPath)

		// Use explicit response type if annotated, otherwise derive from verb name
		var responseType string
		if rt, ok := c.typeInfo.CustomVerbResponse[verbName]; ok && rt != "" {
			responseType = rt
		} else {
			singular := strings.TrimSuffix(verbName, "s")
			responseType = strings.ToUpper(singular[:1]) + singular[1:] + "List"
		}
		responseRef := &Schema{Ref: fmt.Sprintf("#/components/schemas/%s", c.g.schemaKey(c.groupName, responseType))}

		verbParams := make([]Parameter, len(c.itemParams))
		copy(verbParams, c.itemParams)
		verbParams = append(verbParams,
			Parameter{Name: "page", In: "query", Schema: &Schema{Type: "integer"}},
			Parameter{Name: "pageSize", In: "query", Schema: &Schema{Type: "integer"}},
			Parameter{Name: "sortBy", In: "query", Schema: &Schema{Type: "string"}},
			Parameter{Name: "sortOrder", In: "query", Schema: &Schema{Type: "string", Enum: []string{"asc", "desc"}}},
		)

		verbPathItem.Get = &Operation{
			Summary:     verbSummary,
			OperationID: fmt.Sprintf("list%s%s", strings.ToUpper(verbName[:1])+verbName[1:], c.opSuffix),
			Tags:        []string{c.tag},
			Parameters:  verbParams,
			Responses: map[string]*Response{
				"200": {
					Description: "OK",
					Content: map[string]MediaType{
						"application/json": {Schema: responseRef},
					},
				},
			},
		}
	}
}

var pathParamRegexp = regexp.MustCompile(`\{(\w+)\}`)

// extractPathParams returns all {param} names from a path template.
func extractPathParams(path string) []string {
	matches := pathParamRegexp.FindAllStringSubmatch(path, -1)
	params := make([]string, 0, len(matches))
	for _, m := range matches {
		params = append(params, m[1])
	}
	return params
}

// deriveIDParam derives the item ID parameter name from the last segment
// of a resource path. e.g. "/users" -> "userId", "/namespaces/{namespaceId}/users" -> "userId"
func deriveIDParam(resourcePath string) string {
	seg := lastSegment(resourcePath)
	// Singularize: strip trailing "s"
	singular := strings.TrimSuffix(seg, "s")
	return singular + "Id"
}

// lastSegment returns the last path segment (without leading slash).
// e.g. "/namespaces/{namespaceId}/users" -> "users"
func lastSegment(path string) string {
	parts := strings.Split(strings.TrimSuffix(path, "/"), "/")
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] != "" && !strings.HasPrefix(parts[i], "{") {
			return parts[i]
		}
	}
	return ""
}

// operationSuffix builds a unique operation ID suffix from the path.
// For top-level "/users" → "User", for nested "/namespaces/{namespaceId}/users" → "NamespaceUser".
func operationSuffix(resourcePath, typeName string) string {
	parts := strings.Split(strings.Trim(resourcePath, "/"), "/")
	// Collect non-param segments
	var segments []string
	for _, p := range parts {
		if p != "" && !strings.HasPrefix(p, "{") {
			segments = append(segments, p)
		}
	}
	if len(segments) <= 1 {
		return typeName
	}
	// Build prefix from parent segments: "namespaces" → "Namespace"
	var prefix string
	for _, seg := range segments[:len(segments)-1] {
		singular := strings.TrimSuffix(seg, "s")
		prefix += strings.ToUpper(singular[:1]) + singular[1:]
	}
	return prefix + typeName
}

func sortedKeys(m map[string][]Tag) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// toCamelCase converts "change-password" to "changePassword".
func toCamelCase(s string) string {
	parts := strings.Split(s, "-")
	for i := 1; i < len(parts); i++ {
		if len(parts[i]) > 0 {
			parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
		}
	}
	return strings.Join(parts, "")
}

func getOrCreatePathItem(doc *Document, path string) *PathItem {
	if item, ok := doc.Paths[path]; ok {
		return item
	}
	item := &PathItem{}
	doc.Paths[path] = item
	return item
}
