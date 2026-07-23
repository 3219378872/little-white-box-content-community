package validator

import (
	"strings"
	"testing"

	"errx"
)

func TestValidateUserName_Boundaries(t *testing.T) {
	tests := []struct {
		name  string
		value string
		valid bool
	}{
		{name: "five runes below minimum", value: "12345"},
		{name: "six runes at minimum", value: "123456", valid: true},
		{name: "fifty runes at maximum", value: strings.Repeat("a", 50), valid: true},
		{name: "fifty one runes above maximum", value: strings.Repeat("a", 51)},
		{name: "unicode counted as runes", value: "小白盒用户甲", valid: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateUserName(tt.value)
			if tt.valid && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tt.valid && !errx.Is(err, errx.ParamError) {
				t.Fatalf("got %v, want ParamError", err)
			}
		})
	}
}

func TestCheckPasswordStrength_EquivalenceAndBoundaries(t *testing.T) {
	tests := []struct {
		name  string
		value string
		valid bool
	}{
		{name: "seven bytes below minimum", value: "Aa12345"},
		{name: "eight bytes at minimum", value: "Aa123456", valid: true},
		{name: "missing upper class", value: "aa123456"},
		{name: "missing lower class", value: "AA123456"},
		{name: "missing digit class", value: "AaBCdefg"},
		{name: "sixty four bytes at maximum", value: "Aa1" + strings.Repeat("x", 61), valid: true},
		{name: "sixty five bytes above maximum", value: "Aa1" + strings.Repeat("x", 62)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid, err := CheckPasswordStrength(tt.value)
			if valid != tt.valid {
				t.Fatalf("valid=%v want=%v", valid, tt.valid)
			}
			if tt.valid && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tt.valid && !errx.Is(err, errx.ParamError) {
				t.Fatalf("got %v, want ParamError", err)
			}
		})
	}
}

func TestValidatePhone_EquivalenceClasses(t *testing.T) {
	tests := []struct {
		name  string
		value string
		valid bool
	}{
		{name: "valid mobile", value: "13800138000", valid: true},
		{name: "empty"},
		{name: "too short", value: "1380013800"},
		{name: "too long", value: "138001380000"},
		{name: "invalid second digit", value: "12800138000"},
		{name: "non digit", value: "1380013800x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePhone(tt.value)
			if (err == nil) != tt.valid {
				t.Fatalf("error=%v valid=%v", err, tt.valid)
			}
		})
	}
}

func FuzzValidatorsNeverPanic(f *testing.F) {
	for _, seed := range []string{"", "13800138000", "Aa123456", strings.Repeat("x", 1000), "\x00"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, value string) {
		_ = ValidatePhone(value)
		_, _ = CheckPasswordStrength(value)
		_ = ValidateUserName(value)
	})
}
