package logic

import (
	"encoding/json"
	"testing"

	"esx/app/interaction/rpc/internal/model"
	"esx/pkg/event"
	"esx/pkg/mqx"
	"esx/pkg/util"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	_ = util.InitSnowflake(3, 1)
}

func TestInteractionEventBuilderUsesCanonicalV2Topic(t *testing.T) {
	build := interactionEventBuilder(7, 9, "post", event.BehaviorActionLike)
	record, err := build(model.CountSnapshot{LikeCount: 4, FavoriteCount: 2, Seq: 13})

	require.NoError(t, err)
	assert.Equal(t, mqx.TopicUserBehaviorV2, record.Topic)
	assert.Equal(t, event.BehaviorActionLike, record.Tag)
	var behavior event.BehaviorEvent
	require.NoError(t, json.Unmarshal(record.Payload, &behavior))
	assert.Equal(t, int64(7), behavior.UserID)
	assert.Equal(t, int64(9), behavior.TargetID)
	assert.Equal(t, "business-outbox", behavior.Producer)
	// 事务内的计数快照原样写入事件，count-sync 依赖它按序号覆盖。
	assert.Equal(t, &event.InteractionCountSnapshot{LikeCount: 4, FavoriteCount: 2, Seq: 13}, behavior.CountSnapshot)
}

// 无序号的快照无法被下游比较：构造失败，互动事务随之回滚而不是发出不可用的事件。
func TestInteractionEventBuilderRejectsSnapshotWithoutSeq(t *testing.T) {
	build := interactionEventBuilder(7, 9, "post", event.BehaviorActionLike)
	_, err := build(model.CountSnapshot{LikeCount: 1})
	assert.ErrorContains(t, err, "count_snapshot.seq")
}

func TestTargetTypeName(t *testing.T) {
	assert.Equal(t, "post", targetTypeName(1))
	assert.Equal(t, "comment", targetTypeName(2))
}
