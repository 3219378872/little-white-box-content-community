package memory

import (
	"context"
	"database/sql"
	"sort"
	"strings"
	"unicode/utf8"

	"esx/pkg/errx"

	sqlx "esx/pkg/sqlstore"
)

// Add 新增一条记忆；内容与已有条目规范化后相同时返回已有条目且不产生变更记录。
func (s *SQLStore) Add(ctx context.Context, userID int64, target, content, requestID string, nowMs int64) (Entry, int64, error) {
	entries, ids, err := s.mutate(ctx, userID, requestID, []Op{{Op: OpAdd, Target: target, Content: content}}, nowMs)
	if err != nil {
		return Entry{}, 0, err
	}
	if len(entries) == 0 {
		return Entry{}, 0, errx.NewWithCode(errx.SystemError)
	}
	changeID := int64(0)
	if len(ids) > 0 {
		changeID = ids[0]
	}
	return entries[0], changeID, nil
}

// Replace 按版本号 CAS 改写一条记忆；返回的 changeID 用于撤销。
func (s *SQLStore) Replace(ctx context.Context, userID, id int64, content string, version int32, requestID string, nowMs int64) (Entry, int64, error) {
	entries, ids, err := s.mutate(ctx, userID, requestID, []Op{{Op: OpReplace, ID: id, Content: content, Version: version}}, nowMs)
	if err != nil {
		return Entry{}, 0, err
	}
	changeID := int64(0)
	if len(ids) > 0 {
		changeID = ids[0]
	}
	if len(entries) == 0 {
		return Entry{}, changeID, nil
	}
	return entries[0], changeID, nil
}

// Remove 按版本号 CAS 软删除一条记忆，返回变更 ID。
func (s *SQLStore) Remove(ctx context.Context, userID, id int64, version int32, requestID string, nowMs int64) (int64, error) {
	_, ids, err := s.mutate(ctx, userID, requestID, []Op{{Op: OpRemove, ID: id, Version: version}}, nowMs)
	if err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}
	return ids[0], nil
}

// Batch 在一个事务内依次应用多条操作，任一失败整体回滚。
func (s *SQLStore) Batch(ctx context.Context, userID int64, requestID string, ops []Op, nowMs int64) ([]Entry, []int64, error) {
	return s.mutate(ctx, userID, requestID, ops, nowMs)
}

// mutate 是所有写操作的事务入口：先按目标加锁再逐条应用；事务失败时若同一 requestID
// 已被另一次提交写入，则返回那次结果，让并发重试也能幂等收敛。
func (s *SQLStore) mutate(ctx context.Context, userID int64, requestID string, ops []Op, nowMs int64) ([]Entry, []int64, error) {
	if userID <= 0 {
		return nil, nil, errx.NewWithCode(errx.LoginRequired)
	}
	if len(ops) == 0 {
		return nil, nil, errx.NewWithCode(errx.ParamError)
	}
	var entries []Entry
	var changeIDs []int64
	err := s.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		if err := s.lockMutationTargets(ctx, session, userID, ops); err != nil {
			return err
		}
		out, ids, err := s.applyOps(ctx, session, userID, requestID, ops, nowMs)
		entries, changeIDs = out, ids
		return err
	})
	if err != nil && requestID != "" {
		if replayed, ids, found, replayErr := s.replayOps(ctx, s.conn, userID, requestID, ops); replayErr != nil {
			return nil, nil, replayErr
		} else if found {
			return replayed, ids, nil
		}
	}
	return entries, changeIDs, err
}

// lockMutationTargets 收集本批操作涉及的全部目标并统一加锁，容量与去重校验因此在锁内进行。
func (s *SQLStore) lockMutationTargets(ctx context.Context, session sqlx.Session, userID int64, ops []Op) error {
	targets := make(map[string]struct{}, 2)
	for _, op := range ops {
		switch strings.ToLower(strings.TrimSpace(op.Op)) {
		case OpAdd, "":
			if !ValidTarget(op.Target) {
				return errx.New(errx.ParamError, "memory target must be memory or user")
			}
			targets[op.Target] = struct{}{}
		case OpReplace, OpRemove:
			entry, err := s.getOwnedAny(ctx, session, userID, op.ID)
			if err != nil {
				return err
			}
			targets[entry.Target] = struct{}{}
		default:
			return errx.New(errx.ParamError, "unknown memory op")
		}
	}
	ordered := make([]string, 0, len(targets))
	for target := range targets {
		ordered = append(ordered, target)
	}
	return s.lockTargets(ctx, session, userID, ordered...)
}

