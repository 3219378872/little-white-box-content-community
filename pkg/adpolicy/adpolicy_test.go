package adpolicy

import (
	"strings"
	"testing"
)

func TestLandingURLValid(t *testing.T) {
	cases := []struct {
		raw    string
		domain string
		ok     bool
	}{
		{"https://shop.example.com/sale?id=1", "shop.example.com", true},
		{"https://Shop.Example.COM:443/x", "shop.example.com", true},
		{"http://shop.example.com/", "", false},
		{"https://user:pass@shop.example.com/", "", false},
		{"https://user@shop.example.com/", "", false},
		{"https://127.0.0.1/", "", false},
		{"https://[::1]/", "", false},
		{"https://10.0.0.8/", "", false},
		{"https://localhost/", "", false},
		{"https://printer.local/", "", false},
		{"https://api.internal/", "", false},
		{"https://host.home.arpa/", "", false},
		{"https://intranet/", "", false},
		{"https://shop.example.com:8443/", "", false},
		{"https://bad_host.com/", "", false},
		{"javascript:alert(1)", "", false},
		{" https://shop.example.com/", "", false},
		{"https://shop.example.com/" + strings.Repeat("a", MaxLandingURLLength), "", false},
		{"", "", false},
	}
	for _, tc := range cases {
		domain, ok := LandingURLValid(tc.raw)
		if ok != tc.ok || domain != tc.domain {
			t.Errorf("LandingURLValid(%q) = (%q, %v), want (%q, %v)", tc.raw, domain, ok, tc.domain, tc.ok)
		}
	}
}

func TestAgeRestrictedIndustriesForbiddenEverywhere(t *testing.T) {
	for _, market := range Markets {
		for _, industry := range []string{IndustryAlcohol, IndustryGambling} {
			d := DispositionOf(market.Code, industry)
			if !d.Forbidden || d.PolicyCode != "INDUSTRY."+industry || !IsCode(d.PolicyCode) {
				t.Errorf("%s/%s disposition = %+v, want forbidden with policy code", market.Code, industry, d)
			}
		}
	}
}

func TestMatrixPerMarket(t *testing.T) {
	if d := DispositionOf("US", IndustryWeight); d.Forbidden || d.NeedsQualification || d.AutoPassAllowed {
		t.Errorf("US weight = %+v, want allowed without auto pass", d)
	}
	if d := DispositionOf("DE", IndustryWeight); !d.NeedsQualification {
		t.Errorf("DE weight = %+v, want qualification", d)
	}
	if d := DispositionOf("ID", IndustryWeight); !d.Forbidden {
		t.Errorf("ID weight = %+v, want forbidden", d)
	}
	if d := DispositionOf("DE", IndustryFinancial); !d.NeedsQualification || d.AutoPassAllowed {
		t.Errorf("DE financial = %+v", d)
	}
	if d := DispositionOf("ID", IndustryGeneral); !d.AutoPassAllowed {
		t.Errorf("ID general = %+v", d)
	}
}

func TestCodesAreUniqueAndKnown(t *testing.T) {
	seen := map[string]bool{}
	for _, code := range Codes {
		if seen[code.Code] {
			t.Fatalf("duplicate policy code %s", code.Code)
		}
		seen[code.Code] = true
	}
	if len(Codes) != 30 {
		t.Fatalf("SPEC-sponsored-ads defines 30 policy codes, got %d", len(Codes))
	}
	for _, code := range []string{CodeQualification, CodeLandingURL, CodeLandingDomain} {
		if !IsCode(code) {
			t.Errorf("%s is not a policy code", code)
		}
	}
	if LanguageOf("DE") != "de" || IsMarket("CN") {
		t.Error("unexpected demo market table")
	}
}
