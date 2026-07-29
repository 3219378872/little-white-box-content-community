package mqs

import (
	"testing"
	"time"
)

func TestSearchEventLagSeconds(t *testing.T) {
	now := time.Unix(10, 0)
	if got := searchEventLagSeconds(now.Add(-time.Second).UnixMilli(), now); got != 1 {
		t.Fatalf("lag=%v want=1", got)
	}
	if got := searchEventLagSeconds(now.Add(time.Second).UnixMilli(), now); got != 0 {
		t.Fatalf("future lag=%v want=0", got)
	}
}
