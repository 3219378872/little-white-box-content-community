package store

import (
	"context"
	"time"
)

func (m *MemoryStore) ReserveWatchQuota(_ context.Context, bucketID, userID int64, taskIDs []int64, dayStartMs, hourStartMs int64, dailyLimit, hourlyLimit int) (bool, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	bucket, ok := m.buckets[bucketID]
	if !ok || bucket.UserID != userID || (bucket.Status != "pending" && bucket.Status != "deferred") {
		return false, 0, nil
	}
	if _, exists := m.reservations[bucketID]; exists {
		return true, 0, nil
	}
	dayKey := sentKey(userID, 0, "day", dayStartMs)
	if m.sent[dayKey]+m.reserved[dayKey] >= dailyLimit {
		return false, dayStartMs + int64((24 * time.Hour).Milliseconds()), nil
	}
	seen := map[int64]struct{}{}
	for _, taskID := range taskIDs {
		if taskID <= 0 {
			continue
		}
		if _, duplicate := seen[taskID]; duplicate {
			continue
		}
		seen[taskID] = struct{}{}
		key := sentKey(userID, taskID, "hour", hourStartMs)
		if m.sent[key]+m.reserved[key] >= hourlyLimit {
			return false, hourStartMs + int64(time.Hour.Milliseconds()), nil
		}
	}
	keys := map[string]struct{}{dayKey: {}}
	for taskID := range seen {
		keys[sentKey(userID, taskID, "hour", hourStartMs)] = struct{}{}
	}
	for key := range keys {
		m.reserved[key]++
	}
	m.reservations[bucketID] = keys
	return true, 0, nil
}

func (m *MemoryStore) releaseWatchReservationsLocked(bucketID int64) {
	for key := range m.reservations[bucketID] {
		if m.reserved[key] > 0 {
			m.reserved[key]--
		}
	}
	delete(m.reservations, bucketID)
}

func (m *MemoryStore) CountSent(_ context.Context, userID, taskID int64, periodKind string, periodStartMs int64) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sent[sentKey(userID, taskID, periodKind, periodStartMs)], nil
}

func (m *MemoryStore) IncrSent(_ context.Context, userID, taskID int64, periodKind string, periodStartMs int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent[sentKey(userID, taskID, periodKind, periodStartMs)]++
	return nil
}
