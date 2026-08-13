package main

import (
	"testing"
	"time"
)

func TestAggregateWindowIncludesYesterday(t *testing.T) {
	now := time.Date(2026, 8, 13, 15, 30, 0, 0, time.UTC)
	from, to := aggregateWindow(now, 1)
	if !from.Equal(time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC)) || !to.Equal(time.Date(2026, 8, 13, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("window = [%s, %s), want [2026-08-12, 2026-08-13)", from, to)
	}
}

func TestAggregateWindowBackfill(t *testing.T) {
	now := time.Date(2026, 8, 13, 0, 0, 0, 0, time.UTC)
	from, to := aggregateWindow(now, 90)
	if !from.Equal(time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)) || !to.Equal(now) {
		t.Fatalf("window = [%s, %s), want [2026-05-15, 2026-08-13)", from, to)
	}
}

func TestAggregateWindowClampsBackfillDays(t *testing.T) {
	now := time.Date(2026, 8, 13, 0, 0, 0, 0, time.UTC)
	from, to := aggregateWindow(now, 0)
	if !from.Equal(time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC)) || !to.Equal(now) {
		t.Fatalf("window = [%s, %s), want [2026-08-12, 2026-08-13)", from, to)
	}
}
