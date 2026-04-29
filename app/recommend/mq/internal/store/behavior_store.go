package store

import "context"

// BehaviorEvent is a generic user behavior event.
type BehaviorEvent struct {
	UserID     int64  `json:"user_id"`
	Action     string `json:"action"`
	TargetID   int64  `json:"target_id"`
	TargetType string `json:"target_type"`
}

// BehaviorStore is the future profile/feature write interface.
type BehaviorStore interface {
	Record(ctx context.Context, event BehaviorEvent) error
}

// NoopBehaviorStore is the default no-op implementation.
type NoopBehaviorStore struct{}

func (n *NoopBehaviorStore) Record(ctx context.Context, event BehaviorEvent) error { return nil }
