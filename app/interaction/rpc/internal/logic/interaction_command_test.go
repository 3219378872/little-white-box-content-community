package logic

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"esx/app/interaction/rpc/internal/model"
	"esx/app/interaction/rpc/internal/svc"
	"esx/pkg/event"
	"esx/pkg/outboxx"

	"github.com/stretchr/testify/require"
)

// countKey 标识 action_count 中的一行。
type countKey struct {
	targetID, targetType int64
}

// fakeInteractionCommands 用记录模型 mock 模拟关系状态变更，用内存计数模拟 action_count 与 count_seq，
// 并像真实事务一样在计数变化后用快照调用事件构造器，测试据此检查计数与事件。
type fakeInteractionCommands struct {
	service *svc.ServiceContext
	counts  map[countKey]model.CountSnapshot
	// countErr 模拟计数写入失败，此时不产生事件。
	countErr error
	events   []outboxx.Event
}

// fakeInteractionCommandsFor 创建以 service 中模型 mock 驱动状态变更的命令 fake。
func fakeInteractionCommandsFor(service *svc.ServiceContext) *fakeInteractionCommands {
	return &fakeInteractionCommands{service: service, counts: map[countKey]model.CountSnapshot{}}
}

// adjust 按增量调整内存计数（不低于 0）并递增序号，再用快照构造事件，对应真实事务的 upsert + 入队。
func (c *fakeInteractionCommands) adjust(targetID, targetType, likeDelta, favoriteDelta int64, build model.EventBuilder) error {
	if c.countErr != nil {
		return c.countErr
	}
	key := countKey{targetID: targetID, targetType: targetType}
	snapshot := c.counts[key]
	snapshot.LikeCount = max(snapshot.LikeCount+likeDelta, 0)
	snapshot.FavoriteCount = max(snapshot.FavoriteCount+favoriteDelta, 0)
	snapshot.Seq++
	c.counts[key] = snapshot
	ev, err := build(snapshot)
	if err != nil {
		return err
	}
	c.events = append(c.events, ev)
	return nil
}

// lastSnapshot 解码最后一个事件携带的计数快照。
func (c *fakeInteractionCommands) lastSnapshot(t *testing.T) event.InteractionCountSnapshot {
	t.Helper()
	require.NotEmpty(t, c.events, "expected an interaction event")
	var behavior event.BehaviorEvent
	require.NoError(t, json.Unmarshal(c.events[len(c.events)-1].Payload, &behavior))
	require.NotNil(t, behavior.CountSnapshot, "interaction event must carry a count snapshot")
	return *behavior.CountSnapshot
}

// Like 用点赞记录 mock 判断状态是否变化，变化时点赞数加一并产生事件。
func (c *fakeInteractionCommands) Like(
	ctx context.Context,
	userID, targetID, targetType int64,
	build model.EventBuilder,
) (int64, error) {
	result, id, err := c.service.LikeRecordModel.UpsertLikeStatusTx(
		ctx, c.service.Conn, userID, targetID, targetType, model.StatusActive,
	)
	if err != nil {
		return 0, err
	}
	if result == nil {
		return 0, errors.New("nil like result")
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if changed == 0 {
		return 0, model.ErrNoStateChange
	}
	if err := c.adjust(targetID, targetType, 1, 0, build); err != nil {
		return 0, err
	}
	return id, nil
}

// Unlike 用条件更新 mock 判断状态是否变化，变化时点赞数减一并产生事件。
func (c *fakeInteractionCommands) Unlike(
	ctx context.Context,
	recordID, targetID, targetType int64,
	build model.EventBuilder,
) error {
	result, err := c.service.LikeRecordModel.UpdateStatusById(
		ctx, recordID, model.StatusActive, model.StatusInactive,
	)
	if err != nil {
		return err
	}
	if result == nil {
		return errors.New("nil unlike result")
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed == 0 {
		return model.ErrNoStateChange
	}
	return c.adjust(targetID, targetType, -1, 0, build)
}

// Favorite 用收藏记录 mock 判断状态是否变化，变化时收藏数加一并产生事件。
func (c *fakeInteractionCommands) Favorite(
	ctx context.Context,
	userID, postID int64,
	build model.EventBuilder,
) (int64, error) {
	result, err := c.service.FavoriteModel.UpsertFavoriteStatus(ctx, userID, postID, model.StatusActive)
	if err != nil {
		return 0, err
	}
	if result == nil {
		return 0, errors.New("nil favorite result")
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if changed == 0 {
		return 0, model.ErrNoStateChange
	}
	if err := c.adjust(postID, 1, 0, 1, build); err != nil {
		return 0, err
	}
	return 1, nil
}

// Unfavorite 用条件更新 mock 判断状态是否变化，变化时收藏数减一并产生事件。
func (c *fakeInteractionCommands) Unfavorite(
	ctx context.Context,
	recordID, postID int64,
	build model.EventBuilder,
) error {
	result, err := c.service.FavoriteModel.UpdateStatusById(
		ctx, recordID, model.StatusActive, model.StatusInactive,
	)
	if err != nil {
		return err
	}
	if result == nil {
		return errors.New("nil unfavorite result")
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed == 0 {
		return model.ErrNoStateChange
	}
	return c.adjust(postID, 1, 0, -1, build)
}
