package store

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

// MemoryStore is an in-memory Store for unit tests.
type MemoryStore struct {
	txMu          sync.Mutex
	stepMu        sync.Mutex
	mu            sync.Mutex
	next          int64
	threads       map[int64]Thread
	sessions      map[int64]Session
	messages      map[int64]Message
	runs          map[int64]Run
	events        map[int64][]Event
	toolCalls     map[string]ToolCall
	journals      map[string]Journal
	sources       map[string]Source
	confirms      map[string]Confirmation
	inputCommands map[string]InputCommand
	queue         map[int64][]QueueItem
	alerts        map[string]Alert
	outbox        []Outbox
	buckets       map[int64]DeliveryBucket
	bucketByKey   map[string]int64
	sent          map[string]int
	reserved      map[string]int
	reservations  map[int64]map[string]struct{}
	claimFail     bool
	consents      map[int64]int32
	evidence      map[string]Evidence
	questions     map[string]QuestionRequest
	presentations map[int64]AnswerPresentation
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		next:          1,
		threads:       map[int64]Thread{},
		sessions:      map[int64]Session{},
		messages:      map[int64]Message{},
		runs:          map[int64]Run{},
		events:        map[int64][]Event{},
		toolCalls:     map[string]ToolCall{},
		journals:      map[string]Journal{},
		sources:       map[string]Source{},
		confirms:      map[string]Confirmation{},
		inputCommands: map[string]InputCommand{},
		queue:         map[int64][]QueueItem{},
		alerts:        map[string]Alert{},
		buckets:       map[int64]DeliveryBucket{},
		bucketByKey:   map[string]int64{},
		sent:          map[string]int{},
		reserved:      map[string]int{},
		reservations:  map[int64]map[string]struct{}{},
		consents:      map[int64]int32{},
		evidence:      map[string]Evidence{},
		questions:     map[string]QuestionRequest{},
		presentations: map[int64]AnswerPresentation{},
	}
}

func (m *MemoryStore) nextID() int64 {
	id := m.next
	m.next++
	return id
}

func (m *MemoryStore) Transact(ctx context.Context, fn func(ctx context.Context, tx Store) error) error {
	m.txMu.Lock()
	defer m.txMu.Unlock()
	return fn(ctx, m)
}

func (m *MemoryStore) RunStep(ctx context.Context, fence LeaseFence, fn func(ctx context.Context, tx Store) error) error {
	m.stepMu.Lock()
	defer m.stepMu.Unlock()
	m.mu.Lock()
	run, ok := m.runs[fence.RunID]
	valid := ok && run.Status == StatusRunning && run.LeaseOwner == fence.Owner &&
		run.LeaseGeneration == fence.Generation && run.LeaseUntilMs >= NowMs()
	m.mu.Unlock()
	if !valid {
		return ErrLeaseLost
	}
	return fn(ctx, m)
}

func (m *MemoryStore) LockThread(_ context.Context, userID int64) (*Thread, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	thread, ok := m.threads[userID]
	if !ok {
		thread = Thread{UserID: userID, UpdatedAtMs: NowMs()}
		m.threads[userID] = thread
	}
	cp := thread
	return &cp, nil
}

func (m *MemoryStore) GetThread(_ context.Context, userID int64) (*Thread, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	thread, ok := m.threads[userID]
	if !ok {
		return &Thread{UserID: userID}, nil
	}
	cp := thread
	return &cp, nil
}

func (m *MemoryStore) SaveThread(_ context.Context, thread Thread) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.threads[thread.UserID] = thread
	return nil
}

func (m *MemoryStore) CreateSession(_ context.Context, session Session) (Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	session.ID = m.nextID()
	m.sessions[session.ID] = session
	return session, nil
}

func (m *MemoryStore) GetSession(_ context.Context, id int64) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	session, ok := m.sessions[id]
	if !ok {
		return nil, sqlx.ErrNotFound
	}
	cp := session
	return &cp, nil
}

func (m *MemoryStore) UpdateSession(_ context.Context, session Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[session.ID] = session
	return nil
}

func (m *MemoryStore) CloseSession(_ context.Context, id int64, closedAtMs int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	session, ok := m.sessions[id]
	if !ok {
		return sqlx.ErrNotFound
	}
	session.Status = SessionClosed
	session.ClosedAtMs = closedAtMs
	m.sessions[id] = session
	return nil
}

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

