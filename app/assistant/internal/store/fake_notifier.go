package store

import (
	"context"
	"sync"
)

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
