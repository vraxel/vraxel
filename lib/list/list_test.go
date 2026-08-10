package list

import "testing"

func TestPaginationToOffsetLimit(t *testing.T) {
	tests := []struct {
		name       string
		p          Pagination
		wantOffset int32
		wantLimit  int32
	}{
		{"defaults", Pagination{}, 0, 20},
		{"page 2, size 10", Pagination{Page: 2, PageSize: 10}, 10, 10},
		{"page 0 normalized to 1", Pagination{Page: 0, PageSize: 50}, 0, 50},
		{"negative page normalized to 1", Pagination{Page: -3, PageSize: 50}, 0, 50},
		{"size 5000 capped at 100", Pagination{Page: 1, PageSize: 5000}, 0, 100},
		{"size 101 capped at 100", Pagination{Page: 1, PageSize: 101}, 0, 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o, l := PaginationToOffsetLimit(tt.p)
			if o != tt.wantOffset || l != tt.wantLimit {
				t.Fatalf("got (%d,%d), want (%d,%d)", o, l, tt.wantOffset, tt.wantLimit)
			}
		})
	}
}

func TestPaginationToOffsetLimitWithMax(t *testing.T) {
	tests := []struct {
		name       string
		p          Pagination
		maxPage    int32
		wantOffset int32
		wantLimit  int32
	}{
		{"5000 cap with PageSize 5000", Pagination{Page: 1, PageSize: 5000}, 5000, 0, 5000},
		{"5000 cap with PageSize 6000 still capped", Pagination{Page: 1, PageSize: 6000}, 5000, 0, 5000},
		{"5000 cap with PageSize 200 unchanged", Pagination{Page: 1, PageSize: 200}, 5000, 0, 200},
		{"zero max falls back to 100", Pagination{Page: 1, PageSize: 5000}, 0, 0, 100},
		{"negative max falls back to 100", Pagination{Page: 1, PageSize: 5000}, -1, 0, 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o, l := PaginationToOffsetLimitWithMax(tt.p, tt.maxPage)
			if o != tt.wantOffset || l != tt.wantLimit {
				t.Fatalf("got (%d,%d), want (%d,%d)", o, l, tt.wantOffset, tt.wantLimit)
			}
		})
	}
}

func TestQueryOffsetLimit(t *testing.T) {
	t.Run("zero MaxPageSize uses default 100 cap", func(t *testing.T) {
		q := Query{Pagination: Pagination{Page: 1, PageSize: 5000}}
		o, l := q.OffsetLimit()
		if o != 0 || l != 100 {
			t.Fatalf("got (%d,%d), want (0,100)", o, l)
		}
	})
	t.Run("non-zero MaxPageSize lifts cap", func(t *testing.T) {
		q := Query{Pagination: Pagination{Page: 1, PageSize: 5000}, MaxPageSize: 5000}
		o, l := q.OffsetLimit()
		if o != 0 || l != 5000 {
			t.Fatalf("got (%d,%d), want (0,5000)", o, l)
		}
	})
}
