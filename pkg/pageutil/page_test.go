package pageutil

import "testing"

func TestClampPage(t *testing.T) {
	tests := []struct {
		in   int32
		want int32
	}{
		{in: 0, want: 1},
		{in: -5, want: 1},
		{in: 1, want: 1},
		{in: 42, want: 42},
	}
	for _, tt := range tests {
		if got := ClampPage(tt.in); got != tt.want {
			t.Fatalf("ClampPage(%d) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestClampPageSize(t *testing.T) {
	tests := []struct {
		in   int32
		want int32
	}{
		{in: 0, want: DefaultPageSize},
		{in: -1, want: DefaultPageSize},
		{in: 20, want: 20},
		{in: ContentMaxPageSize, want: ContentMaxPageSize},
		{in: ContentMaxPageSize + 1, want: ContentMaxPageSize},
		{in: 9999, want: ContentMaxPageSize},
	}
	for _, tt := range tests {
		if got := ClampPageSize(tt.in); got != tt.want {
			t.Fatalf("ClampPageSize(%d) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestClampPageSizeTo(t *testing.T) {
	tests := []struct {
		pageSize, def, max, want int32
	}{
		{pageSize: 0, def: 20, max: 100, want: 20},
		{pageSize: -1, def: 20, max: 100, want: 20},
		{pageSize: 20, def: 20, max: 100, want: 20},
		{pageSize: 100, def: 20, max: 100, want: 100},
		{pageSize: 101, def: 20, max: 100, want: 100},
		{pageSize: 999, def: 20, max: 100, want: 100},
	}
	for _, tt := range tests {
		if got := ClampPageSizeTo(tt.pageSize, tt.def, tt.max); got != tt.want {
			t.Fatalf("ClampPageSizeTo(%d,%d,%d) = %d, want %d", tt.pageSize, tt.def, tt.max, got, tt.want)
		}
	}
}
