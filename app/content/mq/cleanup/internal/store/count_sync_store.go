package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"esx/pkg/event"
	"esx/pkg/mqx"
	"esx/pkg/outboxx"

	sqlx "esx/pkg/sqlstore"
)

const (
	postCacheKeyPrefix    = "cache:post:id:"
	commentCacheKeyPrefix = "cache:comment:id:"
)

// CountSyncStore 把 Interaction 权威计数快照投影到内容计数列（CORE-032、REL-008）。
// 投影按 action_count 序号单调覆盖：重复投递与乱序到达都只会被更大的序号推进，无需事件去重。
type CountSyncStore interface {
	// ApplyBehaviorCount 返回快照是否推进了投影；旧序号、重复事件或目标不存在时为 false。
	ApplyBehaviorCount(ctx context.Context, behavior event.BehaviorEvent) (bool, error)
}

// RedisCmdable 是计数同步用到的 Redis 子集：投影推进后失效详情缓存。
type RedisCmdable interface {
	DelCtx(ctx context.Context, keys ...string) (int, error)
}

// CountSyncConn 是计数同步用到的数据库子集：评论计数单条更新，帖子计数与 counted 事件同事务。
type CountSyncConn interface {
	ExecCtx(ctx context.Context, query string, args ...any) (sql.Result, error)
	TransactCtx(ctx context.Context, fn func(context.Context, sqlx.Session) error) error
}

// countSyncStore 用 MySQL 保存计数投影，用 Redis 失效详情缓存。
type countSyncStore struct {
	conn  CountSyncConn
	redis RedisCmdable
	// now 生成 stats_seq 的时间下限，测试可替换。
	now func() time.Time
}

// NewCountSyncStore 创建计数同步存储。
func NewCountSyncStore(conn CountSyncConn, redisClient RedisCmdable) CountSyncStore {
	return &countSyncStore{conn: conn, redis: redisClient, now: time.Now}
}

// ApplyBehaviorCount 用事件携带的计数快照覆盖目标计数；非计数类行为直接忽略。
// 缺少快照、目标类型未知等重试也无法处理的情况返回永久错误，由消费者确认丢弃。
func (s *countSyncStore) ApplyBehaviorCount(ctx context.Context, behavior event.BehaviorEvent) (bool, error) {
	if err := behavior.Validate(); err != nil {
		return false, mqx.ErrPermanentEvent(fmt.Sprintf("count-sync: invalid behavior event: %v", err))
	}
	if !isCountAction(behavior.Action) {
		return false, nil
	}
	// 只信任权威事务给出的绝对计数；没有快照的旧格式事件无法安全地按序覆盖。
	snapshot := behavior.CountSnapshot
	if snapshot == nil {
		return false, mqx.ErrPermanentEvent("count-sync: behavior event has no count snapshot")
	}

	var applied bool
	var err error
	switch behavior.TargetType {
	case "post":
		applied, err = s.applyPostSnapshot(ctx, behavior.TargetID, behavior.EventID, *snapshot)
	case "comment":
		applied, err = s.applyCommentSnapshot(ctx, behavior.TargetID, *snapshot)
	default:
		return false, mqx.ErrPermanentEvent(fmt.Sprintf("count-sync: unsupported target type %q", behavior.TargetType))
	}
	if err != nil {
		return false, err
	}
	// 只有投影真正推进时详情缓存才可能过期。
	if applied {
		s.invalidateCaches(ctx, behavior.TargetType, behavior.TargetID)
	}
	return applied, nil
}

// isCountAction 判断行为是否影响点赞/收藏计数。
func isCountAction(action string) bool {
	switch action {
	case event.BehaviorActionLike, event.BehaviorActionUnlike,
		event.BehaviorActionFavorite, event.BehaviorActionUnfavorite:
		return true
	default:
		return false
	}
}

