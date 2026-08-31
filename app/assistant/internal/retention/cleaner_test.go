package retention

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeStore struct {
	messageBatches []int
	hitBatches     []int
	execBatches    []int
	messageCutoffs []int64
	watchCutoffs   []int64
	errMessages    error
}

func (f *fakeStore) PurgeExpiredMessages(_ context.Context, cutoffMs int64, _ int) (int, error) {
	f.messageCutoffs = append(f.messageCutoffs, cutoffMs)
	if f.errMessages != nil {
		return 0, f.errMessages
	}
	return popBatch(&f.messageBatches), nil
}

func (f *fakeStore) PurgeExpiredWatchHits(_ context.Context, cutoffMs int64, _ int) (int, error) {
	f.watchCutoffs = append(f.watchCutoffs, cutoffMs)
	return popBatch(&f.hitBatches), nil
}

func (f *fakeStore) PurgeExpiredWatchExecutions(_ context.Context, cutoffMs int64, _ int) (int, error) {
	f.watchCutoffs = append(f.watchCutoffs, cutoffMs)
	return popBatch(&f.execBatches), nil
}

func popBatch(batches *[]int) int {
	if len(*batches) == 0 {
		return 0
	}
	n := (*batches)[0]
	*batches = (*batches)[1:]
	return n
}

func TestCleanerUsesBoundedBatchesAndRetentionCutoffs(t *testing.T) {
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	store := &fakeStore{
		messageBatches: []int{2, 2, 1},
		hitBatches:     []int{2, 0},
		execBatches:    []int{1},
	}
	cleaner := New(store)
	cleaner.now = func() time.Time { return now }
	cleaner.batchSize = 2
	cleaner.maxBatches = 3

	result, err := cleaner.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result != (Result{Messages: 5, WatchHits: 2, WatchExecutions: 1}) {
		t.Fatalf("result=%+v", result)
	}
	if len(store.messageCutoffs) != 3 || store.messageCutoffs[0] != now.Add(-AssistantMessageRetention).UnixMilli() {
		t.Fatalf("message cutoffs=%v", store.messageCutoffs)
	}
	if len(store.watchCutoffs) != 3 {
		t.Fatalf("watch cutoffs=%v", store.watchCutoffs)
	}
	for _, cutoff := range store.watchCutoffs {
		if cutoff != now.Add(-WatchAuditRetention).UnixMilli() {
			t.Fatalf("watch cutoff=%d", cutoff)
		}
	}
}

func TestCleanerContinuesWatchCleanupAfterMessageFailure(t *testing.T) {
	wantErr := errors.New("message purge failed")
	store := &fakeStore{errMessages: wantErr, hitBatches: []int{1}, execBatches: []int{1}}
	cleaner := New(store)
	cleaner.batchSize = 10

	result, err := cleaner.RunOnce(context.Background())
	if !errors.Is(err, wantErr) {
		t.Fatalf("err=%v", err)
	}
	if result.WatchHits != 1 || result.WatchExecutions != 1 {
		t.Fatalf("result=%+v", result)
	}
}
