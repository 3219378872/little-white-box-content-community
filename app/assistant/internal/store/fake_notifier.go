package store

import (
	"context"
	"strconv"
	"sync"
)

// MemoryNotifier counts wakes per run for unit tests.
type MemoryNotifier struct {
	mu    sync.Mutex
	token map[int64]int64
}

// NewMemoryNotifier returns a notifier with all counters at zero.
func NewMemoryNotifier() *MemoryNotifier {
	return &MemoryNotifier{token: map[int64]int64{}}
}

// Wake increments the run's counter.
func (n *MemoryNotifier) Wake(_ context.Context, runID int64) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.token[runID]++
	return nil
}

// WakeToken returns the run's counter as a string, like the Redis notifier.
func (n *MemoryNotifier) WakeToken(_ context.Context, runID int64) (string, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	return strconv.FormatInt(n.token[runID], 10), nil
}
