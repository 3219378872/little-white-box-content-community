package logic

import (
	"encoding/json"
	"time"

	"esx/pkg/event"
	"esx/pkg/mqx"
	"esx/pkg/outboxx"
	"esx/pkg/util"
)

func followOutboxEvent(userID, targetUserID int64, action string) (outboxx.Event, error) {
	eventID, err := util.NextID()
	if err != nil {
		return outboxx.Event{}, err
	}
	behavior := (&event.InteractionEvent{
		EventID: eventID, EventTime: time.Now().UnixMilli(), UserID: userID,
		Action: action, TargetID: targetUserID, TargetType: "user", Scene: "profile",
	}).ToBehaviorEvent(0)
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
