package outboxx

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"sync"
	"time"

	"esx/pkg/logging"
)

// Publisher sends one outbox record to the broker.
type Publisher interface {
	Publish(ctx context.Context, record Record) error
}

// PublisherFunc adapts a function to Publisher.
type PublisherFunc func(ctx context.Context, record Record) error

// Publish calls f.
func (f PublisherFunc) Publish(ctx context.Context, record Record) error {
	return f(ctx, record)
}

// RelayConfig controls batching, polling, leases, retry backoff and retention.
type RelayConfig struct {
	Service      string
	Owner        string
	BatchSize    int
	PollInterval time.Duration
	Lease        time.Duration
	BaseBackoff  time.Duration
	MaxBackoff   time.Duration
	MaxAttempts  int
	// Retention keeps sent events this long before purging; zero disables purge.
	Retention     time.Duration
	PurgeInterval time.Duration
	PurgeBatch    int
}

// Config uses millisecond integers so project YAML/env loading stays
// consistent across every business service.
type Config struct {
	BatchSize       int
	PollIntervalMs  int
	LeaseMs         int
	BaseBackoffMs   int
	MaxBackoffMs    int
	MaxAttempts     int
	RetentionMs     int
	PurgeIntervalMs int
	PurgeBatchSize  int
}

// RelayConfig fills defaults for unset fields and converts milliseconds to durations.
func (c Config) RelayConfig(service string) RelayConfig {
	if c.BatchSize <= 0 {
		c.BatchSize = 100
	}
	if c.PollIntervalMs <= 0 {
		c.PollIntervalMs = 200
	}
	if c.LeaseMs <= 0 {
		c.LeaseMs = 30_000
	}
	if c.BaseBackoffMs <= 0 {
		c.BaseBackoffMs = 1_000
	}
	if c.MaxBackoffMs <= 0 {
		c.MaxBackoffMs = 60_000
	}
	if c.MaxAttempts <= 0 {
		c.MaxAttempts = 20
	}
	if c.RetentionMs <= 0 {
		c.RetentionMs = 7 * 24 * 3600 * 1000
	}
	if c.PurgeIntervalMs <= 0 {
		c.PurgeIntervalMs = 60_000
	}
	if c.PurgeBatchSize <= 0 {
		c.PurgeBatchSize = 500
	}
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		hostname = "unknown-host"
	}
	return RelayConfig{
		Service:       service,
		Owner:         fmt.Sprintf("%s-%s-%d", service, hostname, os.Getpid()),
		BatchSize:     c.BatchSize,
		PollInterval:  time.Duration(c.PollIntervalMs) * time.Millisecond,
		Lease:         time.Duration(c.LeaseMs) * time.Millisecond,
		BaseBackoff:   time.Duration(c.BaseBackoffMs) * time.Millisecond,
		MaxBackoff:    time.Duration(c.MaxBackoffMs) * time.Millisecond,
		MaxAttempts:   c.MaxAttempts,
		Retention:     time.Duration(c.RetentionMs) * time.Millisecond,
		PurgeInterval: time.Duration(c.PurgeIntervalMs) * time.Millisecond,
		PurgeBatch:    c.PurgeBatchSize,
	}
}

// validate rejects configurations that would stall or spin the relay.
func (c RelayConfig) validate() error {
	if c.Owner == "" {
		return fmt.Errorf("outboxx: relay owner is required")
	}
	if c.BatchSize <= 0 {
		return fmt.Errorf("outboxx: batch size must be positive")
	}
	if c.PollInterval <= 0 || c.Lease <= 0 || c.BaseBackoff <= 0 || c.MaxBackoff <= 0 {
		return fmt.Errorf("outboxx: relay durations must be positive")
	}
	if c.MaxBackoff < c.BaseBackoff {
		return fmt.Errorf("outboxx: max backoff must not be smaller than base backoff")
	}
	if c.MaxAttempts <= 0 {
		return fmt.Errorf("outboxx: max attempts must be positive")
	}
	if c.Retention < 0 {
		return fmt.Errorf("outboxx: retention must not be negative")
	}
	if c.Retention > 0 && (c.PurgeInterval <= 0 || c.PurgeBatch <= 0) {
		return fmt.Errorf("outboxx: purge interval and batch must be positive when retention is set")
	}
	return nil
}

// Relay moves committed outbox events to the broker with at-least-once delivery.
type Relay struct {
	store     Store
	publisher Publisher
	config    RelayConfig
	now       func() time.Time
	// wake fires when a same-process enqueue commits; nil means poll only.
	wake <-chan struct{}
}

// RelayHandle owns one background relay run. Stop cancels the run and waits
// for any in-flight store or publisher call to return before releasing it.
type RelayHandle struct {
	cancel context.CancelFunc
	done   chan error
	once   sync.Once
	err    error
}

// NewRelay validates the configuration and subscribes to store wakeups when available.
func NewRelay(store Store, publisher Publisher, config RelayConfig) (*Relay, error) {
	if store == nil {
		return nil, fmt.Errorf("outboxx: store is required")
	}
	if publisher == nil {
		return nil, fmt.Errorf("outboxx: publisher is required")
	}
	if err := config.validate(); err != nil {
		return nil, err
	}
	relay := &Relay{store: store, publisher: publisher, config: config, now: time.Now}
	if waker, ok := store.(Waker); ok {
		relay.wake = waker.Wakeups()
	}
	return relay, nil
}

// StartRelay runs relay until Stop is called or parent is canceled. A nil
// relay produces a no-op handle so services can disable MQ through config.
func StartRelay(parent context.Context, relay *Relay) *RelayHandle {
	if relay == nil {
		return &RelayHandle{}
	}
	ctx, cancel := context.WithCancel(parent)
	done := make(chan error, 1)
	go func() {
		done <- relay.Run(ctx)
	}()
	return &RelayHandle{cancel: cancel, done: done}
}

