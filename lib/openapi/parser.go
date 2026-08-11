package openapi

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// TypeInfo holds parsed information about an API type.
type TypeInfo struct {
	Name               string
	Package            string
	Fields             []FieldInfo
	Annotations        []Annotation
	Description        string
	IsListType         bool
	SchemaOnly         bool              // +openapi:schema — register in components/schemas but do not generate CRUD paths
	Paths              []string          // from +openapi:path annotations
	OperationSummary   map[string]string // from +openapi:summary.METHOD= (e.g. "list", "create", "get", "update", "patch", "delete", "deleteCollection")
	ActionSummary      map[string]string // from +openapi:action.NAME.summary= (e.g. "change-password")
	CustomVerbSummary  map[string]string // from +openapi:customverb= on standalone functions (e.g. "workspaces" → summary)
	CustomVerbResponse map[string]string // from +openapi:response= on custom verb functions (e.g. "rolebindings" → "RoleBindingList")
	Tag                string            // from +openapi:tag — override the default tag (type name) for grouping
	// AliasTarget is set when this TypeInfo represents a Go type alias
	// (e.g. `type MySQLCloudSource = paasdeploy.CloudSource`) annotated
	// with +openapi:schema. The generator emits the alias as a thin
	// $ref to the target schema rather than re-stamping fields. The
	// target schema must itself be annotated +openapi:schema or already
	// emitted as a top-level resource so the ref resolves.
	AliasTarget string
}

// FieldInfo holds parsed information about a struct field.
type FieldInfo struct {
	Name        string
	JSONName    string
	GoType      string
	OmitEmpty   bool
	Annotations FieldAnnotations
}

// EndpointInfo holds parsed information about a standalone HTTP endpoint
// defined via +openapi:endpoint annotations on functions.
type EndpointInfo struct {
	Path        string             // +openapi:path=...
	Method      string             // +openapi:method=GET|POST|PUT|PATCH|DELETE
	Summary     string             // +openapi:summary=...
	Description string             // +openapi:description=...
	Tag         string             // +openapi:tag=...
	OperationID string             // +openapi:operationId=...
	RequestBody *EndpointBody      // +openapi:requestBody.contentType=... and +openapi:requestBody.schema=...
	Responses   []EndpointResponse // +openapi:response.CODE.description=...
}

// EndpointBody describes the request body of a standalone endpoint.
type EndpointBody struct {
	ContentType string // e.g. "application/x-www-form-urlencoded", "application/json"
	SchemaRef   string // reference name or empty for generic object
}

// EndpointResponse describes a response of a standalone endpoint.
type EndpointResponse struct {
	StatusCode  string
	Description string
	ContentType string
	SchemaRef   string
}

// GroupInfo holds parsed information about an API group.
type GroupInfo struct {
	GroupName    string
	GroupVersion string
	ModuleName   string
	Types        []TypeInfo
	Endpoints    []EndpointInfo // standalone endpoints from +openapi:endpoint
}

// Parser scans Go source files for OpenAPI type definitions.
type Parser struct {
	rootDir string
}

// NewParser creates a parser that will scan from the given root directory.
func NewParser(rootDir string) *Parser {
	return &Parser{rootDir: rootDir}
}

// Parse scans the root directory for API type definitions.
// It looks for Go files with +openapi: annotations in each resource directory
// (e.g., pkg/apis/iam/) where doc.go and types.go define the API group.
func (p *Parser) Parse() ([]GroupInfo, error) {
	var groups []GroupInfo

	entries, err := os.ReadDir(p.rootDir)
	if err != nil {
		return nil, fmt.Errorf("read apis dir: %w", err)
	}

	// Walk every directory under the apis root (one level + an extra
	// level for any "non-resource grouping" parents like shared/).
	// parseGroup itself returns nil when a directory has no annotated
	// types, so the recursion is no-op for irrelevant subtrees and
	// new helper packages get picked up automatically without needing
	// to special-case parent dir names here.
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		resourceDir := filepath.Join(p.rootDir, entry.Name())
		group, err := p.parseGroup(resourceDir, entry.Name())
		if err != nil {
			return nil, fmt.Errorf("parse group %s: %w", entry.Name(), err)
		}
		if group != nil {
			groups = append(groups, *group)
			continue
		}
		// Walk one extra level for "container" dirs whose own types.go
		// is empty (shared/, store/, etc.). parseGroup returned nil
		// above iff len(group.Types) == 0; recurse into children to
		// find annotated types nested under a non-resource parent.
		subGroups, err := p.parseSubGroups(resourceDir, entry.Name())
		if err != nil {
			return nil, err
		}
		groups = append(groups, subGroups...)
	}

	return groups, nil
}

