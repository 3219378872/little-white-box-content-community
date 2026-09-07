package store

import (
	"context"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

func (m *MemoryStore) InsertMessage(_ context.Context, msg Message) (Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	msg.ID = m.nextID()
	m.messages[msg.ID] = msg
	return msg, nil
}

func (m *MemoryStore) GetMessage(_ context.Context, userID, id int64) (*Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	msg, ok := m.messages[id]
	if !ok || msg.UserID != userID {
		return nil, sqlx.ErrNotFound
	}
	cp := msg
	return &cp, nil
}

func (m *MemoryStore) ListMessages(_ context.Context, userID, sessionID, beforeID, afterID int64, limit int) ([]Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if limit <= 0 {
		limit = 50
	}
	out := make([]Message, 0)
	for _, msg := range m.messages {
		if msg.UserID != userID || msg.DeletedAtMs != 0 || !msg.Visible {
			continue
		}
		if sessionID > 0 && msg.SessionID != sessionID {
			continue
		}
		if msg.ID <= afterID {
			continue
		}
		if afterID == 0 && beforeID > 0 && msg.ID >= beforeID {
			continue
		}
		out = append(out, msg)
	}
	sortMessages(out)
	if len(out) > limit {
		if afterID > 0 {
			out = out[:limit]
		} else {
			out = out[len(out)-limit:]
		}
	}
	return out, nil
}

func (m *MemoryStore) ListSessionMessages(_ context.Context, userID, sessionID int64, includeHidden bool) ([]Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Message, 0)
	for _, msg := range m.messages {
		if msg.UserID != userID || msg.SessionID != sessionID || msg.DeletedAtMs != 0 {
			continue
		}
		if !includeHidden && !msg.Visible {
			continue
		}
		out = append(out, msg)
	}
	sortMessages(out)
	return out, nil
}

func (m *MemoryStore) GetMessagesByIDs(_ context.Context, userID int64, ids []int64) ([]Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Message, 0, len(ids))
	for _, id := range ids {
		msg, ok := m.messages[id]
		if ok && msg.UserID == userID {
			out = append(out, msg)
		}
	}
	return out, nil
}

func (m *MemoryStore) ListHistoryAround(_ context.Context, userID, messageID int64, before, after int, cutoffMs int64, excludeIDs []int64) ([]Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	excluded := int64Set(excludeIDs)
	anchorMessage, ok := m.messages[messageID]
	if !ok || !historyMessageEligible(anchorMessage, userID, cutoffMs, excluded) {
		return nil, nil
	}
	eligible := make([]Message, 0)
	for _, message := range m.messages {
		if message.SessionID == anchorMessage.SessionID && historyMessageEligible(message, userID, cutoffMs, excluded) {
			eligible = append(eligible, message)
		}
	}
	sortMessages(eligible)
	anchor := -1
	for index, message := range eligible {
		if message.ID == messageID {
			anchor = index
			break
		}
	}
	if anchor < 0 {
		return nil, nil
	}
	before = boundedHistoryEdge(before)
	after = boundedHistoryEdge(after)
	start := anchor - before
	if start < 0 {
		start = 0
	}
	end := anchor + after + 1
	if end > len(eligible) {
		end = len(eligible)
	}
	return append([]Message(nil), eligible[start:end]...), nil
}

func (m *MemoryStore) ListHistorySessionSummaries(_ context.Context, userID, sessionID int64, limit int, cutoffMs int64, excludeIDs []int64) ([]HistorySessionSummary, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	excluded := int64Set(excludeIDs)
	grouped := map[int64][]Message{}
	for _, message := range m.messages {
		if sessionID > 0 && message.SessionID != sessionID {
			continue
		}
		if historyMessageEligible(message, userID, cutoffMs, excluded) {
			grouped[message.SessionID] = append(grouped[message.SessionID], message)
		}
	}
	out := make([]HistorySessionSummary, 0, len(grouped))
	for id, messages := range grouped {
		sortMessages(messages)
		out = append(out, HistorySessionSummary{
			SessionID: id, First: messages[0], Last: messages[len(messages)-1], LastAtMs: messages[len(messages)-1].CreatedAtMs,
		})
	}
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].LastAtMs > out[i].LastAtMs || (out[j].LastAtMs == out[i].LastAtMs && out[j].SessionID > out[i].SessionID) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	if limit <= 0 {
		limit = 3
	}
	if limit > 10 {
		limit = 10
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *MemoryStore) SoftDeleteMessages(_ context.Context, userID, deletedAtMs int64) ([]int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := make([]int64, 0)
	for id, msg := range m.messages {
		if msg.UserID == userID && msg.DeletedAtMs == 0 {
			msg.DeletedAtMs = deletedAtMs
			m.messages[id] = msg
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func (m *MemoryStore) MarkMessagesRead(_ context.Context, userID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, msg := range m.messages {
		if msg.UserID == userID {
			msg.Unread = false
			m.messages[id] = msg
		}
	}
	return nil
}

func (m *MemoryStore) MarkMessagesCompacted(_ context.Context, ids []int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	want := map[int64]struct{}{}
	for _, id := range ids {
		want[id] = struct{}{}
	}
	for id, msg := range m.messages {
		if _, ok := want[id]; ok {
			msg.Compacted = true
			m.messages[id] = msg
		}
	}
	return nil
}
