package cursorx

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

func TestEncodeDecodeRoundTrip(t *testing.T) {
	in := Data{"c": 1700000000, "i": 9007199254740993, "l": 42}
	token, err := Encode(in)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}
	out, err := Decode(token)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}
	for k, want := range in {
		if got := out[k]; got != want {
			t.Fatalf("field %s: want %d got %d", k, want, got)
		}
	}
}

func TestDecodeRejects(t *testing.T) {
	cases := []struct {
		name  string
		token string
	}{
		{"空串", ""},
		{"非法base64", "!!!not-base64!!!"},
		{"非JSON", strings.Repeat("a", 20)},
		{"空负载", base64.RawURLEncoding.EncodeToString([]byte(`{}`))},
		{"非整数字段", base64.RawURLEncoding.EncodeToString([]byte(`{"i":"x"}`))},
		{"超长token", strings.Repeat("A", 600)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Decode(tc.token); !errors.Is(err, ErrInvalidCursor) {
				t.Fatalf("want ErrInvalidCursor, got %v", err)
			}
		})
	}
}

func TestEncodeEmptyFails(t *testing.T) {
	if _, err := Encode(Data{}); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("want ErrInvalidCursor, got %v", err)
	}
}
