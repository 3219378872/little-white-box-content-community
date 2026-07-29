package mqs

import (
	"testing"
	"time"
)

func TestNonNegativeEventLagSeconds(t *testing.T) {
	now := time.Unix(10, 0)
	if got := nonNegativeEventLagSeconds(now.Add(-2500*time.Millisecond).UnixMilli(), now); got != 2.5 {
		t.Fatalf("lag=%v want=2.5", got)
	}
	if got := nonNegativeEventLagSeconds(now.Add(time.Second).UnixMilli(), now); got != 0 {
		t.Fatalf("future lag=%v want=0", got)
	}
}
