package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"

	"esx/pkg/event"

	"github.com/go-sql-driver/mysql"
)

// countSyncDedupTTLSeconds 与 REL-008 去重语义一致（90 天）。
const countSyncDedupTTLSeconds = 7776000

const (
	postCacheKeyPrefix    = "cache:post:id:"
	commentCacheKeyPrefix = "cache:comment:id:"
	countSyncDedupPrefix  = "content:countsync:dedup:"
)

// CountSyncStore 将互动权威事务产生的行为事件同步到内容计数列（CORE-032）。
type CountSyncStore interface {
	ApplyBehaviorCount(ctx context.Context, behavior event.BehaviorEvent) error
}

type RedisCmdable interface {
	SetnxExCtx(ctx context.Context, key, value string, seconds int) (bool, error)
	DelCtx(ctx context.Context, keys ...string) (int, error)
}

type DBExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

type countSyncStore struct {
	db    DBExecutor
	redis RedisCmdable
}

func NewCountSyncStore(db DBExecutor, redisClient RedisCmdable) CountSyncStore {
	return &countSyncStore{db: db, redis: redisClient}
}

func (s *countSyncStore) ApplyBehaviorCount(ctx context.Context, behavior event.BehaviorEvent) error {
	if err := behavior.Validate(); err != nil {
		return fmt.Errorf("count-sync: invalid behavior event: %w", err)
	}
	column, delta, ok := behaviorCountUpdate(behavior.Action)
	if !ok {
		return nil
	}
	if behavior.TargetID <= 0 {
		return fmt.Errorf("count-sync: target id is required")
	}

	// 去重：同事件重投/重放不得重复累计（REL-008）。
	dedupKey := countSyncDedupPrefix + behavior.EventIDString()
	first, err := s.redis.SetnxExCtx(ctx, dedupKey, "1", countSyncDedupTTLSeconds)
	if err != nil {
		return fmt.Errorf("count-sync: dedup reserve failed: %w", err)
	}
	if !first {
		return nil
	}

	if err := s.applyDelta(ctx, behavior.TargetType, behavior.TargetID, column, delta); err != nil {
		return fmt.Errorf("count-sync: apply delta: %w", err)
	}
	s.invalidateCaches(ctx, behavior.TargetType, behavior.TargetID)
	return nil
}

func behaviorCountUpdate(action string) (column string, delta int64, ok bool) {
	switch action {
	case event.BehaviorActionLike:
		return "like_count", 1, true
	case event.BehaviorActionUnlike:
		return "like_count", -1, true
	case event.BehaviorActionFavorite:
		return "favorite_count", 1, true
	case event.BehaviorActionUnfavorite:
		return "favorite_count", -1, true
	default:
		return "", 0, false
	}
}

func (s *countSyncStore) applyDelta(ctx context.Context, targetType string, targetID int64, column string, delta int64) error {
	if column != "like_count" && column != "favorite_count" {
		return fmt.Errorf("count-sync: unsupported column %q", column)
	}
	switch targetType {
	case "post":
		if delta > 0 {
			_, err := s.db.ExecContext(ctx,
				"UPDATE `post` SET `"+column+"` = `"+column+"` + ? WHERE `id` = ?",
				delta, targetID)
			return err
		}
		_, err := s.db.ExecContext(ctx,
			"UPDATE `post` SET `"+column+"` = GREATEST(`"+column+"` + ?, 0) WHERE `id` = ?",
			delta, targetID)
		return err
	case "comment":
		if delta > 0 {
			_, err := s.db.ExecContext(ctx,
				"UPDATE `comment` SET `"+column+"` = `"+column+"` + ? WHERE `id` = ?",
				delta, targetID)
			return err
		}
		_, err := s.db.ExecContext(ctx,
			"UPDATE `comment` SET `"+column+"` = GREATEST(`"+column+"` + ?, 0) WHERE `id` = ?",
			delta, targetID)
		return err
	default:
		return fmt.Errorf("count-sync: unsupported target type %q", targetType)
	}
}

func (s *countSyncStore) invalidateCaches(ctx context.Context, targetType string, targetID int64) {
	key := postCacheKeyPrefix + strconv.FormatInt(targetID, 10)
	if targetType == "comment" {
		key = commentCacheKeyPrefix + strconv.FormatInt(targetID, 10)
	}
	if _, err := s.redis.DelCtx(ctx, key); err != nil {
		// 缓存失效失败不改变已提交的计数（CORE-053）；只记录。
		return
	}
}

// IsDuplicateKeyError 供测试与日志判断唯一键冲突。
func IsDuplicateKeyError(err error) bool {
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1062
}

var _ = sql.ErrNoRows
