package outboxx

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"sync"
	"time"

	"github.com/zeromicro/go-zero/core/logx"
)

type Publisher interface {
	Publish(ctx context.Context, record Record) error
}

type PublisherFunc func(ctx context.Context, record Record) error

func (f PublisherFunc) Publish(ctx context.Context, record Record) error {
	return f(ctx, record)
}

type RelayConfig struct {
	Service      string
	Owner        string
	BatchSize    int
	PollInterval time.Duration
	Lease        time.Duration
	BaseBackoff  time.Duration
	MaxBackoff   time.Duration
	MaxAttempts  int
}

// Config uses millisecond integers so go-zero YAML/env loading stays
// consistent across every business service.
type Config struct {
	BatchSize      int
	PollIntervalMs int
	LeaseMs        int
	BaseBackoffMs  int
	MaxBackoffMs   int
	MaxAttempts    int
}

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
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		hostname = "unknown-host"
	}
	return RelayConfig{
		Service:      service,
		Owner:        fmt.Sprintf("%s-%s-%d", service, hostname, os.Getpid()),
		BatchSize:    c.BatchSize,
		PollInterval: time.Duration(c.PollIntervalMs) * time.Millisecond,
		Lease:        time.Duration(c.LeaseMs) * time.Millisecond,
		BaseBackoff:  time.Duration(c.BaseBackoffMs) * time.Millisecond,
		MaxBackoff:   time.Duration(c.MaxBackoffMs) * time.Millisecond,
		MaxAttempts:  c.MaxAttempts,
	}
}

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
	return nil
}

type Relay struct {
	store     Store
	publisher Publisher
	config    RelayConfig
	now       func() time.Time
}

// RelayHandle owns one background relay run. Stop cancels the run and waits
// for any in-flight store or publisher call to return before releasing it.
type RelayHandle struct {
	cancel context.CancelFunc
	done   chan error
	once   sync.Once
	err    error
}

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
	return &Relay{store: store, publisher: publisher, config: config, now: time.Now}, nil
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

func (r *Relay) Run(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := r.ProcessBatch(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logx.WithContext(ctx).Errorw("outbox relay batch failed", logx.Field("err", err.Error()))
	}
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

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := ctx.Err(); err != nil {
				return err
			}
			if _, err := r.ProcessBatch(ctx); err != nil && !errors.Is(err, context.Canceled) {
				logx.WithContext(ctx).Errorw("outbox relay batch failed", logx.Field("err", err.Error()))
			}
			if err := ctx.Err(); err != nil {
				return err
			}
		case <-backlogTicker.C:
			if err := ctx.Err(); err != nil {
				return err
			}
			r.observeBacklog(ctx)
		}
	}
}

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
		logx.WithContext(ctx).Errorw("outbox backlog collection failed", logx.Field("err", err.Error()))
		return
	}
	outboxBacklogCollectionsTotal.Inc(service, "success")
	observeBacklogMetrics(service, backlog, r.now())
}

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
