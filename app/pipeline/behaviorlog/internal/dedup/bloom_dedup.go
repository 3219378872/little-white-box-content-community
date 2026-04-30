package dedup

import (
	"context"
	"fmt"
	"time"

	"github.com/zeromicro/go-zero/core/bloom"
	"github.com/zeromicro/go-zero/core/stores/redis"
)

const (
	keyPrefix = "bf:behavior_events:"
	ttlHours  = 48
)

type BloomDedup struct {
	rds  *redis.Redis
	bits uint
}

func NewBloomDedup(rds *redis.Redis, bits uint) *BloomDedup {
	return &BloomDedup{rds: rds, bits: bits}
}

func (d *BloomDedup) keyForDate(t time.Time) string {
	return fmt.Sprintf("%s%s", keyPrefix, t.Format("20060102"))
}

func (d *BloomDedup) IsDuplicate(ctx context.Context, eventID string) (bool, error) {
	now := time.Now()
	data := []byte(eventID)

	todayKey := d.keyForDate(now)
	todayFilter := bloom.New(d.rds, todayKey, d.bits)

	exists, err := todayFilter.ExistsCtx(ctx, data)
	if err != nil {
		return false, fmt.Errorf("bloom exists today: %w", err)
	}
	if exists {
		return true, nil
	}

	yesterday := now.AddDate(0, 0, -1)
	yesterdayKey := d.keyForDate(yesterday)
	yesterdayFilter := bloom.New(d.rds, yesterdayKey, d.bits)

	exists, err = yesterdayFilter.ExistsCtx(ctx, data)
	if err != nil {
		return false, fmt.Errorf("bloom exists yesterday: %w", err)
	}
	if exists {
		return true, nil
	}

	if err := todayFilter.AddCtx(ctx, data); err != nil {
		return false, fmt.Errorf("bloom add: %w", err)
	}
	if err := d.rds.ExpireCtx(ctx, todayKey, int(ttlHours*3600)); err != nil {
		return false, fmt.Errorf("bloom expire: %w", err)
	}

	return false, nil
}
