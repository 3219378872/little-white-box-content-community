package logic

import (
	"encoding/json"
	"time"

	"esx/app/interaction/rpc/internal/model"
	"esx/pkg/event"
	"esx/pkg/mqx"
	"esx/pkg/outboxx"
	"esx/pkg/util"
)

// interactionEventBuilder 返回在互动事务内调用的事件构造器：把互动转换为行为事件，并附上
// 本事务提交后的计数快照，count-sync 据此按序号覆盖内容计数，推荐与行为日志据此更新特征。
func interactionEventBuilder(userID, targetID int64, targetType, action string) model.EventBuilder {
	return func(snapshot model.CountSnapshot) (outboxx.Event, error) {
		eventID, err := util.NextID()
		if err != nil {
			return outboxx.Event{}, err
		}
		interaction := event.InteractionEvent{
			EventID: eventID, EventTime: time.Now().UnixMilli(), UserID: userID,
			Action: action, TargetID: targetID, TargetType: targetType, Scene: "interaction",
		}
		behavior := interaction.ToBehaviorEvent(0)
		// 快照来自与关系状态同一事务的 action_count 行，是下游唯一可信的计数来源。
		behavior.CountSnapshot = &event.InteractionCountSnapshot{
			LikeCount: snapshot.LikeCount, FavoriteCount: snapshot.FavoriteCount, Seq: snapshot.Seq,
		}
		if err := behavior.Validate(); err != nil {
			return outboxx.Event{}, err
		}
		payload, err := json.Marshal(behavior)
		if err != nil {
			return outboxx.Event{}, err
		}
		return outboxx.Event{
			ID: behavior.EventID, Topic: mqx.TopicUserBehaviorV2, Tag: action,
			Key: behavior.EventIDString(), Payload: payload,
		}, nil
	}
}

// targetTypeName 把目标类型编号映射为行为事件中的名称（2=comment，其余为 post）。
func targetTypeName(targetType int32) string {
	if targetType == 2 {
		return "comment"
	}
	return "post"
}
