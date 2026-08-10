package rest

import (
	"net/http/httptest"
	"testing"
)

func TestWriteFile(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteFile(rec, 200, &FileResponse{
		FileName:    "test.pem",
		ContentType: "application/x-pem-file",
		Data:        []byte("-----BEGIN CERTIFICATE-----\ntest\n-----END CERTIFICATE-----\n"),
	})

	if rec.Code != 200 {
		t.Fatalf("status=%d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/x-pem-file" {
		t.Fatalf("Content-Type=%q, want application/x-pem-file", ct)
	}
	if cd := rec.Header().Get("Content-Disposition"); cd != `attachment; filename="test.pem"` {
		t.Fatalf("Content-Disposition=%q", cd)
	}
	if rec.Body.String() != "-----BEGIN CERTIFICATE-----\ntest\n-----END CERTIFICATE-----\n" {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
}
