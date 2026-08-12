package openapi

import (
	"go/ast"
	"go/token"
	"strings"
)

// storageInfo holds the derived routing metadata for a single unexported
// *Storage type discovered during annotation scanning.
type storageInfo struct {
	resourceName    string
	derivedPath     string
	qualifiedPrefix string
	extraPaths      []string
	tag             string
	noDerive        bool
}

// typeSpecWithDoc pairs a type spec with the doc comment of its enclosing
// GenDecl, where +openapi: annotations live for single-spec type declarations.
type typeSpecWithDoc struct {
	spec *ast.TypeSpec
	doc  *ast.CommentGroup
}

// typeSpecs flattens a package's type declarations into a single slice so
// callers iterate once instead of nesting file -> decl -> spec.
func typeSpecs(pkg *ast.Package) []typeSpecWithDoc {
	var out []typeSpecWithDoc
	for _, file := range sortedFiles(pkg) {
		for _, decl := range file.Decls {
			genDecl, ok := decl.(*ast.GenDecl)
			if !ok || genDecl.Tok != token.TYPE {
				continue
			}
			for _, spec := range genDecl.Specs {
				if ts, ok := spec.(*ast.TypeSpec); ok {
					out = append(out, typeSpecWithDoc{spec: ts, doc: genDecl.Doc})
				}
			}
		}
	}
	return out
}

// funcDecls flattens a package's function declarations into a single slice.
func funcDecls(pkg *ast.Package) []*ast.FuncDecl {
	var out []*ast.FuncDecl
	for _, file := range sortedFiles(pkg) {
		for _, decl := range file.Decls {
			if fd, ok := decl.(*ast.FuncDecl); ok {
				out = append(out, fd)
			}
		}
	}
	return out
}

// collectStorageTypes scans a package for unexported *Storage / *Ops
// struct types (the REST layer's naming before and after the typed-Ops
// conversion) and records their derived/overridden routing metadata.
func collectStorageTypes(pkg *ast.Package, storageTypes map[string]*storageInfo) {
	for _, ts := range typeSpecs(pkg) {
		name := ts.spec.Name.Name
		if ts.spec.Name.IsExported() {
			continue
		}
		var base string
		switch {
		case strings.HasSuffix(name, "Storage"):
			base = strings.TrimSuffix(name, "Storage")
		case strings.HasSuffix(name, "Ops"):
			base = strings.TrimSuffix(name, "Ops")
		default:
			continue
		}
		storageTypes[name] = buildStorageInfo(base, ts.doc)
	}
}

// buildStorageInfo derives the resource name and path from a storage
// base name (type name minus the Storage/Ops suffix) and applies any
// +openapi:path / resource / tag / noDerive overrides.
func buildStorageInfo(base string, doc *ast.CommentGroup) *storageInfo {
	resource, path := deriveResourceAndPath(splitCamelCase(base))

	var extraPaths []string
	var overrideResource, overrideTag string
	noDerive := false
	for _, ann := range ParseAnnotations(doc) {
		switch ann.Key {
		case "path":
			if ann.Value != "" {
				extraPaths = append(extraPaths, ann.Value)
			}
		case "resource":
			if ann.Value != "" {
				overrideResource = ann.Value
			}
		case "tag":
			if ann.Value != "" {
				overrideTag = ann.Value
			}
		case "noDerive":
			noDerive = true
		}
	}
	if overrideResource != "" {
		resource = overrideResource
	}

	// The qualified prefix keys this storage's summaries. It must come
	// from the path the storage actually serves (+openapi:path) when one
	// is declared: three rolebinding storages derive three different
	// names but all collapse to the same empty prefix, so the last one
	// parsed silently overwrote the others' summaries.
	prefix := pathToQualifiedPrefix(path)
	if len(extraPaths) > 0 {
		prefix = pathToQualifiedPrefix(extraPaths[0])
	}

	return &storageInfo{
		resourceName:    resource,
		derivedPath:     path,
		qualifiedPrefix: prefix,
		extraPaths:      extraPaths,
		tag:             overrideTag,
		noDerive:        noDerive,
	}
}