// lockTargets 按固定顺序给目标加行锁，避免两个事务交叉加锁导致死锁。
func (s *SQLStore) lockTargets(ctx context.Context, session sqlx.Session, userID int64, targets ...string) error {
	sort.Strings(targets)
	for _, target := range targets {
		// A no-op duplicate update takes the row's exclusive lock directly. Using
		// INSERT IGNORE followed by FOR UPDATE permits two shared-lock holders to
		// deadlock while both try to upgrade.
		if _, err := session.ExecCtx(ctx, `INSERT INTO memory_target_lock (user_id, target) VALUES (?, ?)
			ON DUPLICATE KEY UPDATE target=VALUES(target)`, userID, target); err != nil {
			return err
		}
	}
	return nil
}

// replayOps 在事务外查找本批每条操作已提交的变更；全部找到才视为重放成功，
// 任一缺失返回 found=false，参数不一致则报幂等冲突。
func (s *SQLStore) replayOps(
	ctx context.Context,
	q rowQuerier,
	userID int64,
	requestID string,
	ops []Op,
) ([]Entry, []int64, bool, error) {
	entries := make([]Entry, 0, len(ops))
	changeIDs := make([]int64, 0, len(ops))
	for i, op := range ops {
		change, err := s.findChangeByRequest(ctx, q, userID, opRequestID(requestID, i, len(ops)))
		if err != nil {
			return nil, nil, false, err
		}
		if change == nil {
			return nil, nil, false, nil
		}
		if !memoryReplayMatches(op, *change) {
			return nil, nil, false, errx.NewWithCode(errx.IdempotencyConflict)
		}
		if change.After != nil && !strings.EqualFold(strings.TrimSpace(op.Op), OpRemove) {
			entries = append(entries, *change.After)
		}
		changeIDs = append(changeIDs, change.ID)
	}
	return entries, changeIDs, true, nil
}

// applyOps 在已加锁的事务内逐条应用操作；无 requestID 的调用用 "anon" 占位，不参与重放。
func (s *SQLStore) applyOps(ctx context.Context, session sqlx.Session, userID int64, requestID string, ops []Op, nowMs int64) ([]Entry, []int64, error) {
	if requestID == "" {
		requestID = "anon"
	}
	entries := make([]Entry, 0, len(ops))
	changeIDs := make([]int64, 0, len(ops))
	for i, op := range ops {
		entry, changeID, err := s.applyOne(ctx, session, userID, opRequestID(requestID, i, len(ops)), op, nowMs)
		if err != nil {
			return nil, nil, err
		}
		if entry != nil {
			entries = append(entries, *entry)
		}
		if changeID > 0 {
			changeIDs = append(changeIDs, changeID)
		}
	}
	return entries, changeIDs, nil
}

// applyOne 先按 requestID 尝试幂等重放，未命中时再按操作类型执行。
func (s *SQLStore) applyOne(ctx context.Context, session sqlx.Session, userID int64, requestID string, op Op, nowMs int64) (*Entry, int64, error) {
	if requestID != "" && requestID != "anon" {
		change, err := s.findChangeByRequest(ctx, session, userID, requestID)
		if err != nil {
			return nil, 0, err
		}
		if change != nil {
			return replayResult(op, *change)
		}
	}
	switch strings.ToLower(strings.TrimSpace(op.Op)) {
	case OpAdd, "":
		return s.addOne(ctx, session, userID, op.Target, op.Content, requestID, nowMs)
	case OpReplace:
		return s.replaceOne(ctx, session, userID, op.ID, op.Content, op.Version, requestID, nowMs)
	case OpRemove:
		_, changeID, err := s.removeOne(ctx, session, userID, op.ID, op.Version, requestID, nowMs)
		return nil, changeID, err
	default:
		return nil, 0, errx.New(errx.ParamError, "unknown memory op")
	}
}

