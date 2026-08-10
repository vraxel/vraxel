package openapi

import (
	"go/ast"
	"strings"
)

const annotationPrefix = "+openapi:"

// Annotation represents a parsed OpenAPI annotation from a Go comment.
type Annotation struct {
	Key   string // e.g. "description", "required", "enum", "format"
	Value string // the value after "="
}

// ParseAnnotations extracts +openapi: annotations from a comment group.
func ParseAnnotations(doc *ast.CommentGroup) []Annotation {
	if doc == nil {
		return nil
	}

	var annotations []Annotation
	for _, comment := range doc.List {
		text := strings.TrimSpace(strings.TrimPrefix(comment.Text, "//"))
		if !strings.HasPrefix(text, annotationPrefix) {
			continue
		}

		directive := strings.TrimPrefix(text, annotationPrefix)
		key, value, _ := strings.Cut(directive, "=")
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)

		annotations = append(annotations, Annotation{Key: key, Value: value})
	}
	return annotations
}

// PackageAnnotation holds package-level OpenAPI metadata.
type PackageAnnotation struct {
	GroupName    string // +openapi:groupName=...
	GroupVersion string // +openapi:groupVersion=...
	ModuleName   string // +openapi:moduleName=...
}

// ParsePackageAnnotations extracts package-level annotations from a file's doc comments.
func ParsePackageAnnotations(file *ast.File) PackageAnnotation {
	var pa PackageAnnotation
	if file.Doc == nil {
		return pa
	}

	for _, ann := range ParseAnnotations(file.Doc) {
		switch ann.Key {
		case "groupName":
			pa.GroupName = ann.Value
		case "groupVersion":
			pa.GroupVersion = ann.Value
		case "moduleName":
			pa.ModuleName = ann.Value
		}
	}
	return pa
}

// FieldAnnotations holds annotations parsed from a struct field.
type FieldAnnotations struct {
	Description string
	Required    bool
	Enum        []string
	Format      string
}

// ParseFieldAnnotations extracts field-level annotations from a struct field's doc.
func ParseFieldAnnotations(doc *ast.CommentGroup) FieldAnnotations {
	var fa FieldAnnotations
	for _, ann := range ParseAnnotations(doc) {
		switch ann.Key {
		case "description":
			fa.Description = ann.Value
		case "required":
			fa.Required = ann.Value == "true" || ann.Value == ""
		case "enum":
			fa.Enum = strings.Split(ann.Value, ",")
			for i := range fa.Enum {
				fa.Enum[i] = strings.TrimSpace(fa.Enum[i])
			}
		case "format":
			fa.Format = ann.Value
		}
	}
	return fa
}
