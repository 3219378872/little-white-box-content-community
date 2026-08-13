package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"esx/pkg/event"
)

type ClickHouseStore struct {
	db *sql.DB
}

func NewClickHouseStore(db *sql.DB) *ClickHouseStore {
	return &ClickHouseStore{db: db}
}

func (s *ClickHouseStore) Insert(ctx context.Context, behavior event.BehaviorEvent) (err error) {
	startedAt := time.Now()
	defer func() { observeClickHouseWrite(startedAt, "behavior", err) }()
	if validationErr := behavior.Validate(); validationErr != nil {
		err = fmt.Errorf("validate behavior event: %w", validationErr)
		return err
	}
	// REL-021：完整客户端 IP 不得写入行为分析表；只存 SHA-256 单向哈希。
	behavior.ClientIP = anonymizeIP(behavior.ClientIP)

	query := `INSERT INTO xbh_analytics.behavior_events (
		event_id, client_event_id, schema_version, event_time, received_at,
		user_id, anonymous_id, session_id, request_id, action, target_id,
		target_type, scene, position, duration_ms, recall_source, model_version,
		experiment_id, producer, client_ip, client_version
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err = s.db.ExecContext(ctx, query,
		behavior.EventID, behavior.ClientEventID, behavior.SchemaVersion,
		time.UnixMilli(behavior.EventTime), time.UnixMilli(behavior.ReceivedAt),
		behavior.UserID, behavior.AnonymousID, behavior.SessionID, behavior.RequestID,
		behavior.Action, behavior.TargetID, behavior.TargetType, behavior.Scene,
		nullableInt32(behavior.Position), nullableInt64(behavior.DurationMs),
		behavior.RecallSource, behavior.ModelVersion, behavior.ExperimentID,
		behavior.Producer, behavior.ClientIP, behavior.ClientVersion,
	)
	if err != nil {
		return fmt.Errorf("insert behavior_events: %w", err)
	}
	return nil
}

// anonymizeIP 将客户端 IP 转成 SHA-256 十六进制摘要；空值保持为空。
// 摘要不可逆，且不会将完整 IP 持久化到行为分析表（REL-021）。
func anonymizeIP(ip string) string {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return ""
	}
	digest := sha256.Sum256([]byte(ip))
	return hex.EncodeToString(digest[:])
}

type DeadLetter struct {
	MessageID  string
	EventID    int64
	Payload    []byte
	Error      string
	ReceivedAt int64
}

func (s *ClickHouseStore) InsertDeadLetter(ctx context.Context, letter DeadLetter) (err error) {
	startedAt := time.Now()
	defer func() { observeClickHouseWrite(startedAt, "dead_letter", err) }()
	receivedAt := time.Now()
	if letter.ReceivedAt > 0 {
		receivedAt = time.UnixMilli(letter.ReceivedAt)
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO xbh_analytics.behavior_dead_letters
		(message_id, event_id, payload, error, received_at) VALUES (?, ?, ?, ?, ?)`,
		letter.MessageID, letter.EventID, string(letter.Payload), letter.Error, receivedAt)
	if err != nil {
		return fmt.Errorf("insert behavior_dead_letters: %w", err)
	}
	return nil
}

func nullableInt32(value *int32) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableInt64(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}

// AggregateDaily 把指定日期窗口内的原始行为按 (date,user_id,action,target_type)
// 聚合进 daily_aggregates（REL-020：去标识聚合结果保留 365 天）。
// 读取 behavior_events FINAL（按 event_id 去重收敛），目标表用
// ReplacingMergeTree(aggregated_at)，重复执行幂等、不重复累计。
// 返回窗口内聚合行的数量。
func (s *ClickHouseStore) AggregateDaily(ctx context.Context, from, to time.Time) (int64, error) {
	if s == nil || s.db == nil {
		return 0, fmt.Errorf("aggregate daily: store is nil")
	}
	if !from.Before(to) {
		return 0, fmt.Errorf("aggregate daily: invalid window [%s, %s)", from.Format(time.DateOnly), to.Format(time.DateOnly))
	}
	const query = `
INSERT INTO xbh_analytics.daily_aggregates (date, user_id, action, target_type, cnt, aggregated_at)
SELECT toDate(event_time) AS date, user_id, action, target_type, count() AS cnt, now64(3)
FROM xbh_analytics.behavior_events FINAL
WHERE toDate(event_time) >= ? AND toDate(event_time) < ?
GROUP BY date, user_id, action, target_type`
	if _, err := s.db.ExecContext(ctx, query, from.Format("2006-01-02"), to.Format("2006-01-02")); err != nil {
		return 0, fmt.Errorf("aggregate daily: %w", err)
	}
	var count int64
	if err := s.db.QueryRowContext(ctx,
		`SELECT count() FROM xbh_analytics.daily_aggregates FINAL WHERE date >= ? AND date < ?`,
		from.Format("2006-01-02"), to.Format("2006-01-02")).Scan(&count); err != nil {
		return 0, fmt.Errorf("aggregate daily count: %w", err)
	}
	return count, nil
}
