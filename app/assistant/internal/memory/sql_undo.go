package memory

import (
	"context"
	"database/sql"
	"unicode/utf8"

	"esx/pkg/errx"

	sqlx "esx/pkg/sqlstore"
)

// Undo 撤销一条变更：新增被撤销为删除，改写与删除被撤销为恢复变更前的内容。
// 只有条目仍处于该变更产生的版本时才允许撤销，避免覆盖之后的修改；每条变更只能撤销一次。
func (s *SQLStore) Undo(ctx context.Context, userID, changeID int64, nowMs int64) (*Entry, error) {
	if userID <= 0 || changeID <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	var out *Entry
	err := s.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		// 先不加锁读出变更所属条目的目标，按与写操作相同的顺序加目标锁，再加锁复读变更。
		pre, err := loadUndoChange(ctx, session, userID, changeID, false)
		if err != nil {
			return err
		}
		preEntry, err := s.getOwnedAny(ctx, session, userID, pre.EntryID)
		if err != nil {
			return err
		}
		if err := s.lockTargets(ctx, session, userID, preEntry.Target); err != nil {
			return err
		}
		row, err := loadUndoChange(ctx, session, userID, changeID, true)
		if err != nil {
			return err
		}
		if row.Undone == 1 {
			return errx.New(errx.ContentVersionConflict, msgAlreadyUndone)
		}
		// 条目版本必须仍等于该变更写入的版本。
		current, err := s.getOwnedAnyForUpdate(ctx, session, userID, row.EntryID)
		if err != nil {
			return err
		}
		if int64(current.Version) != row.ResultVersion {
			return errx.New(errx.ContentVersionConflict, msgVersionConflict)
		}
		// 按原操作类型执行反向操作。
		switch row.Op {
		case OpAdd:
			out, err = s.undoAdd(ctx, session, userID, current, nowMs)
		case OpReplace:
			out, err = s.restoreBefore(ctx, session, userID, current, decodeEntry(row.BeforeJSON.String), undoReplaceSQL, nowMs)
		case OpRemove:
			out, err = s.restoreBefore(ctx, session, userID, current, decodeEntry(row.BeforeJSON.String), undoRemoveSQL, nowMs)
		default:
			return errx.NewWithCode(errx.ParamError)
		}
		if err != nil {
			return err
		}
		// 最后标记变更已撤销；条件 undone=0 防止并发撤销重复生效。
		res, err := session.ExecCtx(ctx, `UPDATE memory_change SET undone=1 WHERE id=? AND user_id=? AND undone=0`, changeID, userID)
		if err != nil {
			return err
		}
		return requireMemoryCAS(res)
	})
	return out, err
}

// undoAdd 把新增的条目软删除。
func (s *SQLStore) undoAdd(ctx context.Context, session sqlx.Session, userID int64, current *Entry, nowMs int64) (*Entry, error) {
	res, err := session.ExecCtx(ctx, `UPDATE core_memory_entry SET deleted_at_ms=?, version=version+1, updated_at_ms=? WHERE id=? AND user_id=? AND version=? AND deleted_at_ms IS NULL`,
		nowMs, nowMs, current.ID, userID, current.Version)
	if err != nil {
		return nil, err
	}
	if err := requireMemoryCAS(res); err != nil {
		return nil, err
	}
	current.Deleted = true
	current.Version++
	current.UpdatedAtMs = nowMs
	return current, nil
}

// 撤销改写要求条目仍有效；撤销删除要求条目仍处于删除状态，并同时清除删除标记。
const (
	undoReplaceSQL = `UPDATE core_memory_entry SET content=?, content_norm=?, version=version+1, deleted_at_ms=NULL, updated_at_ms=? WHERE id=? AND user_id=? AND version=? AND deleted_at_ms IS NULL`
	undoRemoveSQL  = `UPDATE core_memory_entry SET deleted_at_ms=NULL, content=?, content_norm=?, version=version+1, updated_at_ms=? WHERE id=? AND user_id=? AND version=? AND deleted_at_ms IS NOT NULL`
)

// restoreBefore 把条目恢复为变更前的内容并递增版本；恢复前复核去重与容量，
// 因为变更之后同目标可能已写入相同内容或占满容量。
func (s *SQLStore) restoreBefore(ctx context.Context, session sqlx.Session, userID int64, current, before *Entry, query string, nowMs int64) (*Entry, error) {
	// 改写与删除的变更记录必定带变更前快照，缺失说明数据损坏。
	if before == nil {
		return nil, errx.NewWithCode(errx.SystemError)
	}
	if err := s.validateUndoRestore(ctx, session, userID, current, before); err != nil {
		return nil, err
	}
	res, err := session.ExecCtx(ctx, query,
		before.Content, clipNorm(Normalize(before.Content)), nowMs, current.ID, userID, current.Version)
	if err != nil {
		return nil, err
	}
	if err := requireMemoryCAS(res); err != nil {
		return nil, err
	}
	before.Version = current.Version + 1
	before.UpdatedAtMs = nowMs
	before.Deleted = false
	return before, nil
}

// undoChangeRow 是撤销需要的 memory_change 列。
type undoChangeRow struct {
	ID            int64          `db:"id"`
	UserID        int64          `db:"user_id"`
	EntryID       int64          `db:"entry_id"`
	Op            string         `db:"op"`
	BeforeJSON    sql.NullString `db:"before_json"`
	AfterJSON     sql.NullString `db:"after_json"`
	ResultVersion int64          `db:"result_version"`
	Undone        int64          `db:"undone"`
}

// loadUndoChange 读取用户自己的变更记录；forUpdate 为 true 时在目标锁内加行锁复读。
func loadUndoChange(ctx context.Context, q rowQuerier, userID, changeID int64, forUpdate bool) (*undoChangeRow, error) {
	query := `SELECT id, user_id, entry_id, op, before_json, after_json, result_version, undone FROM memory_change WHERE id=? AND user_id=?`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	var row undoChangeRow
	if err := q.QueryRowCtx(ctx, &row, query, changeID, userID); err != nil {
		if err == sqlx.ErrNotFound {
			return nil, errx.NewWithCode(errx.NotFound)
		}
		return nil, err
	}
	return &row, nil
}

// validateUndoRestore 确认恢复旧内容不会与现有条目重复，也不会超出目标容量。
func (s *SQLStore) validateUndoRestore(ctx context.Context, session sqlx.Session, userID int64, current *Entry, before *Entry) error {
	duplicate, err := s.findByNorm(ctx, session, userID, current.Target, Normalize(before.Content))
	if err != nil {
		return err
	}
	if duplicate != nil && duplicate.ID != current.ID {
		return errx.New(errx.ContentVersionConflict, msgRestoreConflict)
	}
	all, err := s.listActiveForUpdate(ctx, session, userID, current.Target)
	if err != nil {
		return err
	}
	used := UsedRunes(all, current.Target)
	if !current.Deleted {
		used -= utf8.RuneCountInString(current.Content)
	}
	used += utf8.RuneCountInString(before.Content)
	if used > LimitFor(current.Target) {
		return errx.New(errx.ParamError, msgCapacityExceeded)
	}
	return nil
}
