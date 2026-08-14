package rest

import (
	"errors"
	"net/url"
	"strconv"
)

var (
	// ErrInvalidPage indicates an invalid page number.
	ErrInvalidPage = errors.New("page must be greater than 0")
	// ErrInvalidPageSize indicates an invalid page size.
	ErrInvalidPageSize = errors.New("page size must be between 1 and 100")
)

// SortOrder defines sort direction.
type SortOrder string

const (
	SortOrderAsc  SortOrder = "asc"
	SortOrderDesc SortOrder = "desc"
)

// ListOptions holds options for listing resources.
type ListOptions struct {
	PathParams map[string]string // path parameters
	Filters    map[string]string // filter conditions
	Pagination Pagination        // pagination parameters
	SortBy     string            // sort field
	SortOrder  SortOrder         // sort direction (asc/desc)
}

// Pagination holds pagination parameters.
type Pagination struct {
	Page     int // page number, starting from 1
	PageSize int // items per page
}

// Validate checks that pagination parameters are within valid ranges.
func (p Pagination) Validate() error {
	if p.Page < 1 {
		return ErrInvalidPage
	}
	if p.PageSize < 1 || p.PageSize > 100 {
		return ErrInvalidPageSize
	}
	return nil
}

// ReservedQueryParams are query parameter names that should not be used as filters.
var ReservedQueryParams = map[string]bool{
	"page":       true,
	"page_size":  true,
	"sort_by":    true,
	"sort_order": true,
}

// ParseListOptions parses ListOptions from URL query parameters.
func ParseListOptions(query url.Values) *ListOptions {
	options := &ListOptions{
		Filters: make(map[string]string),
		Pagination: Pagination{
			Page:     1,
			PageSize: 20,
		},
	}

	// Parse filter conditions (exclude reserved parameters)
	for key, values := range query {
		if len(values) > 0 && !ReservedQueryParams[key] {
			options.Filters[key] = values[0]
		}
	}

	// Parse pagination
	if page := query.Get("page"); page != "" {
		if p, err := strconv.Atoi(page); err == nil && p > 0 {
			options.Pagination.Page = p
		}
	}
	if pageSize := query.Get("page_size"); pageSize != "" {
		if ps, err := strconv.Atoi(pageSize); err == nil && ps > 0 {
			options.Pagination.PageSize = ps
		}
	}

	// Parse sorting
	options.SortBy = query.Get("sort_by")
	if sortOrder := query.Get("sort_order"); sortOrder != "" {
		options.SortOrder = SortOrder(sortOrder)
	}

	return options
}

// ParseID parses a string ID (from path params) into int64.
func ParseID(s string) (int64, error) {
	return strconv.ParseInt(s, 10, 64)
}
