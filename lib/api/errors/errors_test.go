package errors

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"vraxel.io/vraxel/lib/api/validation"
	"vraxel.io/vraxel/pkg/db/pgerrors"
)

func TestNewBadRequest(t *testing.T) {
	err := NewBadRequest("invalid input", nil)
	if err.Status != 400 || err.Reason != "BadRequest" {
		t.Errorf("unexpected: %+v", err)
	}
	if err.Error() == "" {
		t.Error("expected non-empty error message")
	}
}

func TestNewNotFound(t *testing.T) {
	err := NewNotFound("User", "alice")
	if err.Status != 404 || err.Reason != "NotFound" {
		t.Errorf("unexpected: %+v", err)
	}
}

func TestNewConflict(t *testing.T) {
	err := NewConflict("User", "alice")
	if err.Status != 409 || err.Reason != "Conflict" {
		t.Errorf("unexpected: %+v", err)
	}
}

func TestNewInternalError(t *testing.T) {
	err := NewInternalError(nil)
	if err.Status != 500 {
		t.Errorf("unexpected: %+v", err)
	}
}

func TestNewForbidden(t *testing.T) {
	err := NewForbidden("access denied")
	if err.Status != 403 || err.Reason != "Forbidden" {
		t.Errorf("unexpected: %+v", err)
	}
	if err.Message != "access denied" {
		t.Errorf("unexpected message: %s", err.Message)
	}
}

func TestIsForbidden(t *testing.T) {
	err := NewForbidden("no access")
	if !IsForbidden(err) {
		t.Error("expected IsForbidden to be true")
	}
	if IsForbidden(NewNotFound("User", "alice")) {
		t.Error("expected IsForbidden to be false for NotFound")
	}
	if IsForbidden(NewBadRequest("x", nil)) {
		t.Error("expected IsForbidden to be false for BadRequest")
	}
}

func TestIsNotFound(t *testing.T) {
	err := NewNotFound("User", "alice")
	if !IsNotFound(err) {
		t.Error("expected IsNotFound to be true")
	}
	if IsNotFound(NewBadRequest("x", nil)) {
		t.Error("expected IsNotFound to be false for BadRequest")
	}
}

func TestFromDomain_Nil(t *testing.T) {
	if FromDomain(nil, "x") != nil {
		t.Error("FromDomain(nil) should be nil")
	}
}

func TestFromDomain_UnknownError(t *testing.T) {
	if got := FromDomain(errors.New("random"), "x"); got != nil {
		t.Errorf("FromDomain(random) = %v, want nil", got)
	}
}

func TestFromDomain_SentinelNotFound(t *testing.T) {
	err := fmt.Errorf("namespace 42: %w", pgerrors.ErrNotFound)
	got := FromDomain(err, "namespace")
	if got == nil || got.Status != 404 || got.Reason != "NotFound" {
		t.Errorf("FromDomain(ErrNotFound wrap) = %+v", got)
	}
}

func TestFromDomain_SentinelConflict(t *testing.T) {
	err := fmt.Errorf("x: %w", pgerrors.ErrConflict)
	got := FromDomain(err, "x")
	if got == nil || got.Status != 409 || got.Reason != "Conflict" {
		t.Errorf("FromDomain(ErrConflict wrap) = %+v", got)
	}
}

func TestFromDomain_SentinelBadRequest(t *testing.T) {
	err := fmt.Errorf("x: %w", pgerrors.ErrBadRequest)
	got := FromDomain(err, "x")
	if got == nil || got.Status != 400 || got.Reason != "BadRequest" {
		t.Errorf("FromDomain(ErrBadRequest wrap) = %+v", got)
	}
}

func TestFromDomain_UniqueViolationMatchesLegacy(t *testing.T) {
	pgErr := &pgconn.PgError{Code: "23505", ConstraintName: "users_email_key"}
	wrapped := pgerrors.CheckPG(pgErr)

	got := FromDomain(wrapped, "user")
	legacy := CheckPGError(pgErr, "user").(*StatusError)

	if got.Status != legacy.Status || got.Reason != legacy.Reason || got.Message != legacy.Message {
		t.Errorf("FromDomain diverges from CheckPGError.\n got    = %+v\n legacy = %+v", got, legacy)
	}
}

