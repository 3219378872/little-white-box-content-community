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
		{name: "oversized page size becomes max", page: 1, pageSize: maxPageSize + 1, wantPage: 1, wantPageSize: maxPageSize},
		{name: "max page size stays", page: 1, pageSize: maxPageSize, wantPage: 1, wantPageSize: maxPageSize},
		{name: "invalid page and oversized size", page: 0, pageSize: 999, wantPage: 1, wantPageSize: maxPageSize},
	}

	for _, tt := range tests {
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

func TestNormalizePageSize(t *testing.T) {
	t.Parallel()
	if got := normalizePageSize(0); got != defaultPageSize {
		t.Fatalf("zero = %d, want %d", got, defaultPageSize)
	}
	if got := normalizePageSize(maxPageSize + 1); got != maxPageSize {
		t.Fatalf("oversized = %d, want %d", got, maxPageSize)
	}
	if got := normalizePageSize(10); got != 10 {
		t.Fatalf("valid = %d, want 10", got)
	}
}
