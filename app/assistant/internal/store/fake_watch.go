package store

import (
	"context"
	"errors"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

func (m *MemoryStore) UpsertDeliveryBucket(_ context.Context, userID, hitID, windowStartMs, nowMs int64) (DeliveryBucket, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := bucketKey(userID, windowStartMs)
	if id, ok := m.bucketByKey[key]; ok {
		b := m.buckets[id]
		b.HitIDs = append(b.HitIDs, hitID)
		m.buckets[id] = b
		return b, nil
	}
	id := m.nextID()
	b := DeliveryBucket{ID: id, UserID: userID, WindowStartMs: windowStartMs, Status: "pending", HitIDs: []int64{hitID}, CreatedAtMs: nowMs}
	m.buckets[id] = b
	m.bucketByKey[key] = id
	return b, nil
}

func (m *MemoryStore) GetBucket(_ context.Context, id int64) (*DeliveryBucket, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.buckets[id]
	if !ok {
		return nil, nil
	}
	cp := b
	return &cp, nil
}

func (m *MemoryStore) ListDueBuckets(_ context.Context, nowMs, windowMs int64) ([]DeliveryBucket, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]DeliveryBucket, 0)
	for _, b := range m.buckets {
		due := (b.NotBeforeMs == 0 && b.WindowStartMs+windowMs <= nowMs) || (b.NotBeforeMs > 0 && b.NotBeforeMs <= nowMs)
		if (b.Status == "pending" || b.Status == "deferred") && due {
			out = append(out, b)
		}
	}
	return out, nil
}

func (m *MemoryStore) GetPendingBucket(_ context.Context, userID int64) (*DeliveryBucket, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var best *DeliveryBucket
	for _, b := range m.buckets {
		if b.UserID != userID || (b.Status != "pending" && b.Status != "deferred") {
			continue
		}
		cp := b
		if best == nil || cp.WindowStartMs < best.WindowStartMs {
			best = &cp
		}
	}
	return best, nil
}

func (m *MemoryStore) MarkBucketScheduled(_ context.Context, id, runID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.buckets[id]
	if !ok || (b.Status != "pending" && b.Status != "deferred") || b.RunID != 0 {
		return errors.New("watch bucket schedule CAS failed")
	}
	b.Status = "scheduled"
	b.RunID = runID
	b.NotBeforeMs = 0
	m.buckets[id] = b
	return nil
}

func (m *MemoryStore) MarkBucketSent(_ context.Context, id, runID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	b := m.buckets[id]
	if b.Status != "scheduled" || b.RunID != runID {
		return sqlx.ErrNotFound
	}
	b.Status = "sent"
	m.buckets[id] = b
	return nil
}

func (m *MemoryStore) DeferBucket(_ context.Context, id, notBeforeMs int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	b := m.buckets[id]
	b.Status = "deferred"
	b.RunID = 0
	b.NotBeforeMs = notBeforeMs
	m.buckets[id] = b
	return nil
}

func (m *MemoryStore) DismissBucket(_ context.Context, id, runID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.buckets[id]
	if !ok {
		return sqlx.ErrNotFound
	}
	allowed := runID == 0 && (b.Status == "pending" || b.Status == "deferred") && b.RunID == 0
	if runID > 0 {
		allowed = b.Status == "scheduled" && b.RunID == runID
	}
	if allowed {
		m.releaseWatchReservationsLocked(id)
		b.Status = "discarded"
		b.RunID = 0
		b.NotBeforeMs = 0
		m.buckets[id] = b
	}
	return nil
}

func (m *MemoryStore) ResetBucket(_ context.Context, id, runID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	b := m.buckets[id]
	if b.Status == "scheduled" && b.RunID == runID {
		m.releaseWatchReservationsLocked(id)
		b.Status = "pending"
		b.RunID = 0
		b.NotBeforeMs = 0
		m.buckets[id] = b
	}
	return nil
}

