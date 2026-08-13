// Package timeutil holds the nil/zero time conversions the store layer
// uses at the domain boundary: nullable timestamptz columns scan into
// *time.Time, while domain/API rows keep plain time.Time with IsZero()
// standing in for NULL (the wire behavior it has always had).
package timeutil

import "time"

// OrZero maps a nullable column value to the domain zero-time convention.
func OrZero(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

// Ptr maps a domain time to a nullable column parameter: the zero time
// becomes NULL, anything else a pointer to a copy.
func Ptr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	c := t
	return &c
}

// FormatOrEmpty renders t for UI/log use, "" when nil.
func FormatOrEmpty(t *time.Time, layout string) string {
	if t == nil {
		return ""
	}
	return t.Format(layout)
}
