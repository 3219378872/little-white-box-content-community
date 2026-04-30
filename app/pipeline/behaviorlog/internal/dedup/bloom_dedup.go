package dedup

import (
	"context"
	"fmt"
	"time"
)

const (
	keyPrefix = "bf:behavior_events:"
	ttlHours  = 48
)

type BloomDedup struct {
	store BloomStore
	bits  uint
}

type BloomStore interface {
	Exists(ctx context.Context, key string, bits uint, data []byte) (bool, error)
	Add(ctx context.Context, key string, bits uint, data []byte) error
	Expire(ctx context.Context, key string, seconds int) error
}

func NewBloomDedup(store BloomStore, bits uint) *BloomDedup {
	return &BloomDedup{store: store, bits: bits}
}

func (d *BloomDedup) keyForDate(t time.Time) string {
	return fmt.Sprintf("%s%s", keyPrefix, t.Format("20060102"))
}

func (d *BloomDedup) IsDuplicate(ctx context.Context, eventID string) (bool, error) {
	now := time.Now()
	data := []byte(eventID)

	todayKey := d.keyForDate(now)
	exists, err := d.store.Exists(ctx, todayKey, d.bits, data)
	if err != nil {
		return false, fmt.Errorf("bloom exists today: %w", err)
	}
	if exists {
		return true, nil
	}

	yesterday := now.AddDate(0, 0, -1)
	yesterdayKey := d.keyForDate(yesterday)
	exists, err = d.store.Exists(ctx, yesterdayKey, d.bits, data)
	if err != nil {
		return false, fmt.Errorf("bloom exists yesterday: %w", err)
	}
	if exists {
		return true, nil
	}

	return false, nil
}

func (d *BloomDedup) MarkProcessed(ctx context.Context, eventID string) error {
	now := time.Now()
	data := []byte(eventID)
	todayKey := d.keyForDate(now)

	if err := d.store.Add(ctx, todayKey, d.bits, data); err != nil {
		return fmt.Errorf("bloom add: %w", err)
	}
	if err := d.store.Expire(ctx, todayKey, int(ttlHours*3600)); err != nil {
		return fmt.Errorf("bloom expire: %w", err)
	}

	return nil
}
