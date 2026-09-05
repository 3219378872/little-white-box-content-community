package main

import (
	"context"
	"reflect"
	"testing"

	"github.com/zeromicro/go-zero/core/logx"
)

func TestShutdownContentCleanupStopsConsumersBeforeDatabase(t *testing.T) {
	var calls []string
	shutdown := func(name string) func() error {
		return func() error {
			calls = append(calls, name)
			return nil
		}
	}

	shutdownContentCleanup(
		logx.WithContext(context.Background()),
		shutdown("count-sync"),
		shutdown("cleanup"),
		shutdown("database"),
	)

	want := []string{"count-sync", "cleanup", "database"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("shutdown order = %v, want %v", calls, want)
	}
}
