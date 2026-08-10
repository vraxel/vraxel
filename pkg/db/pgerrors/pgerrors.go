// Package pgerrors defines domain-level error sentinels for the store layer.
//
// Stores return errors tagged with these sentinels ("this is a not-found /
// conflict / bad-input") without depending on HTTP concepts from lib/api/errors.
// The REST layer re-maps them to HTTP status via lib/api/errors.FromDomain.
package pgerrors

import (
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrNotFound   = errors.New("not found")
	ErrConflict   = errors.New("conflict")
	ErrBadRequest = errors.New("bad request")
	ErrForbidden  = errors.New("forbidden")
)

// CheckPG translates known PostgreSQL error codes into sentinel-tagged errors.
// The original *pgconn.PgError stays in the error chain so callers can extract
// ConstraintName / Detail for detailed HTTP responses.
//
// Codes handled:
//
//	22001 string_data_right_truncation -> ErrBadRequest
//	23001 restrict_violation           -> ErrConflict
//	23503 foreign_key_violation        -> ErrConflict
//	23505 unique_violation             -> ErrConflict
//
// pgx.ErrNoRows is deliberately not handled here — callers typically want to
// attach their own context (missing ID, resource name) at the call site, e.g.
//
//	fmt.Errorf("namespace %d: %w", id, pgerrors.ErrNotFound)
//
// Any other error (including nil) is returned unchanged.
func CheckPG(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return err
	}
	switch pgErr.Code {
	case "22001":
		return fmt.Errorf("%w: %w", ErrBadRequest, err)
	case "23001", "23503", "23505":
		return fmt.Errorf("%w: %w", ErrConflict, err)
	}
	return err
}
