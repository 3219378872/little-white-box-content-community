package main

import (
	"context"
	"testing"
	"time"

	"esx/pkg/event"
)

type blockingCleanupStore struct{}

func (blockingCleanupStore) Record(context.Context, event.BehaviorEvent) error { return nil }

func (blockingCleanupStore) PurgeOptedOutFeatures(context.Context) (int, error) {
	return 0, nil
}

func TestRunOptOutCleanupStopsWhenContextIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		runOptOutCleanup(ctx, 3600, blockingCleanupStore{})
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("opt-out cleanup did not stop after context cancellation")
	}
}

type cancelAwareCleanupStore struct {
	started chan<- struct{}
}

func (cancelAwareCleanupStore) Record(context.Context, event.BehaviorEvent) error { return nil }

func (s cancelAwareCleanupStore) PurgeOptedOutFeatures(ctx context.Context) (int, error) {
	close(s.started)
	<-ctx.Done()
	return 0, ctx.Err()
}

func TestRunOptOutCleanupCancelsInFlightWork(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		runOptOutCleanup(ctx, 1, cancelAwareCleanupStore{started: started})
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("opt-out cleanup did not start")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("in-flight opt-out cleanup did not stop after context cancellation")
	}
}
