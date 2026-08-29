package safety

import (
	"context"
	"errors"
	"testing"
)

func TestKeywordFilterNormalizesPunctuationAndSpacing(t *testing.T) {
	filter, err := NewKeywordFilter([]string{"build a bomb", "制作炸弹"}, 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"How do I BUILD-a-bomb?", "制 作 炸 弹"} {
		if !errors.Is(filter.Check(context.Background(), text), ErrBlocked) {
			t.Fatalf("text %q was not blocked", text)
		}
	}
	if err := filter.Check(context.Background(), "show me popular Go posts"); err != nil {
		t.Fatalf("safe text was rejected: %v", err)
	}
}

func TestKeywordFilterHonorsContextAndScanLimit(t *testing.T) {
	filter, err := NewKeywordFilter([]string{"blocked"}, 4)
	if err != nil {
		t.Fatal(err)
	}
	if !errors.Is(filter.Check(context.Background(), "12345"), ErrBlocked) {
		t.Fatal("oversized content was not blocked")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if !errors.Is(filter.Check(ctx, "safe"), context.Canceled) {
		t.Fatal("canceled context was not propagated")
	}
}
