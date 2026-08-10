package apiserver

import (
	"bytes"
	"io"
	"net/http/httptest"
	"testing"
)

func TestCaptureRequestBody_SmallJSONCapturedAndRestored(t *testing.T) {
	body := []byte(`{"a":1}`)
	r := httptest.NewRequest("POST", "/x", bytes.NewReader(body))
	capture := captureRequestBody(r, false)
	if string(capture) != `{"a":1}` {
		t.Errorf("capture = %s", capture)
	}
	got, err := io.ReadAll(r.Body)
	if err != nil || !bytes.Equal(got, body) {
		t.Fatalf("restored body mismatch: %q err=%v", got, err)
	}
}

// The middleware must not buffer arbitrarily large bodies (memory bound is
// maxBodyCapture+1), while the handler downstream must still receive the
// FULL body via the spliced reader.
func TestCaptureRequestBody_LargeBodyNotCapturedButPreserved(t *testing.T) {
	big := bytes.Repeat([]byte("a"), maxBodyCapture*3)
	r := httptest.NewRequest("POST", "/x", bytes.NewReader(big))
	capture := captureRequestBody(r, false)
	if capture != nil {
		t.Errorf("oversized body must not be captured, got %d bytes", len(capture))
	}
	got, err := io.ReadAll(r.Body)
	if err != nil || !bytes.Equal(got, big) {
		t.Fatalf("handler must still see the full body: got %d want %d err=%v", len(got), len(big), err)
	}
}

func TestCaptureRequestBody_SensitiveSkipsButPreserves(t *testing.T) {
	body := []byte(`{"secret":"x"}`)
	r := httptest.NewRequest("POST", "/x", bytes.NewReader(body))
	if c := captureRequestBody(r, true); c != nil {
		t.Errorf("sensitive capture must be nil, got %s", c)
	}
	got, _ := io.ReadAll(r.Body)
	if !bytes.Equal(got, body) {
		t.Fatalf("sensitive body must still flow to the handler")
	}
}
