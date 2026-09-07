package store

import (
	"context"
	"errors"
)

func (m *MemoryStore) InsertEvent(_ context.Context, runID int64, eventType string, payload []byte, createdAtMs int64) (Event, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if eventType == EventDone || eventType == EventError {
		for _, existing := range m.events[runID] {
			if existing.Type == EventDone || existing.Type == EventError {
				return Event{}, errors.New("duplicate terminal event")
			}
		}
	}
	seq := int64(len(m.events[runID]) + 1)
	ev := Event{ID: m.nextID(), RunID: runID, Seq: seq, Type: eventType, PayloadJSON: payload, CreatedAtMs: createdAtMs}
	m.events[runID] = append(m.events[runID], ev)
	return ev, nil
}

func (m *MemoryStore) ListEventsAfter(_ context.Context, runID, afterSeq int64) ([]Event, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Event, 0)
	for _, ev := range m.events[runID] {
		if ev.Seq > afterSeq {
			out = append(out, ev)
		}
	}
	return out, nil
}

func (m *MemoryStore) MaxEventSeq(_ context.Context, runID int64) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return int64(len(m.events[runID])), nil
}
