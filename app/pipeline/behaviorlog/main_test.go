package main

import (
	"context"
	"testing"
	"time"

	"esx/pkg/event"
)

type cancelAwareAggregationStore struct {
	started chan<- struct{}
}

func (cancelAwareAggregationStore) Insert(context.Context, event.BehaviorEvent) error { return nil }

func (s cancelAwareAggregationStore) AggregateDaily(ctx context.Context, _, _ time.Time) (int64, error) {
	close(s.started)
	<-ctx.Done()
	return 0, ctx.Err()
}

func TestRunDailyAggregationCancelsInFlightWork(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		runDailyAggregation(ctx, 3600, 1, cancelAwareAggregationStore{started: started})
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("daily aggregation did not start")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("daily aggregation did not stop after context cancellation")
	}
}

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
