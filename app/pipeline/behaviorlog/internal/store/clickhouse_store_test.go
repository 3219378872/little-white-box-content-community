package store

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func TestAnonymizeIPHashesFullIP(t *testing.T) {
	hashed := anonymizeIP("203.0.113.7")
	if hashed == "" || hashed == "203.0.113.7" || len(hashed) != 64 {
		t.Fatalf("anonymizeIP(%q) = %q, want a 64-char SHA-256 hex digest", "203.0.113.7", hashed)
	}
	digest := sha256.Sum256([]byte("203.0.113.7"))
	if hashed != hex.EncodeToString(digest[:]) {
		t.Fatalf("anonymizeIP produced a non-deterministic hash: %q", hashed)
	}
}

func TestAnonymizeIPEmptyStaysEmpty(t *testing.T) {
	if got := anonymizeIP(""); got != "" {
		t.Fatalf("anonymizeIP(\"\") = %q, want empty", got)
	}
	if got := anonymizeIP("  "); got != "" {
		t.Fatalf("anonymizeIP(whitespace) = %q, want empty", got)
	}
}

func TestAnonymizeIPNeverContainsOriginal(t *testing.T) {
	ip := "198.51.100.42"
	hashed := anonymizeIP(ip)
	if strings.Contains(hashed, ip) {
		t.Fatalf("anonymized value still contains the original IP: %q", hashed)
	}
}