// Stop cancels the relay and waits for it; a cancellation result is not an error.
func (h *RelayHandle) Stop() error {
	if h == nil {
		return nil
	}
	h.once.Do(func() {
		if h.cancel == nil {
			return
		}
		h.cancel()
		h.err = <-h.done
		if errors.Is(h.err, context.Canceled) {
			h.err = nil
		}
	})
	return h.err
}

// ProcessBatch publishes every claimed event independently. Broker success is
// recorded before the lease is released; a crash in between deliberately
// produces a duplicate, which consumers must absorb by event_id.
func (r *Relay) ProcessBatch(ctx context.Context) (int, error) {
	now := r.now()
	records, err := r.store.Claim(ctx, r.config.Owner, r.config.BatchSize, now, r.config.Lease)
	if err != nil {
		return 0, err
	}

	var failures []error
	for _, record := range records {
		if err := r.publisher.Publish(ctx, record); err != nil {
			next := now.Add(retryBackoff(record.Attempts, r.config.BaseBackoff, r.config.MaxBackoff))
			if markErr := r.store.MarkRetry(
				ctx, record.ID, r.config.Owner, record.Attempts, r.config.MaxAttempts, next, err,
			); markErr != nil {
				failures = append(failures, fmt.Errorf("event %d publish failed: %v; mark retry: %w", record.ID, err, markErr))
			} else {
				failures = append(failures, fmt.Errorf("event %d publish: %w", record.ID, err))
			}
			continue
		}
		if err := r.store.MarkSent(ctx, record.ID, r.config.Owner, r.now()); err != nil {
			failures = append(failures, fmt.Errorf("event %d mark sent: %w", record.ID, err))
		} else {
			observeDeliveryLatency(r.config.Service, record.CreatedAt, now.UnixMilli())
		}
	}
	return len(records), errors.Join(failures...)
}

// maxPurgeBatchesPerTick bounds one purge pass so a large first cleanup
// cannot hold the relay loop away from delivery for long.
const maxPurgeBatchesPerTick = 20

// Run drains on start, then on every wakeup or poll tick, while periodically reporting
// the backlog and purging delivered events past retention.
func (r *Relay) Run(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.drain(ctx)
	if err := ctx.Err(); err != nil {
		return err
	}
	r.observeBacklog(ctx)
	if err := ctx.Err(); err != nil {
		return err
	}
	ticker := time.NewTicker(r.config.PollInterval)
	defer ticker.Stop()
	backlogTicker := time.NewTicker(15 * time.Second)
	defer backlogTicker.Stop()
	var purge <-chan time.Time
	if r.config.Retention > 0 {
		purgeTicker := time.NewTicker(r.config.PurgeInterval)
		defer purgeTicker.Stop()
		purge = purgeTicker.C
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-r.wake:
			r.drain(ctx)
		case <-ticker.C:
			r.drain(ctx)
		case <-backlogTicker.C:
			if err := ctx.Err(); err != nil {
				return err
			}
			r.observeBacklog(ctx)
		case <-purge:
			r.purge(ctx)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
}

// drain keeps processing while batches come back full and fully delivered,
// so a burst is not throttled to one batch per poll. Any failure stops the
// pass; failed events already carry their own backoff.
func (r *Relay) drain(ctx context.Context) {
	for ctx.Err() == nil {
		processed, err := r.ProcessBatch(ctx)
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				logging.WithContext(ctx).Errorw("outbox relay batch failed", logging.Field("err", err.Error()))
			}
			return
		}
		if processed < r.config.BatchSize {
			return
		}
	}
}

// purge deletes delivered events older than the retention in bounded batches.
func (r *Relay) purge(ctx context.Context) {
	service := r.config.Service
	if service == "" {
		service = "unknown"
	}
	cutoff := r.now().Add(-r.config.Retention)
	for range maxPurgeBatchesPerTick {
		if ctx.Err() != nil {
			return
		}
		deleted, err := r.store.Purge(ctx, cutoff, r.config.PurgeBatch)
		if err != nil {
			if !errors.Is(err, context.Canceled) && ctx.Err() == nil {
				logging.WithContext(ctx).Errorw("outbox purge failed", logging.Field("err", err.Error()))
			}
			return
		}
		outboxPurgedTotal.Add(float64(deleted), service)
		if deleted < int64(r.config.PurgeBatch) {
			return
		}
	}
}

// observeBacklog reports the backlog metrics; read failures are only logged.
func (r *Relay) observeBacklog(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	service := r.config.Service
	if service == "" {
		service = "unknown"
	}
	backlog, err := r.store.Backlog(ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) || ctx.Err() != nil {
			return
		}
		outboxBacklogCollectionsTotal.Inc(service, "failure")
		logging.WithContext(ctx).Errorw("outbox backlog collection failed", logging.Field("err", err.Error()))
		return
	}
	outboxBacklogCollectionsTotal.Inc(service, "success")
	observeBacklogMetrics(service, backlog, r.now())
}

// retryBackoff doubles the delay per attempt from base, capped at maximum.
func retryBackoff(attempt int, base, maximum time.Duration) time.Duration {
	if attempt <= 1 {
		return base
	}
	power := attempt - 1
	if power > 30 {
		return maximum
	}
	multiplier := int64(math.Pow(2, float64(power)))
	if multiplier > int64(maximum/base) {
		return maximum
	}
	delay := time.Duration(multiplier) * base
	if delay > maximum {
		return maximum
	}
	return delay
}
