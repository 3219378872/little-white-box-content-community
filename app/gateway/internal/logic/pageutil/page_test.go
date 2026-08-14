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
		{in: 0, want: 20},
		{in: -1, want: 20},
		{in: 20, want: 20},
		{in: 50, want: 50},
		{in: 51, want: 50},
		{in: 9999, want: 50},
	}
	for _, tt := range tests {
		if got := ClampPageSize(tt.in); got != tt.want {
			t.Fatalf("ClampPageSize(%d) = %d, want %d", tt.in, got, tt.want)
		}
	}
}
