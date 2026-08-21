package store

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// A breach write that loses the race with a rule deletion must read as
// success: the row's absence is what the deletion wanted, and every
// instance replays the race once per beat until its rule cache catches
// up.
func TestForeignKeyViolationIsBenign(t *testing.T) {
	fk := fmt.Errorf("wrapped: %w", &pgconn.PgError{Code: "23503"})
	if !isForeignKeyViolation(fk) {
		t.Fatal("a wrapped 23503 must be recognised")
	}
	if isForeignKeyViolation(errors.New("boom")) || isForeignKeyViolation(&pgconn.PgError{Code: "23505"}) {
		t.Fatal("only foreign-key violations qualify")
	}
}
