package logic

import "testing"

func TestNormalizePage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		page         int
		pageSize     int
		wantPage     int
		wantPageSize int
	}{
		{name: "valid page stays", page: 2, pageSize: 10, wantPage: 2, wantPageSize: 10},
		{name: "zero page becomes one", page: 0, pageSize: 10, wantPage: 1, wantPageSize: 10},
		{name: "negative page becomes one", page: -3, pageSize: 10, wantPage: 1, wantPageSize: 10},
		{name: "zero page size becomes default", page: 1, pageSize: 0, wantPage: 1, wantPageSize: defaultPageSize},
		{name: "negative page size becomes default", page: 1, pageSize: -1, wantPage: 1, wantPageSize: defaultPageSize},
		{name: "oversized page size becomes default", page: 1, pageSize: maxPageSize + 1, wantPage: 1, wantPageSize: defaultPageSize},
		{name: "max page size stays", page: 1, pageSize: maxPageSize, wantPage: 1, wantPageSize: maxPageSize},
		{name: "both defaults applied", page: 0, pageSize: 999, wantPage: 1, wantPageSize: defaultPageSize},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotPage, gotPageSize := normalizePage(tt.page, tt.pageSize)
			if gotPage != tt.wantPage || gotPageSize != tt.wantPageSize {
				t.Fatalf("normalizePage(%d, %d) = (%d, %d), want (%d, %d)",
					tt.page, tt.pageSize, gotPage, gotPageSize, tt.wantPage, tt.wantPageSize)
			}
		})
	}
}