// parseSubGroups walks one extra directory level under a "container" dir
// (one whose own types.go produced no annotated types) and collects any
// annotated groups nested beneath it. An unreadable resourceDir yields no
// groups and no error (matches the original continue-on-read-error).
func (p *Parser) parseSubGroups(resourceDir, parentName string) ([]GroupInfo, error) {
	subEntries, subErr := os.ReadDir(resourceDir)
	if subErr != nil {
		return nil, nil
	}
	var groups []GroupInfo
	for _, sub := range subEntries {
		if !sub.IsDir() {
			continue
		}
		subDir := filepath.Join(resourceDir, sub.Name())
		subGroup, subErr := p.parseGroup(subDir, sub.Name())
		if subErr != nil {
			return nil, fmt.Errorf("parse %s/%s: %w", parentName, sub.Name(), subErr)
		}
		if subGroup != nil {
			groups = append(groups, *subGroup)
		}
	}
	return groups, nil
}

func (p *Parser) parseGroup(dir string, dirName string) (*GroupInfo, error) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse directory %s: %w", dir, err)
	}

	group := &GroupInfo{
		ModuleName: dirName,
	}

	for _, pkg := range pkgs {
		applyPackageAnnotations(group, pkg)

		collectGroupTypes(group, pkg)
	}

	// Phase 2: Scan storage types, methods, and standalone functions
	// to collect operation-level annotations and merge into TypeInfo.
	mergeStorageAnnotations(group, pkgs)

	// Phase 3: Scan for standalone endpoint annotations (+openapi:endpoint)
	group.Endpoints = parseEndpointAnnotations(pkgs)

	if len(group.Types) == 0 {
		return nil, nil
	}

	return group, nil
}

// mergeStorageAnnotations scans for storage types (*Storage structs),
// their methods, and standalone functions with +openapi: annotations,
// then merges the derived paths and summaries into the corresponding TypeInfo.
// resolveAliasTargetName extracts the target schema name from an
// alias's RHS expression. Handles both `type X Y` (named-type) and
// `type X foo.Y` (selector). Returns "" for forms openapi-gen
// doesn't model (array / map / func aliases).
func resolveAliasTargetName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		if t.Sel != nil {
			return t.Sel.Name
		}
	case *ast.StarExpr:
		return resolveAliasTargetName(t.X)
	}
	return ""
}

func mergeStorageAnnotations(group *GroupInfo, pkgs map[string]*ast.Package) {
	typeIndex := make(map[string]int, len(group.Types))
	for i, t := range group.Types {
		typeIndex[t.Name] = i
	}

	// storageTypes accumulates across packages; each package's merge,
	// func-annotation, and path-operation passes run against the running set.
	storageTypes := make(map[string]*storageInfo)
	for _, pkg := range pkgs {
		collectStorageTypes(pkg, storageTypes)
		mergeStoragePaths(group, typeIndex, storageTypes)
		scanFuncAnnotations(pkg, group, typeIndex, storageTypes)
	}
}

// splitCamelCase splits a camelCase name into lowercase segments.
// e.g., "workspaceUser" → ["workspace", "user"]
func splitCamelCase(s string) []string {
	var segments []string
	start := 0
	for i := 1; i < len(s); i++ {
		if s[i] >= 'A' && s[i] <= 'Z' {
			segments = append(segments, strings.ToLower(s[start:i]))
			start = i
		}
	}
	segments = append(segments, strings.ToLower(s[start:]))
	return segments
}

// deriveResourceAndPath derives the resource type name and API path from
// camelCase-split storage name segments.
// e.g., ["workspace", "user"] → ("User", "/workspaces/{workspaceId}/users")
func deriveResourceAndPath(segments []string) (resource, path string) {
	if len(segments) == 0 {
		return "", ""
	}
	last := segments[len(segments)-1]
	resource = strings.ToUpper(last[:1]) + last[1:]

	var parts []string
	for _, seg := range segments[:len(segments)-1] {
		parts = append(parts, seg+"s", "{"+seg+"Id}")
	}
	parts = append(parts, last+"s")
	path = "/" + strings.Join(parts, "/")
	return
}

