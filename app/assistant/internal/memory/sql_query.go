package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"esx/pkg/errx"

	sqlx "esx/pkg/sqlstore"
)

// rowQuerier 同时被连接与事务会话满足，让读取 helper 既能在事务外也能在事务内使用。
type rowQuerier interface {
	QueryRowCtx(ctx context.Context, v any, query string, args ...any) error
	QueryRowsCtx(ctx context.Context, v any, query string, args ...any) error
	ExecCtx(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// listActive 读取有效条目；target 为空表示全部目标。
func (s *SQLStore) listActive(ctx context.Context, q rowQuerier, userID int64, target string) ([]Entry, error) {
	return s.listActiveQuery(ctx, q, userID, target, false)
}

// listActiveForUpdate 在事务内加锁读取有效条目，容量与去重判断基于它。
func (s *SQLStore) listActiveForUpdate(ctx context.Context, q rowQuerier, userID int64, target string) ([]Entry, error) {
	return s.listActiveQuery(ctx, q, userID, target, true)
}

// listActiveQuery 按目标与 id 排序读取有效条目，保证展示与容量计算顺序稳定。
func (s *SQLStore) listActiveQuery(ctx context.Context, q rowQuerier, userID int64, target string, forUpdate bool) ([]Entry, error) {
	query := `SELECT id, user_id, target, content, version, created_at_ms, updated_at_ms FROM core_memory_entry WHERE user_id=? AND deleted_at_ms IS NULL`
	args := []any{userID}
	if target != "" {
		query += ` AND target=?`
		args = append(args, target)
	}
	query += ` ORDER BY target ASC, id ASC`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	var rows []struct {
		ID          int64  `db:"id"`
		UserID      int64  `db:"user_id"`
		Target      string `db:"target"`
		Content     string `db:"content"`
		Version     int64  `db:"version"`
		CreatedAtMs int64  `db:"created_at_ms"`
		UpdatedAtMs int64  `db:"updated_at_ms"`
	}
	if err := q.QueryRowsCtx(ctx, &rows, query, args...); err != nil {
		return nil, err
	}
	out := make([]Entry, 0, len(rows))
	for _, row := range rows {
		out = append(out, Entry{ID: row.ID, UserID: row.UserID, Target: row.Target, Content: row.Content,
			Version: int32(row.Version), CreatedAtMs: row.CreatedAtMs, UpdatedAtMs: row.UpdatedAtMs})
	}
	return out, nil
}

// capacities 计算两个目标的已用与上限；读取失败时按 0 占用展示，不阻断列表接口。
func (s *SQLStore) capacities(ctx context.Context, q rowQuerier, userID int64) []Capacity {
	all, err := s.listActive(ctx, q, userID, "")
	if err != nil {
		return []Capacity{
			{Target: TargetMemory, Used: 0, Limit: CapacityMemory},
			{Target: TargetUser, Used: 0, Limit: CapacityUser},
		}
	}
	return []Capacity{
		{Target: TargetMemory, Used: UsedRunes(all, TargetMemory), Limit: CapacityMemory},
		{Target: TargetUser, Used: UsedRunes(all, TargetUser), Limit: CapacityUser},
	}
}

// The bounded active document is the identity authority. content_norm remains
// a legacy prefix for rollback compatibility, not an equality/uniqueness key.
// Read current rows under the target mutation lock, even if an earlier lookup
// established a REPEATABLE READ snapshot before that lock was acquired.
func (s *SQLStore) findByNorm(ctx context.Context, q rowQuerier, userID int64, target, norm string) (*Entry, error) {
	entries, err := s.listActiveForUpdate(ctx, q, userID, target)
	if err != nil {
		return nil, err
	}
	for i := range entries {
		if Normalize(entries[i].Content) == norm {
			return &entries[i], nil
		}
	}
	return nil, nil
}

// getOwned 读取属于该用户且未删除的条目。
func (s *SQLStore) getOwned(ctx context.Context, q rowQuerier, userID, id int64) (*Entry, error) {
	entry, err := s.getOwnedAny(ctx, q, userID, id)
	if err != nil {
		return nil, err
	}
	if entry.Deleted {
		return nil, errx.NewWithCode(errx.NotFound)
	}
	return entry, nil
}

// getOwnedAny 读取属于该用户的条目（含已删除），撤销删除时需要它。
func (s *SQLStore) getOwnedAny(ctx context.Context, q rowQuerier, userID, id int64) (*Entry, error) {
	return s.getOwnedAnyQuery(ctx, q, userID, id, false)
}

// getOwnedAnyForUpdate 是 getOwnedAny 的加锁版本。
func (s *SQLStore) getOwnedAnyForUpdate(ctx context.Context, q rowQuerier, userID, id int64) (*Entry, error) {
	return s.getOwnedAnyQuery(ctx, q, userID, id, true)
}

// getOwnedAnyQuery 按 id 与 user_id 读取条目，他人的条目一律视为不存在。
func (s *SQLStore) getOwnedAnyQuery(ctx context.Context, q rowQuerier, userID, id int64, forUpdate bool) (*Entry, error) {
	var row struct {
		ID          int64         `db:"id"`
		UserID      int64         `db:"user_id"`
		Target      string        `db:"target"`
		Content     string        `db:"content"`
		Version     int64         `db:"version"`
		DeletedAtMs sql.NullInt64 `db:"deleted_at_ms"`
		CreatedAtMs int64         `db:"created_at_ms"`
		UpdatedAtMs int64         `db:"updated_at_ms"`
	}
	query := `SELECT id, user_id, target, content, version, deleted_at_ms, created_at_ms, updated_at_ms
		FROM core_memory_entry WHERE id=? AND user_id=?`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	err := q.QueryRowCtx(ctx, &row, query, id, userID)
	if err == sqlx.ErrNotFound {
		return nil, errx.NewWithCode(errx.NotFound)
	}
	if err != nil {
		return nil, err
	}
	return &Entry{ID: row.ID, UserID: row.UserID, Target: row.Target, Content: row.Content, Version: int32(row.Version),
		CreatedAtMs: row.CreatedAtMs, UpdatedAtMs: row.UpdatedAtMs, Deleted: row.DeletedAtMs.Valid}, nil
}

// encodeEntry 把条目快照序列化进 before_json/after_json；nil 写 NULL。
func encodeEntry(entry *Entry) any {
	if entry == nil {
		return nil
	}
	raw, _ := json.Marshal(entry)
	return string(raw)
}

// decodeEntry 解析条目快照；空值或损坏数据返回 nil。
func decodeEntry(raw string) *Entry {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var entry Entry
	if json.Unmarshal([]byte(raw), &entry) != nil {
		return nil
	}
	return &entry
}

// clipNorm 把规范化内容截到 content_norm 列宽（512 字符）；该列只为兼容回滚保留。
func clipNorm(v string) string {
	runes := []rune(v)
	if len(runes) > 512 {
		return string(runes[:512])
	}
	return v
}