func (m *MemoryStore) InsertRun(_ context.Context, run Run) (Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	run.ID = m.nextID()
	m.runs[run.ID] = run
	return run, nil
}

func (m *MemoryStore) GetRun(_ context.Context, id int64) (*Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	run, ok := m.runs[id]
	if !ok {
		return nil, sqlx.ErrNotFound
	}
	cp := run
	return &cp, nil
}

func (m *MemoryStore) GetRunByRequestID(_ context.Context, userID int64, requestID string) (*Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, run := range m.runs {
		if run.UserID == userID && run.RequestID == requestID {
			cp := run
			return &cp, nil
		}
	}
	return nil, nil
}

func (m *MemoryStore) UpdateRun(_ context.Context, run Run) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.runs[run.ID]; ok {
		run.CancelRequested = run.CancelRequested || existing.CancelRequested
		run.QueuedPayload = append([]byte(nil), existing.QueuedPayload...)
		run.LeaseOwner = existing.LeaseOwner
		run.LeaseGeneration = existing.LeaseGeneration
		run.LeaseUntilMs = existing.LeaseUntilMs
		run.HeartbeatAtMs = existing.HeartbeatAtMs
		run.ConsentVersion = existing.ConsentVersion
		run.InputVersion = existing.InputVersion
	}
	m.runs[run.ID] = run
	return nil
}

func (m *MemoryStore) ExpireLease(runID, leaseUntilMs int64) {
	m.stepMu.Lock()
	defer m.stepMu.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	run := m.runs[runID]
	run.LeaseUntilMs = leaseUntilMs
	m.runs[runID] = run
}

func (m *MemoryStore) SetRunInput(_ context.Context, runID int64, payload []byte, lastActivityMs int64) error {
	m.stepMu.Lock()
	defer m.stepMu.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	run, ok := m.runs[runID]
	if !ok || (run.Status != StatusRunning && run.Status != StatusQueued) {
		return sqlx.ErrNotFound
	}
	run.QueuedPayload = append([]byte(nil), payload...)
	run.InputVersion++
	run.LastActivityAtMs = lastActivityMs
	m.runs[runID] = run
	return nil
}

func (m *MemoryStore) RequestCancel(_ context.Context, userID, runID int64) error {
	m.stepMu.Lock()
	defer m.stepMu.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	run, ok := m.runs[runID]
	if !ok || run.UserID != userID {
		return sqlx.ErrNotFound
	}
	run.CancelRequested = true
	m.runs[runID] = run
	return nil
}

func (m *MemoryStore) RequestCancelAll(_ context.Context, userID int64) error {
	m.stepMu.Lock()
	defer m.stepMu.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, run := range m.runs {
		if run.UserID == userID && !IsTerminalStatus(run.Status) {
			run.CancelRequested = true
			m.runs[id] = run
		}
	}
	return nil
}

func (m *MemoryStore) CancelOpenBackground(_ context.Context, userID int64, sources []string) ([]Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	want := map[string]struct{}{}
	for _, src := range sources {
		want[src] = struct{}{}
	}
	out := make([]Run, 0)
	for id, run := range m.runs {
		if run.UserID != userID || IsTerminalStatus(run.Status) {
			continue
		}
		if _, ok := want[run.Source]; !ok {
			continue
		}
		run.CancelRequested = true
		m.runs[id] = run
		out = append(out, run)
	}
	return out, nil
}

func (m *MemoryStore) Claim(_ context.Context, owner string, nowMs, leaseMs int64) (*Run, error) {
	m.stepMu.Lock()
	defer m.stepMu.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.claimFail {
		return nil, nil
	}
	var best *Run
	for _, run := range m.runs {
		claimable := run.Status == StatusQueued ||
			(run.Status == StatusRunning && (run.LeaseUntilMs == 0 || run.LeaseUntilMs < nowMs))
		if !claimable {
			continue
		}
		cp := run
		if best == nil || cp.Priority < best.Priority || (cp.Priority == best.Priority && cp.CreatedAtMs < best.CreatedAtMs) {
			best = &cp
		}
	}
	if best == nil {
		return nil, nil
	}
	best.Status = StatusRunning
	if best.StartedAtMs == 0 {
		best.StartedAtMs = nowMs
	}
	best.LeaseOwner = owner
	best.LeaseGeneration++
	best.LeaseUntilMs = nowMs + leaseMs
	best.HeartbeatAtMs = nowMs
	best.LastActivityAtMs = nowMs
	m.runs[best.ID] = *best
	return best, nil
}

