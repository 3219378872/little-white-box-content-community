package retention

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeStore struct {
	messageBatches  []int
	evidenceBatches []int
	messageCutoffs  []int64
	evidenceCutoffs []int64
	errMessages     error
}

func (f *fakeStore) PurgeExpiredMessages(_ context.Context, cutoffMs int64, _ int) (int, error) {
	f.messageCutoffs = append(f.messageCutoffs, cutoffMs)
	if f.errMessages != nil {
		return 0, f.errMessages
	}
	return popBatch(&f.messageBatches), nil
}

func (f *fakeStore) PurgeExpiredSourceEvidence(_ context.Context, cutoffMs int64, _ int) (int, error) {
	f.evidenceCutoffs = append(f.evidenceCutoffs, cutoffMs)
	return popBatch(&f.evidenceBatches), nil
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
		messageBatches:  []int{2, 2, 1},
		evidenceBatches: []int{2, 0},
	}
	cleaner := New(store)
	cleaner.now = func() time.Time { return now }
	cleaner.batchSize = 2
	cleaner.maxBatches = 3

	result, err := cleaner.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result != (Result{Messages: 5, SourceEvidence: 2}) {
		t.Fatalf("result=%+v", result)
	}
	cutoff := now.Add(-AssistantMessageRetention).UnixMilli()
	if len(store.messageCutoffs) != 3 || store.messageCutoffs[0] != cutoff {
		t.Fatalf("message cutoffs=%v", store.messageCutoffs)
	}
	if len(store.evidenceCutoffs) != 2 || store.evidenceCutoffs[0] != cutoff {
		t.Fatalf("evidence cutoffs=%v", store.evidenceCutoffs)
	}
}

func TestCleanerContinuesEvidenceCleanupAfterMessageFailure(t *testing.T) {
	wantErr := errors.New("message purge failed")
	store := &fakeStore{errMessages: wantErr, evidenceBatches: []int{1}}
	cleaner := New(store)
	cleaner.batchSize = 10

	result, err := cleaner.RunOnce(context.Background())
	if !errors.Is(err, wantErr) {
		t.Fatalf("err=%v", err)
	}
	if result.SourceEvidence != 1 {
		t.Fatalf("result=%+v", result)
	}
}
