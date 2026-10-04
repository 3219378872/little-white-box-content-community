package outboxx

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeStore struct {
	mu      sync.Mutex
	records []Record
	// batches, when set, are returned one per Claim before falling back to records.
	batches      [][]Record
	purgeResults []int64
	purgeCutoffs []time.Time
	claimErr     error
	markSentErr  error
	claimCalls   int
	backlogCalls int
	retries      []int64
	sent         []int64
}

func (s *fakeStore) Claim(context.Context, string, int, time.Time, time.Duration) ([]Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.claimCalls++
	if len(s.batches) > 0 {
		batch := s.batches[0]
		s.batches = s.batches[1:]
		return batch, s.claimErr
	}
	return append([]Record(nil), s.records...), s.claimErr
}

func (s *fakeStore) Purge(_ context.Context, createdBefore time.Time, _ int) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.purgeCutoffs = append(s.purgeCutoffs, createdBefore)
	if len(s.purgeResults) == 0 {
		return 0, nil
	}
	deleted := s.purgeResults[0]
	s.purgeResults = s.purgeResults[1:]
	return deleted, nil
}

// wakingStore lets tests drive the commit wakeup path without a database.
type wakingStore struct {
	*fakeStore
	wake chan struct{}
}

func (s *wakingStore) Wakeups() <-chan struct{} { return s.wake }

func (s *fakeStore) MarkSent(_ context.Context, id int64, _ string, _ time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent = append(s.sent, id)
	return s.markSentErr
}

func (s *fakeStore) MarkRetry(_ context.Context, id int64, _ string, _, _ int, _ time.Time, _ error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.retries = append(s.retries, id)
	return nil
}

func (s *fakeStore) Backlog(context.Context) (Backlog, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.backlogCalls++
	return Backlog{}, nil
}

func (s *fakeStore) backlogCallCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.backlogCalls
}

func (s *fakeStore) claimCallCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.claimCalls
}

func testRelay(t *testing.T, store Store, publisher Publisher) *Relay {
	t.Helper()
	relay, err := NewRelay(store, publisher, RelayConfig{
		Owner: "relay-test", BatchSize: 10, PollInterval: time.Millisecond,
		Lease: time.Second, BaseBackoff: time.Second, MaxBackoff: time.Minute, MaxAttempts: 8,
	})
	require.NoError(t, err)
	return relay
}

func TestEventValidate(t *testing.T) {
	valid := Event{ID: 1, Topic: "events", Key: "1", Payload: []byte("{}")}
	require.NoError(t, valid.Validate())

	tests := []Event{
		{Topic: "events", Key: "1", Payload: []byte("{}")},
		{ID: 1, Key: "1", Payload: []byte("{}")},
		{ID: 1, Topic: "events", Payload: []byte("{}")},
		{ID: 1, Topic: "events", Key: "1"},
	}
	for _, event := range tests {
		assert.Error(t, event.Validate())
	}
}

func TestRelayProcessBatchMarksBrokerAcknowledgementsSent(t *testing.T) {
	store := &fakeStore{records: []Record{{ID: 11}, {ID: 12}}}
	relay := testRelay(t, store, PublisherFunc(func(context.Context, Record) error { return nil }))

	processed, err := relay.ProcessBatch(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 2, processed)
	assert.Equal(t, []int64{11, 12}, store.sent)
	assert.Empty(t, store.retries)
}

func TestRelayProcessBatchSchedulesPublishFailureForRetry(t *testing.T) {
	store := &fakeStore{records: []Record{{ID: 21, Attempts: 2}}}
	relay := testRelay(t, store, PublisherFunc(func(context.Context, Record) error {
		return errors.New("broker unavailable")
	}))

	processed, err := relay.ProcessBatch(context.Background())

	assert.Equal(t, 1, processed)
	assert.ErrorContains(t, err, "broker unavailable")
	assert.Equal(t, []int64{21}, store.retries)
	assert.Empty(t, store.sent)
}