func (m *MemoryStore) RequeueFailedBuckets(_ context.Context, nowMs int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, b := range m.buckets {
		run, ok := m.runs[b.RunID]
		if b.Status == "scheduled" && ok && run.UserID == b.UserID && run.Source == SourceWatch && (run.Status == StatusError || run.Status == StatusCancelled) {
			target, notBeforeMs := "discarded", int64(0)
			failedAttempts := 0
			if watchBucketIDFromPayload(run.QueuedPayload) == id {
				if run.Status == StatusError {
					failedAttempts = m.failedWatchAttemptsLocked(id, b.UserID, run.ID)
				}
				var err error
				target, notBeforeMs, err = watchTerminalBucketState(run.Status, b, failedAttempts, nowMs)
				if err != nil {
					return err
				}
			}
			m.releaseWatchReservationsLocked(id)
			b.Status = target
			b.RunID = 0
			b.NotBeforeMs = notBeforeMs
			m.buckets[id] = b
		}
	}
	return nil
}

func (m *MemoryStore) FinishWatchDelivery(_ context.Context, id, userID, runID int64, runStatus string, nowMs int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	bucket, ok := m.buckets[id]
	if !ok || bucket.UserID != userID {
		return sqlx.ErrNotFound
	}
	if bucket.Status == "sent" {
		return nil
	}
	if bucket.Status == "pending" || bucket.Status == "deferred" || bucket.Status == "discarded" {
		m.releaseWatchReservationsLocked(id)
		return nil
	}
	if bucket.RunID != runID {
		// A stale finalizer may race a new delivery for the same bucket. It
		// must not fail the old run or release the new run's reservation.
		return nil
	}
	if bucket.Status != "scheduled" {
		return errors.New("watch bucket is not scheduled")
	}
	if runStatus != StatusDone {
		failedAttempts := 0
		if runStatus == StatusError {
			failedAttempts = m.failedWatchAttemptsLocked(id, userID, runID)
		}
		target, notBeforeMs, err := watchTerminalBucketState(runStatus, bucket, failedAttempts, nowMs)
		if err != nil {
			return err
		}
		m.releaseWatchReservationsLocked(id)
		bucket.Status = target
		bucket.RunID = 0
		bucket.NotBeforeMs = notBeforeMs
		m.buckets[id] = bucket
		return nil
	}
	bucket.Status = "sent"
	m.buckets[id] = bucket
	for key := range m.reservations[id] {
		if m.reserved[key] > 0 {
			m.reserved[key]--
		}
		m.sent[key]++
	}
	delete(m.reservations, id)
	return nil
}

func (m *MemoryStore) failedWatchAttemptsLocked(bucketID, userID, currentRunID int64) int {
	current, ok := m.runs[currentRunID]
	bucket, bucketOK := m.buckets[bucketID]
	if !ok || !bucketOK || bucket.UserID != userID || current.UserID != userID || current.Source != SourceWatch || current.Status != StatusError || watchBucketIDFromPayload(current.QueuedPayload) != bucketID {
		return watchDeliveryMaxAttempts
	}
	attempts := 1
	for id, run := range m.runs {
		if id < currentRunID && run.CreatedAtMs >= bucket.CreatedAtMs && run.UserID == userID && run.Source == SourceWatch && run.Status == StatusError && watchBucketIDFromPayload(run.QueuedPayload) == bucketID {
			attempts++
			if attempts >= watchDeliveryMaxAttempts {
				return attempts
			}
		}
	}
	return attempts
}

func (m *MemoryStore) ResetUnsentBuckets(_ context.Context, userID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, b := range m.buckets {
		if b.UserID == userID && b.Status == "scheduled" {
			m.releaseWatchReservationsLocked(id)
			b.Status = "pending"
			b.RunID = 0
			b.NotBeforeMs = 0
			m.buckets[id] = b
		}
	}
	return nil
}
