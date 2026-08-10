package handler

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewRootHandler_Livez(t *testing.T) {
	rh := NewRootHandler(RootHandlerConfig{})
	r := httptest.NewRequest(http.MethodGet, "/livez", nil)
	w := httptest.NewRecorder()
	if !rh(w, r) {
		t.Fatal("/livez not handled")
	}
	res := w.Result()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	body, _ := io.ReadAll(res.Body)
	if string(body) != "OK" {
		t.Fatalf("body = %q, want OK", body)
	}
}

func TestNewRootHandler_Readyz_NoChecks(t *testing.T) {
	rh := NewRootHandler(RootHandlerConfig{})
	for _, p := range []string{"/readyz", "/healthz"} {
		r := httptest.NewRequest(http.MethodGet, p, nil)
		w := httptest.NewRecorder()
		if !rh(w, r) {
			t.Fatalf("%s not handled", p)
		}
		if w.Result().StatusCode != http.StatusOK {
			t.Fatalf("%s status = %d, want 200", p, w.Result().StatusCode)
		}
	}
}

func TestNewRootHandler_Readyz_AllOK(t *testing.T) {
	rh := NewRootHandler(RootHandlerConfig{
		ReadinessChecks: []ReadinessCheck{
			{Name: "database", Fn: func(context.Context) error { return nil }},
			{Name: "cache", Fn: func(context.Context) error { return nil }},
		},
	})
	r := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()
	rh(w, r)

	res := w.Result()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	body, _ := io.ReadAll(res.Body)
	for _, want := range []string{"[+]database ok", "[+]cache ok"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("body missing %q: %s", want, body)
		}
	}
}

func TestNewRootHandler_Readyz_Fails(t *testing.T) {
	rh := NewRootHandler(RootHandlerConfig{
		ReadinessChecks: []ReadinessCheck{
			{Name: "database", Fn: func(context.Context) error { return errors.New("conn refused") }},
			{Name: "cache", Fn: func(context.Context) error { return nil }},
		},
	})
	r := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	rh(w, r)

	res := w.Result()
	if res.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", res.StatusCode)
	}
	body, _ := io.ReadAll(res.Body)
	if !strings.Contains(string(body), "[-]database failed\n") {
		t.Errorf("body missing failure line: %s", body)
	}
	if strings.Contains(string(body), "conn refused") {
		t.Errorf("body must not leak underlying error: %s", body)
	}
	if !strings.Contains(string(body), "[+]cache ok") {
		t.Errorf("body missing cache ok: %s", body)
	}
}