func TestRelayProcessBatchKeepsSentEventLeasedWhenMarkFails(t *testing.T) {
	store := &fakeStore{
		records:     []Record{{ID: 31}},
		markSentErr: errors.New("database unavailable"),
	}
	relay := testRelay(t, store, PublisherFunc(func(context.Context, Record) error { return nil }))

	_, err := relay.ProcessBatch(context.Background())

	assert.ErrorContains(t, err, "mark sent")
	assert.Empty(t, store.retries)
}

func TestRetryBackoffIsBounded(t *testing.T) {
	assert.Equal(t, time.Second, retryBackoff(1, time.Second, time.Minute))
	assert.Equal(t, 2*time.Second, retryBackoff(2, time.Second, time.Minute))
	assert.Equal(t, time.Minute, retryBackoff(100, time.Second, time.Minute))
}

func TestNewRelayRejectsInvalidConfiguration(t *testing.T) {
	_, err := NewRelay(&fakeStore{}, PublisherFunc(func(context.Context, Record) error { return nil }), RelayConfig{})
	assert.Error(t, err)
}

func TestRelayHandleStopCancelsAndJoinsInFlightPublish(t *testing.T) {
	store := &fakeStore{records: []Record{{ID: 41}}}
	publishStarted := make(chan struct{})
	cancelObserved := make(chan struct{})
	releasePublish := make(chan struct{})
	relay := testRelay(t, store, PublisherFunc(func(ctx context.Context, _ Record) error {
		close(publishStarted)
		<-ctx.Done()
		close(cancelObserved)
		<-releasePublish
		return ctx.Err()
	}))
	handle := StartRelay(context.Background(), relay)

	select {
	case <-publishStarted:
	case <-time.After(time.Second):
		t.Fatal("relay did not start publishing")
	}
	stopped := make(chan error, 1)
	go func() {
		stopped <- handle.Stop()
	}()

	select {
	case <-cancelObserved:
	case <-time.After(time.Second):
		t.Fatal("relay publish did not observe cancellation")
	}
	select {
	case err := <-stopped:
		t.Fatalf("Stop returned before in-flight publish completed: %v", err)
	default:
	}

	close(releasePublish)
	select {
	case err := <-stopped:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("Stop did not return after in-flight publish completed")
	}
	assert.Zero(t, store.backlogCallCount(), "canceled relay should not query backlog during shutdown")
	require.NoError(t, handle.Stop(), "Stop should be idempotent")
}

func TestStartRelayWithNilRelayIsNoOp(t *testing.T) {
	require.NoError(t, StartRelay(context.Background(), nil).Stop())
}

func TestRelayRunWithCanceledContextDoesNotAccessStore(t *testing.T) {
	store := &fakeStore{}
	relay := testRelay(t, store, PublisherFunc(func(context.Context, Record) error { return nil }))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := relay.Run(ctx)

	require.ErrorIs(t, err, context.Canceled)
	assert.Zero(t, store.claimCallCount())
	assert.Zero(t, store.backlogCallCount())
}

func TestConfigRelayConfigAppliesOperationalDefaults(t *testing.T) {
	config := (Config{}).RelayConfig("content")

	assert.Equal(t, "content", config.Service)
	assert.Contains(t, config.Owner, "content-")
	assert.Equal(t, 100, config.BatchSize)
	assert.Equal(t, 200*time.Millisecond, config.PollInterval)
	assert.Equal(t, 30*time.Second, config.Lease)
	assert.Equal(t, 20, config.MaxAttempts)
	assert.Equal(t, 7*24*time.Hour, config.Retention)
	assert.Equal(t, time.Minute, config.PurgeInterval)
	assert.Equal(t, 500, config.PurgeBatch)
}

func TestNewRelayRejectsInvalidPurgeConfiguration(t *testing.T) {
	base := RelayConfig{
		Owner: "relay-test", BatchSize: 10, PollInterval: time.Millisecond,
		Lease: time.Second, BaseBackoff: time.Second, MaxBackoff: time.Minute, MaxAttempts: 8,
	}
	publisher := PublisherFunc(func(context.Context, Record) error { return nil })

	negative := base
	negative.Retention = -time.Second
	_, err := NewRelay(&fakeStore{}, publisher, negative)
	assert.ErrorContains(t, err, "retention")

	missingBatch := base
	missingBatch.Retention, missingBatch.PurgeInterval = time.Hour, time.Minute
	_, err = NewRelay(&fakeStore{}, publisher, missingBatch)
	assert.ErrorContains(t, err, "purge")
}

