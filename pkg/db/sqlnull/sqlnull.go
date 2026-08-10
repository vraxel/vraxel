// Package sqlnull provides zero-value to nil-pointer adapters for sqlc's
// nullable parameters. sqlc generates `*T` (not sql.NullT) for nullable
// columns, so inputs with a zero Go value need to be translated to nil
// at the SQL boundary to become NULL.
//
// These helpers are store-layer only: they exist to serve the Params
// structs that sqlc emits, so they live under pkg/db alongside the other
// store-layer gateway subpackages (pgtime, hostlabel, hostagentsync).
package sqlnull

// ToNullString returns nil for an empty string, otherwise a pointer to s.
func ToNullString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// ToNullInt32 returns nil for 0, otherwise a pointer to n.
func ToNullInt32(n int32) *int32 {
	if n == 0 {
		return nil
	}
	return &n
}

// ToNullInt64 returns nil for 0, otherwise a pointer to n.
func ToNullInt64(n int64) *int64 {
	if n == 0 {
		return nil
	}
	return &n
}
