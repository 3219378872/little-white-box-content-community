package behaviorlog

import (
	"testing"
	"time"
)

func TestEventLagSeconds(t *testing.T) {
	now := time.Unix(100, 0)
	if got := eventLagSeconds(now.Add(-1500*time.Millisecond).UnixMilli(), now); got != 1.5 {
		t.Fatalf("eventLagSeconds()=%v want=1.5", got)
	}
	if got := eventLagSeconds(now.Add(time.Second).UnixMilli(), now); got != 0 {
		t.Fatalf("future event lag=%v want=0", got)
	}
}
