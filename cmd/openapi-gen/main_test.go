package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSpecMatchesRouteTable guards the committed spec against drift: the
// generator now derives paths from the apiserver's route table, so any
// route added without re-running `make generate` shows up here instead
// of shipping a spec that lies about the API.
func TestSpecMatchesRouteTable(t *testing.T) {
	specPath := filepath.Join("..", "..", "app", "vraxel-server", "apis", "openapi.json")
	raw, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatalf("read committed spec: %v", err)
	}
	var doc struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse committed spec: %v", err)
	}

	inSpec := make(map[string]bool)
	for path, ops := range doc.Paths {
		for method := range ops {
			switch method {
			case "get", "post", "put", "patch", "delete":
				inSpec[method+" "+path] = true
			}
		}
	}

	registered := routes()
	if len(registered) == 0 {
		t.Fatal("no routes registered -- the comparison would pass vacuously")
	}
	for _, r := range registered {
		// HEAD / OPTIONS exist only on the k8s proxy passthrough, which
		// forwards every method; the spec documents the standard verbs,
		// not those two, so they are not expected to appear.
		if r.Method == "HEAD" || r.Method == "OPTIONS" {
			continue
		}
		key := strings.ToLower(r.Method) + " " + r.Path
		if !inSpec[key] {
			t.Errorf("route %s %s is served but absent from the committed spec (run `make generate`)", r.Method, r.Path)
		}
		delete(inSpec, key)
	}
	for key := range inSpec {
		// /oidc and /.well-known are declared by annotation on their
		// handlers, not registered on the apiserver, so they have no
		// route here. Everything under /api must.
		if strings.Contains(key, " /api/") {
			t.Errorf("spec declares %q with no matching route", key)
		}
	}
}
