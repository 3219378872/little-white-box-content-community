package event

import "fmt"

// PostEventType 标识帖子生命周期事件类型，用作 RocketMQ Tag 或单独 topic 区分。
type PostEventType string

const (
	PostEventCreated PostEventType = "post.created"
	PostEventUpdated PostEventType = "post.updated"
	PostEventDeleted PostEventType = "post.deleted"
)

// PostEvent 是 search-index / embedding / content-cleanup / feed-fanout 等 L1 消费者
// 共享的帖子事件载荷。字段尽量保持稳定，避免因下游不同消费方各自定义 schema。
type PostEvent struct {
	EventID     int64         `json:"event_id"`
	EventTime   int64         `json:"event_time"` // Unix ms
	Type        PostEventType `json:"type"`
	PostID      int64         `json:"post_id"`
	AuthorID    int64         `json:"author_id"`
	Title       string        `json:"title,omitempty"`
	BodyExcerpt string        `json:"body_excerpt,omitempty"`
	CategoryID  int64         `json:"category_id,omitempty"`
	Tags        []string      `json:"tags,omitempty"`
}

func (e *PostEvent) Validate() error {
	if e.EventID <= 0 {
		return fmt.Errorf("event_id is required")
	}
	if e.PostID <= 0 {
		return fmt.Errorf("post_id is required")
	}
	if e.Type == "" {
		return fmt.Errorf("type is required")
	}
	switch e.Type {
	case PostEventCreated, PostEventUpdated, PostEventDeleted:
	default:
		return fmt.Errorf("unknown post event type: %s", e.Type)
	}
	if e.Type != PostEventDeleted && e.AuthorID <= 0 {
		return fmt.Errorf("author_id is required for non-delete events")
	}
	return nil
}

// InteractionEvent 描述用户对内容/用户的互动。
// 用作 like/unlike/favorite/unfavorite/comment-create/user-follow 的统一载荷，
// 由 Interaction RPC、Content RPC、User RPC 各自发出，behavior-log/content-stat/user-feature 消费。
type InteractionEvent struct {
	EventID    int64  `json:"event_id"`
	EventTime  int64  `json:"event_time"` // Unix ms
	UserID     int64  `json:"user_id"`
	Action     string `json:"action"` // like / unlike / favorite / unfavorite / comment / follow / unfollow
	TargetID   int64  `json:"target_id"`
	TargetType string `json:"target_type"` // post / user / tag
	Scene      string `json:"scene,omitempty"`
	ClientIP   string `json:"client_ip,omitempty"`
}

func (e *InteractionEvent) Validate() error {
	if e.EventID <= 0 {
		return fmt.Errorf("event_id is required")
	}
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

// ToBehaviorEvent 把交互事件转成 ClickHouse 落库的 BehaviorEvent。
// pipeline/behaviorlog 用这个适配；保持两个 struct 解耦，业务事件和分析事件可独立演进。
func (e *InteractionEvent) ToBehaviorEvent(duration int32) BehaviorEvent {
	return BehaviorEvent{
		EventID:    e.EventID,
		EventTime:  e.EventTime,
		UserID:     e.UserID,
		Action:     e.Action,
		TargetID:   e.TargetID,
		TargetType: e.TargetType,
		Duration:   duration,
		Scene:      e.Scene,
		ClientIP:   e.ClientIP,
	}
}
