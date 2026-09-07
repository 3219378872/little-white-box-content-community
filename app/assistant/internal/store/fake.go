package store

import (
	"context"
	"sync"
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
