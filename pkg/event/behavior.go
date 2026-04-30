package event

import (
	"fmt"
	"strconv"
)

type BehaviorEvent struct {
	EventID    int64  `json:"event_id"`
	EventTime  int64  `json:"event_time"`
	UserID     int64  `json:"user_id"`
	Action     string `json:"action"`
	TargetID   int64  `json:"target_id"`
	TargetType string `json:"target_type"`
	Duration   int32  `json:"duration"`
	Scene      string `json:"scene"`
	ClientIP   string `json:"client_ip"`
}

func (e *BehaviorEvent) Validate() error {
	if e.UserID <= 0 {
		return fmt.Errorf("user_id is required")
	}
	if e.Action == "" {
		return fmt.Errorf("action is required")
	}
	if e.TargetID <= 0 {
		return fmt.Errorf("target_id is required")
	}
	if e.TargetType == "" {
		return fmt.Errorf("target_type is required")
	}
	return nil
}

func (e *BehaviorEvent) EventIDString() string {
	return strconv.FormatInt(e.EventID, 10)
}
