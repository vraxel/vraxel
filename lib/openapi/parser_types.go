package openapi

import (
	"go/ast"
	"strings"
)

// applyPackageAnnotations folds a package's doc.go +openapi: group metadata
// (groupName / groupVersion / moduleName) into the group being built.
func applyPackageAnnotations(group *GroupInfo, pkg *ast.Package) {
	for _, file := range pkg.Files {
		pa := ParsePackageAnnotations(file)
		if pa.GroupVersion != "" {
			group.GroupName = pa.GroupName
			group.GroupVersion = pa.GroupVersion
		}
		if pa.ModuleName != "" {
			group.ModuleName = pa.ModuleName
		}
	}
}

// collectGroupTypes appends a TypeInfo for every exported, +openapi:-annotated
// type (struct or aliased schema) in the package. Unexported types (storage
// types) and types without annotations are skipped.
func collectGroupTypes(group *GroupInfo, pkg *ast.Package) {
	for _, ts := range typeSpecs(pkg) {
		if !ts.spec.Name.IsExported() {
			continue
		}
		annotations := ParseAnnotations(ts.doc)
		if len(annotations) == 0 {
			continue
		}
		if ti, ok := buildTypeInfo(ts.spec, annotations, pkg.Name); ok {
			group.Types = append(group.Types, ti)
		}
	}
}

// buildTypeInfo routes a type spec to the alias or struct builder. Non-struct
// types are treated as aliases (emitted as a $ref to the target schema).
func buildTypeInfo(spec *ast.TypeSpec, annotations []Annotation, pkgName string) (TypeInfo, bool) {
	if _, isStruct := spec.Type.(*ast.StructType); !isStruct {
		return buildAliasTypeInfo(spec, annotations, pkgName)
	}
	return buildStructTypeInfo(spec, annotations, pkgName), true
}

// buildAliasTypeInfo builds a schema-only TypeInfo for a type alias annotated
// with +openapi:schema, resolving the alias target. Returns ok=false for
// unresolvable targets or aliases not marked +openapi:schema.
func buildAliasTypeInfo(spec *ast.TypeSpec, annotations []Annotation, pkgName string) (TypeInfo, bool) {
	target := resolveAliasTargetName(spec.Type)
	if target == "" {
		return TypeInfo{}, false
	}
	hasSchema := false
	var description string
	for _, ann := range annotations {
		if ann.Key == "schema" {
			hasSchema = true
		}
		if ann.Key == "description" && description == "" {
			description = ann.Value
		}
	}
	if !hasSchema {
		return TypeInfo{}, false
	}
	return TypeInfo{
		Name:        spec.Name.Name,
		Package:     pkgName,
		Annotations: annotations,
		Description: description,
		SchemaOnly:  true,
		AliasTarget: target,
	}, true
}

// buildStructTypeInfo builds a TypeInfo from a struct type and its direct
// +openapi: annotations (schema / description / path / summary.METHOD /
// action.NAME.summary) plus its parsed fields.
func buildStructTypeInfo(spec *ast.TypeSpec, annotations []Annotation, pkgName string) TypeInfo {
	structType := spec.Type.(*ast.StructType)

	var description string
	var paths []string
	var schemaOnly bool
	opSummary := make(map[string]string)
	actionSummary := make(map[string]string)
	for _, ann := range annotations {
		switch {
		case ann.Key == "schema":
			schemaOnly = true
		case ann.Key == "description":
			if description == "" {
				description = ann.Value
			}
		case ann.Key == "path":
			if ann.Value != "" {
				paths = append(paths, ann.Value)
			}
		case strings.HasPrefix(ann.Key, "summary."):
			method := strings.TrimPrefix(ann.Key, "summary.")
			opSummary[method] = ann.Value
		case strings.HasPrefix(ann.Key, "action.") && strings.HasSuffix(ann.Key, ".summary"):
			actionName := strings.TrimPrefix(ann.Key, "action.")
			actionName = strings.TrimSuffix(actionName, ".summary")
			actionSummary[actionName] = ann.Value
		}
	}

	typeInfo := TypeInfo{
		Name:               spec.Name.Name,
		Package:            pkgName,
		Annotations:        annotations,
		Description:        description,
		IsListType:         strings.HasSuffix(spec.Name.Name, "List"),
		SchemaOnly:         schemaOnly,
		Paths:              paths,
		OperationSummary:   opSummary,
		ActionSummary:      actionSummary,
		CustomVerbSummary:  make(map[string]string),
		CustomVerbResponse: make(map[string]string),
	}
	for _, field := range structType.Fields.List {
		if fi := parseField(field); fi != nil {
			typeInfo.Fields = append(typeInfo.Fields, *fi)
		}
	}
	return typeInfo
}
