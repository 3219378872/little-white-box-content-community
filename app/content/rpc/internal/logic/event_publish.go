package logic

import (
	"encoding/json"
	"strconv"
	"time"

	"esx/pkg/event"
	"esx/pkg/outboxx"
	"mqx"
	"util"
)

func buildPostOutboxEvent(topic string, e event.PostEvent) (outboxx.Event, error) {
	if e.EventID == 0 {
		id, err := util.NextID()
		if err != nil {
			return outboxx.Event{}, err
		}
		e.EventID = id
	}
	if e.EventTime == 0 {
		e.EventTime = time.Now().UnixMilli()
	}
	if err := e.Validate(); err != nil {
		return outboxx.Event{}, err
	}
	body, err := json.Marshal(e)
	if err != nil {
		return outboxx.Event{}, err
	}
	return outboxx.Event{
		ID: e.EventID, Topic: topic, Tag: mqx.TagDefault,
		Key: strconv.FormatInt(e.EventID, 10), Payload: body,
	}, nil
}
