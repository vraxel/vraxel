package pgerrors

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestCheckPG_Nil(t *testing.T) {
	if got := CheckPG(nil); got != nil {
		t.Errorf("CheckPG(nil) = %v, want nil", got)
	}
}

func TestCheckPG_NonPG(t *testing.T) {
	raw := errors.New("plain error")
	got := CheckPG(raw)
	if got != raw {
		t.Errorf("CheckPG(plain) = %v, want pass-through", got)
	}
}

func TestCheckPG_UniqueViolation(t *testing.T) {
	pgErr := &pgconn.PgError{Code: "23505", ConstraintName: "users_email_key"}
	got := CheckPG(pgErr)
	if !errors.Is(got, ErrConflict) {
		t.Errorf("CheckPG(23505) not tagged with ErrConflict: %v", got)
	}
	var out *pgconn.PgError
	if !errors.As(got, &out) || out.ConstraintName != "users_email_key" {
		t.Errorf("CheckPG(23505) lost pgconn.PgError details: %v", got)
	}
}

func TestCheckPG_ForeignKeyViolation(t *testing.T) {
	pgErr := &pgconn.PgError{Code: "23503", Detail: `Key (id)=(1) is referenced from "orders".`}
	got := CheckPG(pgErr)
	if !errors.Is(got, ErrConflict) {
		t.Errorf("CheckPG(23503) not tagged with ErrConflict: %v", got)
	}
	var out *pgconn.PgError
	if !errors.As(got, &out) || out.Detail == "" {
		t.Errorf("CheckPG(23503) lost pgconn.PgError details: %v", got)
	}
}

func TestCheckPG_RestrictViolation(t *testing.T) {
	pgErr := &pgconn.PgError{Code: "23001"}
	got := CheckPG(pgErr)
	if !errors.Is(got, ErrConflict) {
		t.Errorf("CheckPG(23001) not tagged with ErrConflict: %v", got)
	}
}

func TestCheckPG_StringTruncation(t *testing.T) {
	pgErr := &pgconn.PgError{Code: "22001"}
	got := CheckPG(pgErr)
	if !errors.Is(got, ErrBadRequest) {
		t.Errorf("CheckPG(22001) not tagged with ErrBadRequest: %v", got)
	}
}

func TestCheckPG_UnknownCode(t *testing.T) {
	pgErr := &pgconn.PgError{Code: "42P01"}
	got := CheckPG(pgErr)
	if got != pgErr {
		t.Errorf("CheckPG(unknown code) = %v, want pass-through", got)
	}
}

func TestSentinels_NotEqual(t *testing.T) {
	if errors.Is(ErrNotFound, ErrConflict) || errors.Is(ErrConflict, ErrBadRequest) {
		t.Error("sentinels must be distinct")
	}
}
