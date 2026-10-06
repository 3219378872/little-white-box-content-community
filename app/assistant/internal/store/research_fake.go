package store

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
)

// cloneResearch deep-copies research values through JSON so callers cannot mutate stored state.
func cloneResearch[T any](value T) T {
	raw, _ := json.Marshal(value)
	var copy T
	_ = json.Unmarshal(raw, &copy)
	return copy
}

// LockRun is the in-memory Store.LockRun used by unit tests.
func (m *MemoryStore) LockRun(ctx context.Context, id int64) (*Run, error) { return m.GetRun(ctx, id) }

// ListSourceEvents is the in-memory Store.ListSourceEvents used by unit tests.
func (m *MemoryStore) ListSourceEvents(ctx context.Context, runID int64) ([]Event, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Event
	for _, event := range m.events[runID] {
		if event.Type == EventSourceCard {
			out = append(out, event)
		}
	}
	if len(out) > 100 {
		out = out[len(out)-100:]
	}
	return out, nil
}

// HasDeletedRunHistory is the in-memory Store.HasDeletedRunHistory used by unit tests.
func (m *MemoryStore) HasDeletedRunHistory(_ context.Context, run Run) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var inputID int64
	for _, command := range m.inputCommands {
		if command.UserID == run.UserID && command.RequestID == run.RequestID {
			inputID = command.MessageID
			break
		}
	}
	for _, message := range m.messages {
		if message.UserID == run.UserID && message.DeletedAtMs > 0 && (message.RunID == run.ID || message.ID == inputID) {
			return true, nil
		}
	}
	return false, nil
}

// ListWaitingConfirmRuns is the in-memory Store.ListWaitingConfirmRuns used by unit tests.
func (m *MemoryStore) ListWaitingConfirmRuns(context.Context) ([]Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Run
	for _, run := range m.runs {
		if run.Status == StatusWaitingConfirm {
			out = append(out, run)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastActivityAtMs < out[j].LastActivityAtMs })
	if len(out) > 100 {
		out = out[:100]
	}
	return out, nil
}

// ListWaitingRuns is the in-memory Store.ListWaitingRuns used by unit tests.
func (m *MemoryStore) ListWaitingRuns(context.Context) ([]Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Run
	for _, run := range m.runs {
		if run.Status == StatusWaitingInput {
			out = append(out, run)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastActivityAtMs < out[j].LastActivityAtMs })
	if len(out) > 100 {
		out = out[:100]
	}
	return out, nil
}

// PutEvidence is the in-memory Store.PutEvidence used by unit tests.
func (m *MemoryStore) PutEvidence(_ context.Context, item Evidence) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := fmt.Sprintf("%d/%s", item.RunID, item.ID)
	if _, ok := m.evidence[key]; !ok {
		m.evidence[key] = item
	}
	return nil
}

// ListEvidence is the in-memory Store.ListEvidence used by unit tests.
func (m *MemoryStore) ListEvidence(_ context.Context, runID int64, handle string) ([]Evidence, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Evidence
	for _, item := range m.evidence {
		if item.RunID == runID && item.Handle == handle {
			out = append(out, item)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// SaveQuestion is the in-memory Store.SaveQuestion used by unit tests.
func (m *MemoryStore) SaveQuestion(_ context.Context, item QuestionRequest) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	copy := cloneResearch(item)
	copy.UserID, copy.AnswerRequestID, copy.AnswerDigest = item.UserID, item.AnswerRequestID, item.AnswerDigest
	m.questions[fmt.Sprintf("%d/%s", item.RunID, item.ID)] = copy
	return nil
}

// ListQuestions is the in-memory Store.ListQuestions used by unit tests.
func (m *MemoryStore) ListQuestions(_ context.Context, runID int64) ([]QuestionRequest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []QuestionRequest
	for _, item := range m.questions {
		if item.RunID == runID {
			copy := cloneResearch(item)
			copy.UserID, copy.AnswerRequestID, copy.AnswerDigest = item.UserID, item.AnswerRequestID, item.AnswerDigest
			out = append(out, copy)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].MessageID < out[j].MessageID })
	return out, nil
}

// SavePresentation is the in-memory Store.SavePresentation used by unit tests.
func (m *MemoryStore) SavePresentation(_ context.Context, item AnswerPresentation) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.presentations[item.MessageID] = cloneResearch(item)
	return nil
}

// GetPresentation is the in-memory Store.GetPresentation used by unit tests.
func (m *MemoryStore) GetPresentation(_ context.Context, id int64) (*AnswerPresentation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.presentations[id]
	if !ok {
		return nil, nil
	}
	copy := cloneResearch(item)
	return &copy, nil
}

// ClearResearchHistory is the in-memory Store.ClearResearchHistory used by unit tests.
func (m *MemoryStore) ClearResearchHistory(_ context.Context, userID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for key, q := range m.questions {
		if q.UserID == userID {
			delete(m.questions, key)
		}
	}
	for id := range m.presentations {
		if m.messages[id].UserID == userID {
			delete(m.presentations, id)
		}
	}
	for key, ev := range m.evidence {
		if m.runs[ev.RunID].UserID == userID {
			delete(m.evidence, key)
		}
	}
	for key, src := range m.sources {
		if m.runs[src.RunID].UserID == userID {
			delete(m.sources, key)
		}
	}
	return nil
}
