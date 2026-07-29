package dedup

import (
	"context"
	"fmt"
)

const exactKeyPrefix = "dedup:behavior:v2:"

type ExactStore interface {
	Exists(ctx context.Context, key string) (bool, error)
	Set(ctx context.Context, key, value string, seconds int) error
}

type ExactDedup struct {
	store      ExactStore
	ttlSeconds int
}

func NewExactDedup(store ExactStore, ttlSeconds int) *ExactDedup {
	return &ExactDedup{store: store, ttlSeconds: ttlSeconds}
}

func (d *ExactDedup) IsDuplicate(ctx context.Context, eventID string) (bool, error) {
	exists, err := d.store.Exists(ctx, exactKeyPrefix+eventID)
	if err != nil {
		return false, fmt.Errorf("exact dedup exists: %w", err)
	}
	return exists, nil
}

func (d *ExactDedup) MarkProcessed(ctx context.Context, eventID string) error {
	if err := d.store.Set(ctx, exactKeyPrefix+eventID, "1", d.ttlSeconds); err != nil {
		return fmt.Errorf("exact dedup set: %w", err)
	}
	return nil
}