// 帖子同时投影点赞数与收藏数：同一 action_count 行的快照总是两者一起有效。
// 序号条件让旧快照与重复投递影响 0 行；stats_seq 取 GREATEST(stats_seq+1, 当前毫秒)，
// 在行锁内单调递增，并与搜索中已有的时间戳序号兼容。
const (
	applyPostSnapshotSQL = "UPDATE `post` SET `like_count` = ?, `favorite_count` = ?, `interaction_seq` = ?, " +
		"`stats_seq` = GREATEST(`stats_seq` + 1, ?) WHERE `id` = ? AND `interaction_seq` < ?"
	postCountsSQL           = "SELECT `like_count`, `comment_count`, `stats_seq` FROM `post` WHERE `id` = ? LIMIT 1"
	applyCommentSnapshotSQL = "UPDATE `comment` SET `like_count` = ?, `interaction_seq` = ? " +
		"WHERE `id` = ? AND `interaction_seq` < ?"
)

// postCounts 是写入 counted 事件所需的帖子计数与快照序号。
type postCounts struct {
	LikeCount    int64 `db:"like_count"`
	CommentCount int64 `db:"comment_count"`
	StatsSeq     int64 `db:"stats_seq"`
}

// applyPostSnapshot 在一个事务内覆盖帖子计数，并写入带最新计数的 counted 事件到 outbox，
// 搜索等下游据此局部更新计数。旧序号或帖子不存在时不写任何内容。
func (s *countSyncStore) applyPostSnapshot(
	ctx context.Context,
	postID, eventID int64,
	snapshot event.InteractionCountSnapshot,
) (bool, error) {
	applied := false
	err := s.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		result, err := session.ExecCtx(ctx, applyPostSnapshotSQL,
			snapshot.LikeCount, snapshot.FavoriteCount, snapshot.Seq, s.now().UnixMilli(), postID, snapshot.Seq)
		if err != nil {
			return err
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return err
		}
		// 0 行：已投影了相同或更新的快照（重复/乱序），或帖子不存在；两者都无需再处理。
		if changed == 0 {
			return nil
		}
		applied = true
		// 读回本事务刚写的计数与 stats_seq，评论数取帖子当前值一并下发。
		var counts postCounts
		if err := session.QueryRowCtx(ctx, &counts, postCountsSQL, postID); err != nil {
			return err
		}
		payload, err := json.Marshal(event.PostEvent{
			EventID: eventID, EventTime: s.now().UnixMilli(), Type: event.PostEventCounted, PostID: postID,
			LikeCount: counts.LikeCount, CommentCount: counts.CommentCount, StatsSeq: counts.StatsSeq,
		})
		if err != nil {
			return err
		}
		// 收藏变化也要下发：它的快照可能顺带修正了此前被判为旧序号而跳过的点赞数。
		// outbox ID 复用行为事件 ID：序号守卫已保证同一事件只会推进一次。
		return (&outboxx.SQLStore{}).Enqueue(ctx, session, outboxx.Event{
			ID: eventID, Topic: mqx.TopicPostUpdate, Tag: mqx.TagDefault,
			Key: strconv.FormatInt(postID, 10), Payload: payload,
		})
	})
	if err != nil {
		return false, err
	}
	return applied, nil
}

// applyCommentSnapshot 覆盖评论点赞数；评论不支持收藏，快照中的收藏数忽略。
func (s *countSyncStore) applyCommentSnapshot(
	ctx context.Context,
	commentID int64,
	snapshot event.InteractionCountSnapshot,
) (bool, error) {
	result, err := s.conn.ExecCtx(ctx, applyCommentSnapshotSQL,
		snapshot.LikeCount, snapshot.Seq, commentID, snapshot.Seq)
	if err != nil {
		return false, err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return changed > 0, nil
}

// invalidateCaches 删除被调整对象的详情缓存，下次读取回源数据库。
func (s *countSyncStore) invalidateCaches(ctx context.Context, targetType string, targetID int64) {
	key := postCacheKeyPrefix + strconv.FormatInt(targetID, 10)
	if targetType == "comment" {
		key = commentCacheKeyPrefix + strconv.FormatInt(targetID, 10)
	}
	// 缓存失效失败不改变已提交的计数（CORE-053），缓存随 TTL 自然过期。
	_, _ = s.redis.DelCtx(ctx, key)
}