// findChangeByRequest 读取某个 requestID 最早的一条变更；不存在时返回 nil。
func (s *SQLStore) findChangeByRequest(ctx context.Context, q rowQuerier, userID int64, requestID string) (*Change, error) {
	var row struct {
		ID            int64          `db:"id"`
		EntryID       int64          `db:"entry_id"`
		Op            string         `db:"op"`
		BeforeJSON    sql.NullString `db:"before_json"`
		AfterJSON     sql.NullString `db:"after_json"`
		ResultVersion int64          `db:"result_version"`
		Undone        int64          `db:"undone"`
		CreatedAtMs   int64          `db:"created_at_ms"`
	}
	err := q.QueryRowCtx(ctx, &row, `SELECT id, entry_id, op, before_json, after_json, result_version, undone, created_at_ms
		FROM memory_change WHERE user_id=? AND request_id=? ORDER BY id ASC LIMIT 1`, userID, requestID)
	if err == sqlx.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &Change{
		ID: row.ID, UserID: userID, EntryID: row.EntryID, Op: row.Op,
		Before: decodeEntry(row.BeforeJSON.String), After: decodeEntry(row.AfterJSON.String),
		ResultVersion: int32(row.ResultVersion), RequestID: requestID, Undone: row.Undone == 1, CreatedAtMs: row.CreatedAtMs,
	}, nil
}

// addOne 新增条目：校验目标与内容安全，规范化去重，再检查容量后写入并记录变更。
func (s *SQLStore) addOne(ctx context.Context, session sqlx.Session, userID int64, target, content, requestID string, nowMs int64) (*Entry, int64, error) {
	if !ValidTarget(target) {
		return nil, 0, errx.New(errx.ParamError, "memory target must be memory or user")
	}
	content = strings.TrimSpace(content)
	if err := ScanContent(ctx, s.Scanner, content); err != nil {
		return nil, 0, err
	}
	norm := Normalize(content)
	existing, err := s.findByNorm(ctx, session, userID, target, norm)
	if err != nil {
		return nil, 0, err
	}
	if existing != nil {
		return existing, 0, nil
	}
	all, err := s.listActiveForUpdate(ctx, session, userID, target)
	if err != nil {
		return nil, 0, err
	}
	if UsedRunes(all, target)+utf8.RuneCountInString(content) > LimitFor(target) {
		return nil, 0, errx.New(errx.ParamError, "memory capacity exceeded")
	}
	res, err := session.ExecCtx(ctx, `INSERT INTO core_memory_entry (user_id, target, content, content_norm, version, deleted_at_ms, created_at_ms, updated_at_ms)
		VALUES (?, ?, ?, ?, 1, NULL, ?, ?)`, userID, target, content, clipNorm(norm), nowMs, nowMs)
	if err != nil {
		return nil, 0, err
	}
	id, _ := res.LastInsertId()
	entry := Entry{ID: id, UserID: userID, Target: target, Content: content, Version: 1, CreatedAtMs: nowMs, UpdatedAtMs: nowMs}
	changeID, err := s.insertChange(ctx, session, userID, id, OpAdd, nil, &entry, 1, requestID, nowMs)
	if err != nil {
		return nil, 0, err
	}
	return &entry, changeID, nil
}