func (m *MemoryStore) RenewLease(_ context.Context, runID int64, owner string, generation, leaseUntilMs, heartbeatMs int64) (bool, error) {
	m.stepMu.Lock()
	defer m.stepMu.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	run, ok := m.runs[runID]
	if !ok || run.LeaseOwner != owner || run.LeaseGeneration != generation || run.Status != StatusRunning || run.LeaseUntilMs < heartbeatMs {
		return false, nil
	}
	run.LeaseUntilMs = leaseUntilMs
	run.HeartbeatAtMs = heartbeatMs
	m.runs[runID] = run
	return true, nil
}

func (m *MemoryStore) AgentConsent(_ context.Context, userID int64) (int32, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	version, ok := m.consents[userID]
	if !ok {
		return 2, true, nil
	}
	return version, version > 0, nil
}

func (m *MemoryStore) SetAgentConsent(userID int64, version int32) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.consents[userID] = version
}

func (m *MemoryStore) OldestQueuedAgeMs(_ context.Context, nowMs int64) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var oldest int64
	for _, run := range m.runs {
		if run.Status != StatusQueued {
			continue
		}
		if oldest == 0 || run.CreatedAtMs < oldest {
			oldest = run.CreatedAtMs
		}
	}
	if oldest == 0 {
		return 0, nil
	}
	return nowMs - oldest, nil
}

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

func (m *MemoryStore) InsertSource(_ context.Context, src Source) (Source, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	src.ID = m.nextID()
	m.sources[sourceKey(src.RunID, src.Handle)] = src
	return src, nil
}

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
			failedAttempts := 0
			if run.Status == StatusError {
				failedAttempts = m.failedWatchAttemptsLocked(id, b.UserID, run.ID)
			}
			target, notBeforeMs, err := watchTerminalBucketState(run.Status, b, failedAttempts, nowMs)
			if err != nil {
				return err
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
		return sqlx.ErrNotFound
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
	if !ok || current.UserID != userID || current.Source != SourceWatch || current.Status != StatusError || watchBucketIDFromPayload(current.QueuedPayload) != bucketID {
		return watchDeliveryMaxAttempts
	}
	attempts := 1
	for id, run := range m.runs {
		if id != currentRunID && run.UserID == userID && run.Source == SourceWatch && run.Status == StatusError && watchBucketIDFromPayload(run.QueuedPayload) == bucketID {
			attempts++
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

func sortMessages(msgs []Message) {
	for i := 0; i < len(msgs); i++ {
		for j := i + 1; j < len(msgs); j++ {
			if msgs[j].ID < msgs[i].ID {
				msgs[i], msgs[j] = msgs[j], msgs[i]
			}
		}
	}
}

func toolKey(runID int64, callID string) string {
	return itoa(runID) + ":" + callID
}
func journalKey(userID int64, requestID, tool, digest string) string {
	return itoa(userID) + ":" + requestID + ":" + tool + ":" + digest
}
func sourceKey(runID int64, handle string) string { return itoa(runID) + ":" + handle }
func confirmKey(runID int64, callID string) string {
	return itoa(runID) + ":" + callID
}
func inputCommandKey(userID int64, requestID string) string {
	return itoa(userID) + ":" + requestID
}
func alertKey(runID int64, level, dim string) string {
	return itoa(runID) + ":" + level + ":" + dim
}
func bucketKey(userID, window int64) string { return itoa(userID) + ":" + itoa(window) }
func sentKey(userID, taskID int64, kind string, start int64) string {
	return itoa(userID) + ":" + itoa(taskID) + ":" + kind + ":" + itoa(start)
}
func itoa(v int64) string {
	raw, _ := json.Marshal(v)
	return string(raw)
}

type MemoryNotifier struct {
	mu    sync.Mutex
	token map[int64]int64
}

func NewMemoryNotifier() *MemoryNotifier {
	return &MemoryNotifier{token: map[int64]int64{}}
}

func (n *MemoryNotifier) Wake(_ context.Context, runID int64) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.token[runID]++
	return nil
}

func (n *MemoryNotifier) WakeToken(_ context.Context, runID int64) (string, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	return itoa(n.token[runID]), nil
}
