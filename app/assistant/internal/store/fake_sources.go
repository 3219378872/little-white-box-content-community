package store

import (
	"context"
)

// InsertSource is the in-memory Store.InsertSource used by unit tests.
func (m *MemoryStore) InsertSource(_ context.Context, src Source) (Source, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	src.ID = m.nextID()
	m.sources[sourceKey(src.RunID, src.Handle)] = src
	return src, nil
}

// GetSources is the in-memory Store.GetSources used by unit tests.
func (m *MemoryStore) GetSources(_ context.Context, runID int64, handles []string) ([]Source, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Source, 0)
	for _, handle := range handles {
		if src, ok := m.sources[sourceKey(runID, handle)]; ok {
			out = append(out, src)
		}
	}
	return out, nil
}

// ListSources is the in-memory Store.ListSources used by unit tests.
func (m *MemoryStore) ListSources(_ context.Context, runID int64) ([]Source, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Source, 0)
	for _, src := range m.sources {
		if src.RunID == runID {
			out = append(out, src)
		}
	}
	return out, nil
}