// replaceOne 改写条目：版本号必须匹配，新内容不能与同目标其他条目重复且不超容量。
func (s *SQLStore) replaceOne(ctx context.Context, session sqlx.Session, userID, id int64, content string, version int32, requestID string, nowMs int64) (*Entry, int64, error) {
	current, err := s.getOwned(ctx, session, userID, id)
	if err != nil {
		return nil, 0, err
	}
	content = strings.TrimSpace(content)
	if err := ScanContent(ctx, s.Scanner, content); err != nil {
		return nil, 0, err
	}
	if current.Version != version {
		return current, 0, errx.New(errx.ContentVersionConflict, "memory version conflict")
	}
	duplicate, err := s.findByNorm(ctx, session, userID, current.Target, Normalize(content))
	if err != nil {
		return nil, 0, err
	}
	if duplicate != nil && duplicate.ID != current.ID {
		return nil, 0, errx.New(errx.ParamError, "memory content duplicates another entry")
	}
	all, err := s.listActiveForUpdate(ctx, session, userID, current.Target)
	if err != nil {
		return nil, 0, err
	}
	used := UsedRunes(all, current.Target) - utf8.RuneCountInString(current.Content) + utf8.RuneCountInString(content)
	if used > LimitFor(current.Target) {
		return nil, 0, errx.New(errx.ParamError, "memory capacity exceeded")
	}
	next := current.Version + 1
	res, err := session.ExecCtx(ctx, `UPDATE core_memory_entry SET content=?, content_norm=?, version=?, updated_at_ms=? WHERE id=? AND user_id=? AND version=? AND deleted_at_ms IS NULL`,
		content, clipNorm(Normalize(content)), next, nowMs, id, userID, version)
	if err != nil {
		return nil, 0, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		fresh, _ := s.getOwned(ctx, session, userID, id)
		return fresh, 0, errx.New(errx.ContentVersionConflict, "memory version conflict")
	}
	before := *current
	current.Content = content
	current.Version = next
	current.UpdatedAtMs = nowMs
	changeID, err := s.insertChange(ctx, session, userID, id, OpReplace, &before, current, next, requestID, nowMs)
	if err != nil {
		return nil, 0, err
	}
	return current, changeID, nil
}

// removeOne 软删除条目（写 deleted_at_ms），版本号必须匹配。
func (s *SQLStore) removeOne(ctx context.Context, session sqlx.Session, userID, id int64, version int32, requestID string, nowMs int64) (*Entry, int64, error) {
	current, err := s.getOwned(ctx, session, userID, id)
	if err != nil {
		return nil, 0, err
	}
	if current.Version != version {
		return current, 0, errx.New(errx.ContentVersionConflict, "memory version conflict")
	}
	next := current.Version + 1
	res, err := session.ExecCtx(ctx, `UPDATE core_memory_entry SET deleted_at_ms=?, version=?, updated_at_ms=? WHERE id=? AND user_id=? AND version=? AND deleted_at_ms IS NULL`,
		nowMs, next, nowMs, id, userID, version)
	if err != nil {
		return nil, 0, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return nil, 0, errx.New(errx.ContentVersionConflict, "memory version conflict")
	}
	before := *current
	current.Deleted = true
	current.Version = next
	current.UpdatedAtMs = nowMs
	changeID, err := s.insertChange(ctx, session, userID, id, OpRemove, &before, current, next, requestID, nowMs)
	if err != nil {
		return nil, 0, err
	}
	return current, changeID, nil
}

// insertChange 追加变更记录；同一 requestID 的唯一键冲突说明另一事务已写入，返回已有记录 ID。
func (s *SQLStore) insertChange(ctx context.Context, session sqlx.Session, userID, entryID int64, op string, before, after *Entry, resultVersion int32, requestID string, nowMs int64) (int64, error) {
	res, err := session.ExecCtx(ctx, `INSERT INTO memory_change (user_id, entry_id, op, before_json, after_json, result_version, request_id, undone, created_at_ms)
		VALUES (?, ?, ?, ?, ?, ?, ?, 0, ?)`,
		userID, entryID, op, encodeEntry(before), encodeEntry(after), resultVersion, requestID, nowMs)
	if err != nil {
		if sqlx.IsDuplicateKey(err) {
			var idRow struct {
				ID int64 `db:"id"`
			}
			if qerr := session.QueryRowCtx(ctx, &idRow, `SELECT id FROM memory_change WHERE user_id=? AND request_id=? AND entry_id=? AND op=?`,
				userID, requestID, entryID, op); qerr == nil {
				return idRow.ID, nil
			}
		}
		return 0, err
	}
	id, _ := res.LastInsertId()
	return id, nil
}

// requireMemoryCAS 要求条件更新恰好命中一行，否则说明版本已被并发修改。
func requireMemoryCAS(result sql.Result) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return errx.New(errx.ContentVersionConflict, "memory version conflict")
	}
	return nil
}
