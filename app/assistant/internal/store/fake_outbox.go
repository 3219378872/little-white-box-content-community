package store

import (
	"context"
)

func (m *MemoryStore) InsertAlert(_ context.Context, alert Alert) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := alertKey(alert.RunID, alert.Level, alert.Dimension)
	if _, ok := m.alerts[key]; ok {
		return false, nil
	}
	m.alerts[key] = alert
	return true, nil
}

func (m *MemoryStore) InsertOutbox(_ context.Context, row Outbox) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	row.ID = m.nextID()
	m.outbox = append(m.outbox, row)
	return nil
}

func (m *MemoryStore) ListUnpublishedOutbox(_ context.Context, limit int) ([]Outbox, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Outbox, 0)
	for _, row := range m.outbox {
		if !row.Published {
			out = append(out, row)
			if limit > 0 && len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

func (m *MemoryStore) MarkOutboxPublished(_ context.Context, ids []int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	want := map[int64]struct{}{}
	for _, id := range ids {
		want[id] = struct{}{}
	}
	for i, row := range m.outbox {
		if _, ok := want[row.ID]; ok {
			row.Published = true
			m.outbox[i] = row
		}
	}
	return nil
}
