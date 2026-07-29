package logic

import (
	"encoding/json"
	"time"

	"esx/pkg/event"
	"esx/pkg/outboxx"
	"mqx"
	"util"
)

func interactionOutboxEvent(userID, targetID int64, targetType, action string) (outboxx.Event, error) {
	eventID, err := util.NextID()
	if err != nil {
		return outboxx.Event{}, err
	}
	interaction := event.InteractionEvent{
		EventID: eventID, EventTime: time.Now().UnixMilli(), UserID: userID,
		Action: action, TargetID: targetID, TargetType: targetType, Scene: "interaction",
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
		ID: behavior.EventID, Topic: mqx.TopicUserBehaviorV2, Tag: action,
		Key: behavior.EventIDString(), Payload: payload,
	}, nil
}

func targetTypeName(targetType int32) string {
	if targetType == 2 {
		return "comment"
	}
	return "post"
}
