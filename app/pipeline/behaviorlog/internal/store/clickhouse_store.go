package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"esx/pkg/event"
)

type BehaviorStore interface {
	Insert(ctx context.Context, e event.BehaviorEvent) error
}

type ClickHouseStore struct {
	db *sql.DB
}

func NewClickHouseStore(db *sql.DB) *ClickHouseStore {
	return &ClickHouseStore{db: db}
}

func (s *ClickHouseStore) Insert(ctx context.Context, e event.BehaviorEvent) error {
	if err := e.Validate(); err != nil {
		return fmt.Errorf("validate behavior event: %w", err)
	}

	eventTime := time.UnixMilli(e.EventTime)
	if e.EventTime == 0 {
		eventTime = time.Now()
	}

	query := `INSERT INTO xbh_analytics.behavior_events
		(event_id, event_time, user_id, action, target_id, target_type, duration, scene, client_ip)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := s.db.ExecContext(ctx, query,
		e.EventID, eventTime, e.UserID, e.Action,
		e.TargetID, e.TargetType, e.Duration, e.Scene, e.ClientIP)
	if err != nil {
		return fmt.Errorf("insert behavior_events: %w", err)
	}

	return nil
}
