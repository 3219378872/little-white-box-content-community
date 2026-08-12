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
