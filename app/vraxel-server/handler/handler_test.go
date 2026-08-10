package handler

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

// fakeFrontendFS mirrors the vite output layout: hashed bundles under
// assets/, and a few mutable entry-point files at the root.
func fakeFrontendFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html":           {Data: []byte("<!doctype html><html></html>")},
		"favicon.svg":          {Data: []byte("<svg/>")},
		"assets/main-AAA.js":   {Data: []byte("console.log(1)")},
		"assets/style-BBB.css": {Data: []byte("body{}")},
	}
}

// TestServeFrontend_CacheHeaders pins the SPA cache policy so future
// edits to serveFrontend cannot silently drop it. assets/* must be
// pinned forever (vite content-hashed); everything else must
// revalidate on every load (mutable entry points).
func TestServeFrontend_CacheHeaders(t *testing.T) {
	cases := []struct {
		path   string
		wantCC string
	}{
		// Direct-hit branch (distFS.Open succeeds): root is rewritten to
		// index.html in serveFrontend before lookup, so it lands here too.
		{"/", "no-cache"},
		{"/index.html", "no-cache"},
		{"/favicon.svg", "no-cache"},
		{"/assets/main-AAA.js", "public, max-age=31536000, immutable"},
		{"/assets/style-BBB.css", "public, max-age=31536000, immutable"},
		// SPA fallback branch (distFS.Open fails -> rewrite path to "/").
		{"/some/spa/route", "no-cache"},
	}

	distFS := fakeFrontendFS()
	staticHandler := http.FileServer(http.FS(distFS))

	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, tc.path, nil)
			w := httptest.NewRecorder()
			serveFrontend(w, r, distFS, staticHandler)
			got := w.Result().Header.Get("Cache-Control")
			if got != tc.wantCC {
				t.Errorf("path=%s: Cache-Control = %q, want %q", tc.path, got, tc.wantCC)
			}
		})
	}
}

// TestNewRootHandler_OpenAPISpec_ETag pins the conditional-GET path
// for /docs/openapi.json: a fresh GET returns 200 + body + Cache-Control
// no-cache + a stable ETag; a follow-up GET that echoes the same ETag
// in If-None-Match returns 304 + no body. Without this, regressions
// would silently turn every revalidation into a full re-download.
func TestNewRootHandler_OpenAPISpec_ETag(t *testing.T) {
	spec := []byte(`{"openapi":"3.0.3","info":{"title":"x","version":"1"},"paths":{}}`)
	rh := NewRootHandler(RootHandlerConfig{OpenAPISpec: spec})

	// First GET: full body.
	r1 := httptest.NewRequest(http.MethodGet, "/docs/openapi.json", nil)
	w1 := httptest.NewRecorder()
	if !rh(w1, r1) {
		t.Fatal("/docs/openapi.json not handled on first GET")
	}
	res1 := w1.Result()
	if res1.StatusCode != http.StatusOK {
		t.Fatalf("first GET status = %d, want 200", res1.StatusCode)
	}
	if got := res1.Header.Get("Cache-Control"); got != "no-cache" {
		t.Errorf("first GET Cache-Control = %q, want no-cache", got)
	}
	etag := res1.Header.Get("ETag")
	if etag == "" {
		t.Fatal("first GET missing ETag header")
	}
	body1, _ := io.ReadAll(res1.Body)
	if len(body1) != len(spec) {
		t.Errorf("first GET body length = %d, want %d", len(body1), len(spec))
	}

	// Second GET with matching If-None-Match: 304 + empty body.
	r2 := httptest.NewRequest(http.MethodGet, "/docs/openapi.json", nil)
	r2.Header.Set("If-None-Match", etag)
	w2 := httptest.NewRecorder()
	rh(w2, r2)
	res2 := w2.Result()
	if res2.StatusCode != http.StatusNotModified {
		t.Fatalf("conditional GET status = %d, want 304", res2.StatusCode)
	}
	body2, _ := io.ReadAll(res2.Body)
	if len(body2) != 0 {
		t.Errorf("304 body length = %d, want 0", len(body2))
	}

	// Third GET with a non-matching If-None-Match: 200 + body again.
	r3 := httptest.NewRequest(http.MethodGet, "/docs/openapi.json", nil)
	r3.Header.Set("If-None-Match", `"deadbeefdeadbeef"`)
	w3 := httptest.NewRecorder()
	rh(w3, r3)
	if got := w3.Result().StatusCode; got != http.StatusOK {
		t.Errorf("stale-ETag GET status = %d, want 200", got)
	}
}
