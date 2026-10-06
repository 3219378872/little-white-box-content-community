package dedup

import (
	"context"
	"fmt"
)

// exactKeyPrefix 带版本号，事件 ID 规则变化时可整体切换键空间。
const exactKeyPrefix = "dedup:behavior:v2:"

// ExactStore 是去重标记的存储接口。
type ExactStore interface {
	Exists(ctx context.Context, key string) (bool, error)
	Set(ctx context.Context, key, value string, seconds int) error
}

// ExactDedup 按事件 ID 精确去重，标记在 TTL 后过期。
type ExactDedup struct {
	store      ExactStore
	ttlSeconds int
}

// NewExactDedup 创建去重器。
func NewExactDedup(store ExactStore, ttlSeconds int) *ExactDedup {
	return &ExactDedup{store: store, ttlSeconds: ttlSeconds}
}

// IsDuplicate 判断事件是否已经处理过。
func (d *ExactDedup) IsDuplicate(ctx context.Context, eventID string) (bool, error) {
	exists, err := d.store.Exists(ctx, exactKeyPrefix+eventID)
	if err != nil {
		return false, fmt.Errorf("exact dedup exists: %w", err)
	}
	return exists, nil
}

// MarkProcessed 在事件成功落库后写入去重标记。
func (d *ExactDedup) MarkProcessed(ctx context.Context, eventID string) error {
	if err := d.store.Set(ctx, exactKeyPrefix+eventID, "1", d.ttlSeconds); err != nil {
		return fmt.Errorf("exact dedup set: %w", err)
	}
	return nil
}