// mergeStoragePaths merges each storage type's derived and explicit paths plus
// its tag override into the matching TypeInfo.
func mergeStoragePaths(group *GroupInfo, typeIndex map[string]int, storageTypes map[string]*storageInfo) {
	for _, st := range storageTypes {
		idx, ok := typeIndex[st.resourceName]
		if !ok {
			continue
		}
		if !st.noDerive {
			group.Types[idx].Paths = appendUnique(group.Types[idx].Paths, st.derivedPath)
		}
		for _, ep := range st.extraPaths {
			group.Types[idx].Paths = appendUnique(group.Types[idx].Paths, ep)
		}
		if st.tag != "" {
			group.Types[idx].Tag = st.tag
		}
	}
}

// scanFuncAnnotations scans a package's functions: storage-type methods
// feed operation summaries; standalone functions feed action /
// customverb summaries. Which operations exist is not inferred here --
// that comes from the apiserver route table (see Route).
func scanFuncAnnotations(pkg *ast.Package, group *GroupInfo, typeIndex map[string]int, storageTypes map[string]*storageInfo) {
	for _, fd := range funcDecls(pkg) {
		if fd.Recv != nil {
			applyMethodAnnotation(group, typeIndex, storageTypes, fd)
		} else {
			applyStandaloneFuncAnnotation(group, typeIndex, fd)
		}
	}
}

// applyMethodAnnotation merges a storage-type method's
// +openapi:summary / summary.QUALIFIER into the TypeInfo.
func applyMethodAnnotation(group *GroupInfo, typeIndex map[string]int, storageTypes map[string]*storageInfo, fd *ast.FuncDecl) {
	recvType := receiverTypeName(fd.Recv)
	st, ok := storageTypes[recvType]
	if !ok {
		return
	}
	op, ok := methodToOperation(fd.Name.Name)
	if !ok {
		return
	}

	annotations := ParseAnnotations(fd.Doc)
	if len(annotations) == 0 {
		return
	}
	idx, ok := typeIndex[st.resourceName]
	if !ok {
		return
	}
	for _, ann := range annotations {
		switch {
		case ann.Key == "summary":
			group.Types[idx].OperationSummary[operationKey(st.qualifiedPrefix, op)] = ann.Value
		case strings.HasPrefix(ann.Key, "summary."):
			qualifier := strings.TrimPrefix(ann.Key, "summary.")
			group.Types[idx].OperationSummary[qualifier+"."+op] = ann.Value
		}
	}
}

// applyStandaloneFuncAnnotation merges +openapi:action / customverb metadata
// from a standalone function into the resource named by +openapi:resource.
func applyStandaloneFuncAnnotation(group *GroupInfo, typeIndex map[string]int, fd *ast.FuncDecl) {
	annotations := ParseAnnotations(fd.Doc)
	if len(annotations) == 0 {
		return
	}
	var actionName, customVerb, summary, resource, response string
	for _, ann := range annotations {
		switch ann.Key {
		case "action":
			actionName = ann.Value
		case "customverb":
			customVerb = ann.Value
		case "summary":
			summary = ann.Value
		case "resource":
			resource = ann.Value
		case "response":
			response = ann.Value
		}
	}
	applyActionAnnotation(group, typeIndex, actionName, resource, summary)
	applyCustomVerbAnnotation(group, typeIndex, customVerb, resource, summary, response)
}

// applyActionAnnotation sets an ActionSummary entry when a standalone function
// declares +openapi:action plus a resolvable +openapi:resource and summary.
func applyActionAnnotation(group *GroupInfo, typeIndex map[string]int, actionName, resource, summary string) {
	if actionName == "" || resource == "" {
		return
	}
	if idx, ok := typeIndex[resource]; ok && summary != "" {
		group.Types[idx].ActionSummary[actionName] = summary
	}
}

// applyCustomVerbAnnotation sets CustomVerbSummary / CustomVerbResponse entries
// for a standalone function declaring +openapi:customverb on a resource.
func applyCustomVerbAnnotation(group *GroupInfo, typeIndex map[string]int, customVerb, resource, summary, response string) {
	if customVerb == "" || resource == "" {
		return
	}
	idx, ok := typeIndex[resource]
	if !ok {
		return
	}
	if summary != "" {
		group.Types[idx].CustomVerbSummary[customVerb] = summary
	}
	if response != "" {
		group.Types[idx].CustomVerbResponse[customVerb] = response
	}
}
