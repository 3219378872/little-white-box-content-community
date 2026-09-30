package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"esx/pkg/cachedstore"
	"esx/pkg/event"
	"esx/pkg/mqx"
	"esx/pkg/outboxx"
	"esx/pkg/poststats"

	"github.com/go-sql-driver/mysql"
)

const (
	postCacheKeyPrefix    = "cache:post:id:"
	commentCacheKeyPrefix = "cache:comment:id:"
)

type CountSyncStore interface {
	ApplyBehaviorCount(context.Context, event.BehaviorEvent) error
}
type RedisCmdable interface {
	DelCtx(context.Context, ...string) (int, error)
}
type countSyncStore struct {
	db    *sql.DB
	redis RedisCmdable
}

func NewCountSyncStore(db *sql.DB, redis RedisCmdable) CountSyncStore {
	return &countSyncStore{db: db, redis: redis}
}

// A complete target snapshot makes delivery order irrelevant. The receipt,
// version fence, both counts, and downstream search event commit together.
// Redis is disposable: neither a reservation nor its loss can acknowledge work.
func (s *countSyncStore) ApplyBehaviorCount(ctx context.Context, behavior event.BehaviorEvent) error {
	if err := behavior.Validate(); err != nil {
		return fmt.Errorf("count-sync: invalid behavior event: %w", err)
	}
	switch behavior.Action {
	case event.BehaviorActionLike, event.BehaviorActionUnlike, event.BehaviorActionFavorite, event.BehaviorActionUnfavorite:
	default:
		return nil
	}
	snapshot := behavior.CountSnapshot
	if snapshot != nil && (snapshot.Revision <= 0 || snapshot.LikeCount < 0 || snapshot.FavoriteCount < 0) {
		return fmt.Errorf("count-sync: authoritative count snapshot required; reconcile legacy events before replay")
	}
	table := behavior.TargetType
	if table != "post" && table != "comment" {
		return fmt.Errorf("count-sync: unsupported target type %q", table)
	}
	if s.db == nil {
		return fmt.Errorf("count-sync: database is not configured")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if snapshot == nil {
		// Only a completed, paused-writer reconciliation makes historical
		// unversioned events safe to acknowledge without reapplying deltas.
		var reconciled bool
		if err = tx.QueryRowContext(ctx, "SELECT legacy_reconciled FROM content_count_projection_control WHERE id = 1").Scan(&reconciled); err != nil {
			return err
		}
		if !reconciled {
			return fmt.Errorf("count-sync: authoritative count snapshot required; complete legacy reconciliation before replay")
		}
	}
	// A duplicate INSERT blocks behind an uncommitted original transaction. Only
	// a committed receipt is a no-op; a rollback/crash lets redelivery acquire it.
	_, err = tx.ExecContext(ctx, "INSERT INTO content_count_receipt (event_id, created_at) VALUES (?, ?)", behavior.EventID, time.Now().UnixMilli())
	if IsDuplicateKeyError(err) {
		s.invalidateCaches(ctx, table, behavior.TargetID)
		return nil
	}
	if err != nil {
		return err
	}
	if snapshot == nil {
		if err = tx.Commit(); err != nil {
			return err
		}
		s.invalidateCaches(ctx, table, behavior.TargetID)
		return nil
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO content_count_projection (target_type, target_id, revision) VALUES (?, ?, 0) ON DUPLICATE KEY UPDATE target_id = VALUES(target_id)", table, behavior.TargetID)
	if err != nil {
		return err
	}
	var current int64
	if err = tx.QueryRowContext(ctx, "SELECT revision FROM content_count_projection WHERE target_type = ? AND target_id = ? FOR UPDATE", table, behavior.TargetID).Scan(&current); err != nil {
		return err
	}
	if snapshot.Revision > current {
		// Lock/check existence separately. A zero-to-zero snapshot is legitimate,
		// independent of the driver's clientFoundRows setting.
		var commentCount int64
		if table == "post" {
			err = tx.QueryRowContext(ctx, "SELECT comment_count FROM `post` WHERE id = ? FOR UPDATE", behavior.TargetID).Scan(&commentCount)
		} else {
			var id int64
			err = tx.QueryRowContext(ctx, "SELECT id FROM `comment` WHERE id = ? FOR UPDATE", behavior.TargetID).Scan(&id)
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err == nil {
			if table == "post" {
				_, err = tx.ExecContext(ctx, "UPDATE `post` SET like_count = ?, favorite_count = ? WHERE id = ?", snapshot.LikeCount, snapshot.FavoriteCount, behavior.TargetID)
			} else {
				_, err = tx.ExecContext(ctx, "UPDATE `comment` SET like_count = ? WHERE id = ?", snapshot.LikeCount, behavior.TargetID)
			}
			if err != nil {
				return err
			}
			if table == "post" {
				counts, err := poststats.AdvanceTx(ctx, tx, behavior.TargetID)
				if err != nil {
					return err
				}
				now := time.Now().UnixMilli()
				payload, marshalErr := json.Marshal(event.PostEvent{EventID: behavior.EventID, EventTime: now, Type: event.PostEventCounted, PostID: behavior.TargetID, LikeCount: counts.LikeCount, CommentCount: counts.CommentCount, StatsSeq: counts.Sequence})
				if marshalErr != nil {
					return marshalErr
				}
				if err = (&outboxx.SQLStore{}).EnqueueTx(ctx, tx, outboxx.Event{ID: behavior.EventID, Topic: mqx.TopicPostUpdate, Tag: mqx.TagDefault, Key: strconv.FormatInt(behavior.TargetID, 10), Payload: payload}); err != nil {
					return err
				}
			}
		}
		// Missing content is terminal (deleted IDs are never reused), but retain the
		// version fence so an old snapshot cannot revive its projection later.
		if _, err = tx.ExecContext(ctx, "UPDATE content_count_projection SET revision = ? WHERE target_type = ? AND target_id = ?", snapshot.Revision, table, behavior.TargetID); err != nil {
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	s.invalidateCaches(ctx, table, behavior.TargetID)
	return nil
}

func (s *countSyncStore) invalidateCaches(ctx context.Context, targetType string, targetID int64) {
	if s.redis == nil {
		return
	}
	key := postCacheKeyPrefix + strconv.FormatInt(targetID, 10)
	if targetType == "comment" {
		key = commentCacheKeyPrefix + strconv.FormatInt(targetID, 10)
	}
	// One key per command works for Redis Cluster without cross-slot DEL.
	_, _ = s.redis.DelCtx(ctx, key)
	_, _ = s.redis.DelCtx(ctx, cachedstore.Prefix+key)
}
func IsDuplicateKeyError(err error) bool {
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1062
}
