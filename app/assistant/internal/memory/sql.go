package memory

import (
	"context"

	"esx/pkg/errx"

	sqlx "esx/pkg/sqlstore"
)

// SQLStore 是核心记忆的 MySQL 实现：条目在 core_memory_entry，每次变更追加到 memory_change 以支持幂等重放与撤销。
type SQLStore struct {
	conn    sqlx.SqlConn
	Scanner Scanner
}

// NewSQLStore 绑定连接与写入前的内容安全扫描器。
func NewSQLStore(conn sqlx.SqlConn, scanner Scanner) *SQLStore {
	return &SQLStore{conn: conn, Scanner: scanner}
}

// List 返回目标下的有效条目与两个目标的容量占用，供记忆面板展示。
func (s *SQLStore) List(ctx context.Context, userID int64, target string) ([]Entry, []Capacity, error) {
	if userID <= 0 {
		return nil, nil, errx.NewWithCode(errx.LoginRequired)
	}
	entries, err := s.listActive(ctx, s.conn, userID, target)
	if err != nil {
		return nil, nil, err
	}
	return entries, s.capacities(ctx, s.conn, userID), nil
}

// Active 返回用户全部有效条目，供组装对话上下文。
func (s *SQLStore) Active(ctx context.Context, userID int64) ([]Entry, error) {
	return s.listActive(ctx, s.conn, userID, "")
}

// RecordFeedback 记录用户对推荐结果的反馈原因。
func (s *SQLStore) RecordFeedback(ctx context.Context, userID int64, requestID string, postID int64, reason string) error {
	_, err := s.conn.ExecCtx(ctx, `INSERT INTO recommendation_feedback (user_id, request_id, post_id, reason) VALUES (?, ?, ?, ?)`,
		userID, requestID, postID, reason)
	return err
}
