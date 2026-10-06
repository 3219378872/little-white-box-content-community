package logic

import (
	"encoding/json"
	"strconv"
	"time"

	"esx/pkg/event"
	"esx/pkg/mqx"
	"esx/pkg/outboxx"
	"esx/pkg/util"
)

const postEventExcerptRunes = 256

// runePrefix 按字符截取前 n 个字符，避免切断多字节字符。
func runePrefix(s string, n int) string {
	if n <= 0 || s == "" {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n])
}

// buildPostOutboxEvent 补齐事件 ID 与时间并校验后封装为 outbox 事件；以帖子 ID 作为消息 Key，同一帖子的事件按序投递。
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
		Key: strconv.FormatInt(e.PostID, 10), Payload: body,
	}, nil
}