// pathToQualifiedPrefix converts a path to a dotted qualifier prefix.
// e.g., "/workspaces/{workspaceId}/users" → "workspaces.users"
// Single-segment paths like "/users" return "".
func pathToQualifiedPrefix(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	var segments []string
	for _, p := range parts {
		if p != "" && !strings.HasPrefix(p, "{") {
			segments = append(segments, p)
		}
	}
	if len(segments) <= 1 {
		return ""
	}
	return strings.Join(segments, ".")
}

// operationKey builds the OperationSummary map key from a qualified prefix and operation.
func operationKey(qualifiedPrefix, op string) string {
	if qualifiedPrefix == "" {
		return op
	}
	return qualifiedPrefix + "." + op
}

// receiverTypeName extracts the type name from a method receiver.
func receiverTypeName(recv *ast.FieldList) string {
	if recv == nil || len(recv.List) == 0 {
		return ""
	}
	t := recv.List[0].Type
	if star, ok := t.(*ast.StarExpr); ok {
		t = star.X
	}
	if ident, ok := t.(*ast.Ident); ok {
		return ident.Name
	}
	return ""
}

// methodToOperation maps an Ops method name to its OpenAPI operation
// key. Ops methods are unexported (o.list, o.batchDelete), so the lookup
// is on the lower-camel form; the exported spelling stays accepted for
// any handler still written as a method on an exported storage type.
func methodToOperation(name string) (string, bool) {
	ops := map[string]string{
		"list":             "list",
		"create":           "create",
		"get":              "get",
		"update":           "update",
		"patch":            "patch",
		"delete":           "delete",
		"deleteCollection": "deleteCollection",
		"batchDelete":      "deleteCollection",
	}
	op, ok := ops[strings.ToLower(name[:1])+name[1:]]
	return op, ok
}

func appendUnique(slice []string, val string) []string {
	for _, s := range slice {
		if s == val {
			return slice
		}
	}
	return append(slice, val)
}

// parseEndpointAnnotations scans all functions for +openapi:endpoint annotations
// and builds EndpointInfo entries from them.
func parseEndpointAnnotations(pkgs map[string]*ast.Package) []EndpointInfo {
	var endpoints []EndpointInfo
	for _, pkg := range pkgs {
		for _, fd := range funcDecls(pkg) {
			if fd.Recv != nil {
				continue
			}
			if ep, ok := buildEndpointInfo(fd); ok {
				endpoints = append(endpoints, ep)
			}
		}
	}
	return endpoints
}

func parseField(field *ast.Field) *FieldInfo {
	if len(field.Names) == 0 {
		// Embedded field — skip for now
		return nil
	}

	name := field.Names[0].Name
	jsonName, omitEmpty := parseFieldJSONTag(field, name)

	return &FieldInfo{
		Name:        name,
		JSONName:    jsonName,
		GoType:      typeString(field.Type),
		OmitEmpty:   omitEmpty,
		Annotations: ParseFieldAnnotations(field.Doc),
	}
}

// parseFieldJSONTag resolves a struct field's JSON name and omitempty flag
// from its `json:"..."` tag, defaulting to defaultName with no omitempty
// when absent or set to "-".
func parseFieldJSONTag(field *ast.Field, defaultName string) (jsonName string, omitEmpty bool) {
	jsonName = defaultName
	if field.Tag == nil {
		return jsonName, omitEmpty
	}
	tag := strings.Trim(field.Tag.Value, "`")
	jsonTag := extractTag(tag, "json")
	if jsonTag == "" {
		return jsonName, omitEmpty
	}
	parts := strings.Split(jsonTag, ",")
	if parts[0] == "-" {
		return jsonName, omitEmpty
	}
	if parts[0] != "" {
		jsonName = parts[0]
	}
	for _, p := range parts[1:] {
		if p == "omitempty" {
			omitEmpty = true
		}
	}
	return jsonName, omitEmpty
}

func extractTag(tag, key string) string {
	search := key + `:`
	idx := strings.Index(tag, search)
	if idx < 0 {
		return ""
	}
	val := tag[idx+len(search):]
	if len(val) == 0 {
		return ""
	}
	quote := val[0]
	if quote != '"' {
		return ""
	}
	end := strings.IndexByte(val[1:], quote)
	if end < 0 {
		return ""
	}
	return val[1 : end+1]
}

func typeString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		return typeString(t.X) + "." + t.Sel.Name
	case *ast.StarExpr:
		return "*" + typeString(t.X)
	case *ast.ArrayType:
		return "[]" + typeString(t.Elt)
	case *ast.MapType:
		return "map[" + typeString(t.Key) + "]" + typeString(t.Value)
	default:
		return "interface{}"
	}
}