func TestFromDomain_FKWithDetailMatchesLegacy(t *testing.T) {
	pgErr := &pgconn.PgError{Code: "23503", Detail: `Key (id)=(1) is referenced from "orders".`}
	wrapped := pgerrors.CheckPG(pgErr)

	got := FromDomain(wrapped, "user")
	legacy := CheckPGError(pgErr, "user").(*StatusError)

	if got.Status != legacy.Status || got.Reason != legacy.Reason || got.Message != legacy.Message {
		t.Errorf("FromDomain diverges from CheckPGError.\n got    = %+v\n legacy = %+v", got, legacy)
	}
}

func TestFromDomain_FKNoDetailMatchesLegacy(t *testing.T) {
	pgErr := &pgconn.PgError{Code: "23503"}
	wrapped := pgerrors.CheckPG(pgErr)

	got := FromDomain(wrapped, "user")
	legacy := CheckPGError(pgErr, "user").(*StatusError)

	if got.Status != legacy.Status || got.Reason != legacy.Reason || got.Message != legacy.Message {
		t.Errorf("FromDomain diverges from CheckPGError.\n got    = %+v\n legacy = %+v", got, legacy)
	}
}

func TestFromDomain_StringTruncationMatchesLegacy(t *testing.T) {
	pgErr := &pgconn.PgError{Code: "22001"}
	wrapped := pgerrors.CheckPG(pgErr)

	got := FromDomain(wrapped, "user")
	legacy := CheckPGError(pgErr, "user").(*StatusError)

	if got.Status != legacy.Status || got.Reason != legacy.Reason || got.Message != legacy.Message {
		t.Errorf("FromDomain diverges from CheckPGError.\n got    = %+v\n legacy = %+v", got, legacy)
	}
}

func TestNewInvalidJSONBody_TypeError(t *testing.T) {
	type req struct {
		ShiftBy int64 `json:"shiftBy"`
	}
	var r req
	rawErr := json.Unmarshal([]byte(`{"shiftBy": 1e+22}`), &r)
	if rawErr == nil {
		t.Fatal("expected json.Unmarshal to fail on overflow")
	}
	se := NewInvalidJSONBody(rawErr)
	if se.Status != 400 {
		t.Errorf("status: got %d want 400", se.Status)
	}
	if se.Reason != "InvalidJSONBody" {
		t.Errorf("reason: got %q want InvalidJSONBody", se.Reason)
	}
	// Must NOT leak Go-internal type names from the underlying error.
	for _, leak := range []string{"int64", "Go struct", "kafkaResetOffsetsRequest"} {
		if strings.Contains(se.Message, leak) {
			t.Errorf("message leaks Go internal %q: %s", leak, se.Message)
		}
	}
	// Must reference the JSON field name.
	if !strings.Contains(se.Message, "shiftBy") {
		t.Errorf("message does not reference JSON field: %s", se.Message)
	}
	details, ok := se.Details.(validation.ErrorList)
	if !ok || len(details) != 1 || details[0].Field != "shiftBy" {
		t.Errorf("expected ErrorList with shiftBy field, got %#v", se.Details)
	}
}

func TestNewInvalidJSONBody_SyntaxError(t *testing.T) {
	type req struct{ A int }
	var r req
	rawErr := json.Unmarshal([]byte(`{"A": `), &r)
	if rawErr == nil {
		t.Fatal("expected SyntaxError")
	}
	se := NewInvalidJSONBody(rawErr)
	if se.Status != 400 || se.Reason != "InvalidJSONBody" {
		t.Errorf("unexpected: %+v", se)
	}
	if se.Details != nil {
		t.Errorf("syntax errors carry no field details, got %#v", se.Details)
	}
	if !strings.Contains(se.Message, "malformed") {
		t.Errorf("message does not describe syntax error: %s", se.Message)
	}
}

func TestNewInvalidJSONBody_GenericError(t *testing.T) {
	se := NewInvalidJSONBody(errors.New("io: read failed"))
	if se.Status != 400 || se.Reason != "InvalidJSONBody" {
		t.Errorf("unexpected: %+v", se)
	}
	if se.Details != nil {
		t.Errorf("generic errors carry no details, got %#v", se.Details)
	}
}
