package logic

import (
	"encoding/json"
	"time"

	"esx/pkg/event"
	"esx/pkg/mqx"
	"esx/pkg/outboxx"
	"esx/pkg/util"
)

// buildBusinessBehaviorOutbox 把内容侧互动（如评论）转换为行为事件并封装为 outbox 事件，缺省的 ID 与时间就地补齐。
func buildBusinessBehaviorOutbox(interaction event.InteractionEvent) (outboxx.Event, error) {
	if interaction.EventID == 0 {
		id, err := util.NextID()
		if err != nil {
			return outboxx.Event{}, err
		}
		interaction.EventID = id
	}
	if interaction.EventTime == 0 {
		interaction.EventTime = time.Now().UnixMilli()
	}
	behavior := interaction.ToBehaviorEvent(0)
	if err := behavior.Validate(); err != nil {
		return outboxx.Event{}, err
	}
	payload, err := json.Marshal(behavior)
	if err != nil {
		return outboxx.Event{}, err
	}
	return outboxx.Event{
		ID: behavior.EventID, Topic: mqx.TopicUserBehaviorV2, Tag: behavior.Action,
		Key: behavior.EventIDString(), Payload: payload,
	}, nil
}
