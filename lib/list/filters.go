package list

import (
	"strconv"
	"strings"
	"time"
)

// FilterStr extracts a string filter value from a Query.Filters map. An
// empty string is treated as "no filter" (returns nil), so callers can
// safely forward URL query params where an empty value means "skip".
// When key is "search", LIKE special characters are escaped automatically
// via EscapeLikePattern so the value can be used directly in ILIKE.
func FilterStr(filters map[string]any, key string) *string {
	if v, ok := filters[key]; ok {
		if s, ok := v.(string); ok && s != "" {
			if key == "search" {
				s = EscapeLikePattern(s)
			}
			return &s
		}
	}
	return nil
}

// FilterInt32 extracts an int32 filter value. Accepts int32, int64, and
// string (parsed with strconv.ParseInt, base 10).
func FilterInt32(filters map[string]any, key string) *int32 {
	if v, ok := filters[key]; ok {
		switch val := v.(type) {
		case int32:
			return &val
		case int64:
			i := int32(val)
			return &i
		case string:
			if i, err := strconv.ParseInt(val, 10, 32); err == nil {
				i32 := int32(i)
				return &i32
			}
		}
	}
	return nil
}

// FilterInt64 extracts an int64 filter value. Accepts int64 and string
// (parsed with strconv.ParseInt, base 10).
func FilterInt64(filters map[string]any, key string) *int64 {
	if v, ok := filters[key]; ok {
		switch val := v.(type) {
		case int64:
			return &val
		case string:
			if i, err := strconv.ParseInt(val, 10, 64); err == nil {
				return &i
			}
		}
	}
	return nil
}

// FilterInt64Slice extracts an []int64 filter value. Returns nil if the key
// is absent (meaning "no filter"). Used where the handler has already
// pre-computed a slice of IDs.
func FilterInt64Slice(filters map[string]any, key string) []int64 {
	if v, ok := filters[key]; ok {
		if ids, ok := v.([]int64); ok {
			return ids
		}
	}
	return nil
}

// FilterStrSlice extracts a []string filter value. Accepts a []string or a
// comma-separated string (URL query values are strings, so the comma form is
// how the framework passes through e.g. `?extra_names=a,b,c`). Empty values
// after trimming are dropped; if nothing remains, returns nil.
func FilterStrSlice(filters map[string]any, key string) []string {
	v, ok := filters[key]
	if !ok {
		return nil
	}
	switch val := v.(type) {
	case []string:
		return filterStrSliceTrim(val)
	case string:
		if val == "" {
			return nil
		}
		return filterStrSliceTrim(strings.Split(val, ","))
	}
	return nil
}

// filterStrSliceTrim trims every element, drops empties, and returns nil
// when nothing remains (so an all-blank input is treated as "no filter").
func filterStrSliceTrim(parts []string) []string {
	out := make([]string, 0, len(parts))
	for _, s := range parts {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// FilterBool extracts a bool filter value. Accepts bool and any string
// recognized by strconv.ParseBool ("1","t","T","TRUE","true","True",
// "0","f","F","FALSE","false","False"). Unrecognized strings return nil.
func FilterBool(filters map[string]any, key string) *bool {
	if v, ok := filters[key]; ok {
		switch val := v.(type) {
		case bool:
			return &val
		case string:
			if b, err := strconv.ParseBool(val); err == nil {
				return &b
			}
		}
	}
	return nil
}

// FilterTime extracts a time.Time filter value. Accepts strings in
// RFC3339 format.
func FilterTime(filters map[string]any, key string) *time.Time {
	if v, ok := filters[key]; ok {
		if s, ok := v.(string); ok {
			if t, err := time.Parse(time.RFC3339, s); err == nil {
				return &t
			}
		}
	}
	return nil
}
