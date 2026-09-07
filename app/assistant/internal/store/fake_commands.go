package store

import (
	"context"
	"errors"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

func (m *MemoryStore) InsertToolCall(_ context.Context, call ToolCall) (ToolCall, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	call.ID = m.nextID()
	m.toolCalls[toolKey(call.RunID, call.CallID)] = call
	return call, nil
}

func (m *MemoryStore) GetToolCall(_ context.Context, runID int64, callID string) (*ToolCall, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	call, ok := m.toolCalls[toolKey(runID, callID)]
	if !ok {
		return nil, nil
	}
	cp := call
	return &cp, nil
}

func (m *MemoryStore) UpdateToolCall(_ context.Context, call ToolCall) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := toolKey(call.RunID, call.CallID)
	existing, ok := m.toolCalls[key]
	if !ok {
		return sqlx.ErrNotFound
	}
	existing.Status = call.Status
	existing.ResultJSON = call.ResultJSON
	m.toolCalls[key] = existing
	return nil
}

func (m *MemoryStore) ListToolCalls(_ context.Context, runID int64) ([]ToolCall, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]ToolCall, 0)
	for _, call := range m.toolCalls {
		if call.RunID == runID {
			out = append(out, call)
		}
	}
	return out, nil
}

func (m *MemoryStore) GetJournal(_ context.Context, userID int64, requestID, tool, digest string) (*Journal, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	row, ok := m.journals[journalKey(userID, requestID, tool, digest)]
	if !ok {
		return nil, nil
	}
	cp := row
	return &cp, nil
}

func (m *MemoryStore) ReserveJournal(_ context.Context, row Journal) (*Journal, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := journalKey(row.UserID, row.RequestID, row.Tool, row.CanonicalArgsDigest)
	if existing, ok := m.journals[key]; ok {
		if existing.Status != JournalSuccess && existing.LeaseGeneration < row.LeaseGeneration {
			row.ID = existing.ID
			row.CreatedAtMs = existing.CreatedAtMs
			row.Status = JournalPending
			row.ResultJSON = ""
			row.Takeover = true
			m.journals[key] = row
			return &row, true, nil
		}
		cp := existing
		return &cp, false, nil
	}
	row.ID = m.nextID()
	m.journals[key] = row
	return &row, true, nil
}

func (m *MemoryStore) CompleteJournal(_ context.Context, id int64, status, resultJSON string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for key, row := range m.journals {
		if row.ID == id {
			row.Status = status
			row.ResultJSON = resultJSON
			row.UpdatedAtMs = NowMs()
			m.journals[key] = row
			return nil
		}
	}
	return sqlx.ErrNotFound
}

func (m *MemoryStore) ListSuccessfulJournal(_ context.Context, userID int64, requestID string) ([]Journal, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Journal, 0)
	for _, row := range m.journals {
		if row.UserID == userID && row.RequestID == requestID && row.Status == JournalSuccess {
			out = append(out, row)
		}
	}
	return out, nil
}

func (m *MemoryStore) InsertConfirmation(_ context.Context, row Confirmation) (Confirmation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	row.ID = m.nextID()
	m.confirms[confirmKey(row.RunID, row.CallID)] = row
	return row, nil
}

func (m *MemoryStore) GetConfirmation(_ context.Context, runID int64, callID string) (*Confirmation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	row, ok := m.confirms[confirmKey(runID, callID)]
	if !ok {
		return nil, nil
	}
	cp := row
	return &cp, nil
}

func (m *MemoryStore) ResolveConfirmation(_ context.Context, userID, runID int64, callID, digest string, approved bool, nowMs int64) (*Confirmation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := confirmKey(runID, callID)
	row, ok := m.confirms[key]
	if !ok {
		return nil, nil
	}
	if row.UserID != userID || row.CanonicalArgsDigest != digest || row.Status != ConfirmPending {
		cp := row
		return &cp, nil
	}
	if approved {
		row.Status = ConfirmApproved
	} else {
		row.Status = ConfirmRejected
	}
	row.ResolvedAtMs = nowMs
	m.confirms[key] = row
	cp := row
	return &cp, nil
}

func (m *MemoryStore) GetInputCommand(_ context.Context, userID int64, requestID string) (*InputCommand, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	row, ok := m.inputCommands[inputCommandKey(userID, requestID)]
	if !ok {
		return nil, nil
	}
	cp := row
	return &cp, nil
}

func (m *MemoryStore) InsertInputCommand(_ context.Context, command InputCommand) (InputCommand, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := inputCommandKey(command.UserID, command.RequestID)
	if _, exists := m.inputCommands[key]; exists {
		return InputCommand{}, errors.New("duplicate assistant input command")
	}
	command.ID = m.nextID()
	m.inputCommands[key] = command
	return command, nil
}

func (m *MemoryStore) CountQueue(_ context.Context, runID int64) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.queue[runID]), nil
}

func (m *MemoryStore) Enqueue(_ context.Context, item QueueItem) (QueueItem, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item.ID = m.nextID()
	m.queue[item.RunID] = append(m.queue[item.RunID], item)
	return item, nil
}

func (m *MemoryStore) ListQueue(_ context.Context, runID int64) ([]QueueItem, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := append([]QueueItem(nil), m.queue[runID]...)
	return out, nil
}

func (m *MemoryStore) DeleteQueueThrough(_ context.Context, runID, maxID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	kept := m.queue[runID][:0]
	for _, item := range m.queue[runID] {
		if item.ID > maxID {
			kept = append(kept, item)
		}
	}
	m.queue[runID] = kept
	return nil
}

func (m *MemoryStore) DeleteQueue(_ context.Context, runID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.queue, runID)
	return nil
}
