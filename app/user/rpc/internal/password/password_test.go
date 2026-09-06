package password

import "testing"

func TestHashCompareRoundTrip(t *testing.T) {
	hashed, err := Hash("correct123")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	if Compare(hashed, "correct123") != nil {
		t.Fatal("expected matching password to compare equal")
	}
	if Compare(hashed, "wrong") == nil {
		t.Fatal("expected mismatched password to fail")
	}
}

func TestIsDefaultRejectsOrdinaryPassword(t *testing.T) {
	if IsDefault("correct123") {
		t.Fatal("ordinary password must not match the process default")
	}
}