func TestRelayDrainContinuesWhileBatchesAreFullAndDelivered(t *testing.T) {
	full := make([]Record, 10)
	for index := range full {
		full[index] = Record{ID: int64(index + 1)}
	}
	store := &fakeStore{batches: [][]Record{full, full, {{ID: 99}}, full}}
	relay := testRelay(t, store, PublisherFunc(func(context.Context, Record) error { return nil }))

	relay.drain(context.Background())

	assert.Equal(t, 3, store.claimCallCount(), "drain stops at the first partial batch")
	assert.Len(t, store.sent, 21)
}

func TestRelayDrainStopsOnPublishFailure(t *testing.T) {
	full := make([]Record, 10)
	for index := range full {
		full[index] = Record{ID: int64(index + 1)}
	}
	store := &fakeStore{batches: [][]Record{full, full}}
	relay := testRelay(t, store, PublisherFunc(func(context.Context, Record) error {
		return errors.New("broker unavailable")
	}))

	relay.drain(context.Background())

	assert.Equal(t, 1, store.claimCallCount(), "a failing broker must not spin through the backlog")
	assert.Len(t, store.retries, 10)
}

func TestRelayRunProcessesImmediatelyOnCommitWakeup(t *testing.T) {
	store := &wakingStore{fakeStore: &fakeStore{}, wake: make(chan struct{}, 1)}
	relay, err := NewRelay(store, PublisherFunc(func(context.Context, Record) error { return nil }), RelayConfig{
		Owner: "relay-test", BatchSize: 10, PollInterval: time.Hour,
		Lease: time.Second, BaseBackoff: time.Second, MaxBackoff: time.Minute, MaxAttempts: 8,
	})
	require.NoError(t, err)
	handle := StartRelay(context.Background(), relay)
	t.Cleanup(func() { require.NoError(t, handle.Stop()) })
	require.Eventually(t, func() bool { return store.claimCallCount() == 1 }, time.Second, time.Millisecond)

	store.wake <- struct{}{}

	require.Eventually(t, func() bool { return store.claimCallCount() == 2 }, time.Second, time.Millisecond,
		"wakeup must trigger a claim without waiting for the hour-long poll")
}

func TestRelayPurgeRepeatsFullBatchesAndUsesRetentionCutoff(t *testing.T) {
	store := &fakeStore{purgeResults: []int64{5, 5, 2, 5}}
	relay := testRelay(t, store, PublisherFunc(func(context.Context, Record) error { return nil }))
	relay.config.Retention, relay.config.PurgeInterval, relay.config.PurgeBatch = time.Hour, time.Minute, 5
	now := time.UnixMilli(10_000_000)
	relay.now = func() time.Time { return now }

	relay.purge(context.Background())

	require.Len(t, store.purgeCutoffs, 3, "purge stops at the first partial batch")
	for _, cutoff := range store.purgeCutoffs {
		assert.Equal(t, now.Add(-time.Hour), cutoff)
	}
}

func TestRelayPurgeIsBoundedPerTick(t *testing.T) {
	results := make([]int64, maxPurgeBatchesPerTick+5)
	for index := range results {
		results[index] = 5
	}
	store := &fakeStore{purgeResults: results}
	relay := testRelay(t, store, PublisherFunc(func(context.Context, Record) error { return nil }))
	relay.config.Retention, relay.config.PurgeInterval, relay.config.PurgeBatch = time.Hour, time.Minute, 5

	relay.purge(context.Background())

	assert.Len(t, store.purgeCutoffs, maxPurgeBatchesPerTick)
}

func TestBacklogAgeSecondsHandlesEmptyAndFutureBacklogs(t *testing.T) {
	now := time.UnixMilli(10_000)

	assert.Zero(t, backlogAgeSeconds(Backlog{}, now))
	assert.Zero(t, backlogAgeSeconds(Backlog{Count: 1, OldestCreatedAt: 11_000}, now))
	assert.Equal(t, 3.0, backlogAgeSeconds(Backlog{Count: 2, OldestCreatedAt: 7_000}, now))
}
