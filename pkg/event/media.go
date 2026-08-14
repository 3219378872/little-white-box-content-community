package event

import (
	"encoding/json"
	"fmt"
)

// MediaDeletedEvent 媒体软删事件：S3 删除本身幂等，消费端可安全重试；
// outbox 端到端幂等键（EventID）保证同一删除不重复投递。
type MediaDeletedEvent struct {
	EventID     int64  `json:"event_id,omitempty"`
	EventTime   int64  `json:"event_time,omitempty"`
	MediaID     int64  `json:"media_id"`
	S3ObjectKey string `json:"s3_object_key"`
	Bucket      string `json:"bucket"`
	DeletedAt   int64  `json:"deleted_at"`
}

func (e MediaDeletedEvent) Validate() error {
	if e.MediaID <= 0 {
		return fmt.Errorf("event: media_id is required")
	}
	if e.S3ObjectKey == "" {
		return fmt.Errorf("event: s3_object_key is required")
	}
	return nil
}

// MarshalPayload 返回与 media_cleanup_consumer 解析一致的 JSON 载荷。
func (e MediaDeletedEvent) MarshalPayload() ([]byte, error) {
	return json.Marshal(e)
}
