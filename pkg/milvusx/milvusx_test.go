package milvusx

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestIsStartupError(t *testing.T) {
	cases := map[string]bool{
		"proxy not ready":                    true,
		"rpc error: Service Unavailable":     true,
		"collection not found":               false,
		"invalid dimension for vector field": false,
	}
	for message, want := range cases {
		if got := IsStartupError(errors.New(message)); got != want {
			t.Errorf("IsStartupError(%q) = %v, want %v", message, got, want)
		}
	}
	if IsStartupError(nil) {
		t.Error("nil error must not be a startup error")
	}
}

func TestRetryStartupRetriesOnlyStartupErrors(t *testing.T) {
	attempts := 0
	err := RetryStartup(context.Background(), time.Millisecond, func() error {
		attempts++
		if attempts < 3 {
			return errors.New("not ready")
		}
		return nil
	})
	if err != nil || attempts != 3 {
		t.Fatalf("got err=%v after %d attempts, want success after 3", err, attempts)
	}

	permanent := errors.New("collection not found")
	attempts = 0
	err = RetryStartup(context.Background(), time.Millisecond, func() error {
		attempts++
		return permanent
	})
	if !errors.Is(err, permanent) || attempts != 1 {
		t.Fatalf("got err=%v after %d attempts, want the permanent error at once", err, attempts)
	}
}

func TestRetryStartupStopsWithLastErrorWhenContextEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	startup := errors.New("service unavailable")
	if err := RetryStartup(ctx, time.Hour, func() error { return startup }); !errors.Is(err, startup) {
		t.Fatalf("got %v, want the last startup error", err)
	}
}
