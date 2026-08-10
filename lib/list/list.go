// Package list holds the pure-Go value types that describe a paginated
// list request/response. Every layer of the API stack — handler,
// storage, and store impls alike — passes the same Query into a store,
// the store returns a Result. None of it depends on pgx/sqlc, so this
// package is safe to import from any layer including the REST handler
// surface, while pkg/db stays reserved for the store layer.
package list

import "strings"

// EscapeLikePattern escapes special LIKE/ILIKE characters (%, _, \) in a
// search string so they are treated as literals.
func EscapeLikePattern(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

// Pagination holds common pagination parameters.
type Pagination struct {
	Page      int    `json:"page"` // starts from 1
	PageSize  int    `json:"page_size"`
	SortBy    string `json:"sort_by"`
	SortOrder string `json:"sort_order"` // "asc" or "desc"
}

// Result is a generic paginated result.
type Result[T any] struct {
	Items      []T   `json:"items"`
	TotalCount int64 `json:"total_count"`
}

// Query holds generic filter + pagination parameters for list operations.
type Query struct {
	Filters map[string]any
	Pagination
	// MaxPageSize overrides the default per-request page-size cap (100).
	// Set this on export-style flows that legitimately need to pull
	// larger batches. Zero (the common case) keeps the default cap.
	MaxPageSize int32
}

// OffsetLimit converts the query's pagination to offset+limit, honoring
// MaxPageSize when set. Use this from store list functions that may also
// be invoked from export handlers.
func (q Query) OffsetLimit() (offset int32, limit int32) {
	if q.MaxPageSize > 0 {
		return PaginationToOffsetLimitWithMax(q.Pagination, q.MaxPageSize)
	}
	return PaginationToOffsetLimit(q.Pagination)
}

// PaginationToOffsetLimit converts Pagination to offset and limit with defaults.
// PageSize is capped at 100 to keep accidental ?page_size=999999 requests
// from pulling whole tables. Use PaginationToOffsetLimitWithMax for export
// or batch-processing flows that need a larger cap.
func PaginationToOffsetLimit(p Pagination) (offset int32, limit int32) {
	return PaginationToOffsetLimitWithMax(p, 100)
}

// PaginationToOffsetLimitWithMax is like PaginationToOffsetLimit but caps
// PageSize at maxPageSize (>= 1; falls back to 100 otherwise).
func PaginationToOffsetLimitWithMax(p Pagination, maxPageSize int32) (offset int32, limit int32) {
	if maxPageSize < 1 {
		maxPageSize = 100
	}
	page := p.Page
	if page < 1 {
		page = 1
	}
	pageSize := p.PageSize
	if pageSize < 1 {
		pageSize = 20
	}
	if int32(pageSize) > maxPageSize {
		pageSize = int(maxPageSize)
	}
	return int32((page - 1) * pageSize), int32(pageSize)
}
