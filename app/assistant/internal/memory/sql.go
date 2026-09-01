package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"
	"strings"
	"unicode/utf8"

	"esx/pkg/errx"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

type SQLStore struct {
	conn    sqlx.SqlConn
	Scanner Scanner
}

func NewSQLStore(conn sqlx.SqlConn, scanner Scanner) *SQLStore {
	return &SQLStore{conn: conn, Scanner: scanner}
}

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

func (s *SQLStore) Active(ctx context.Context, userID int64) ([]Entry, error) {
	return s.listActive(ctx, s.conn, userID, "")
}

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

func (s *SQLStore) Batch(ctx context.Context, userID int64, requestID string, ops []Op, nowMs int64) ([]Entry, []int64, error) {
	return s.mutate(ctx, userID, requestID, ops, nowMs)
}

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
		req := requestID
		if len(ops) > 1 {
			req += "#" + itoa(int64(i))
		}
		change, err := s.findChangeByRequest(ctx, q, userID, req)
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

func (s *SQLStore) applyOps(ctx context.Context, session sqlx.Session, userID int64, requestID string, ops []Op, nowMs int64) ([]Entry, []int64, error) {
	entries := make([]Entry, 0, len(ops))
	changeIDs := make([]int64, 0, len(ops))
	for i, op := range ops {
		req := requestID
		if req == "" {
			req = "anon"
		}
		if len(ops) > 1 {
			req = req + "#" + itoa(int64(i))
		}
		entry, changeID, err := s.applyOne(ctx, session, userID, req, op, nowMs)
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

func (s *SQLStore) applyOne(ctx context.Context, session sqlx.Session, userID int64, requestID string, op Op, nowMs int64) (*Entry, int64, error) {
	if requestID != "" && requestID != "anon" {
		change, err := s.findChangeByRequest(ctx, session, userID, requestID)
		if err != nil {
			return nil, 0, err
		}
		if change != nil {
			if !memoryReplayMatches(op, *change) {
				return nil, 0, errx.NewWithCode(errx.IdempotencyConflict)
			}
			if strings.EqualFold(strings.TrimSpace(op.Op), OpRemove) {
				return nil, change.ID, nil
			}
			if change.After == nil {
				return nil, 0, errx.NewWithCode(errx.IdempotencyConflict)
			}
			entry := *change.After
			return &entry, change.ID, nil
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

func memoryReplayMatches(op Op, change Change) bool {
	wantOp := strings.ToLower(strings.TrimSpace(op.Op))
	if wantOp == "" {
		wantOp = OpAdd
	}
	if change.Op != wantOp || change.Undone {
		return false
	}
	switch wantOp {
	case OpAdd:
		return change.After != nil && change.After.Target == op.Target && Normalize(change.After.Content) == Normalize(op.Content)
	case OpReplace:
		return change.Before != nil && change.After != nil && change.EntryID == op.ID &&
			change.Before.Version == op.Version && Normalize(change.After.Content) == Normalize(op.Content)
	case OpRemove:
		return change.Before != nil && change.After != nil && change.EntryID == op.ID &&
			change.Before.Version == op.Version && change.After.Deleted
	default:
		return false
	}
}

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
	all, err := s.listActive(ctx, session, userID, target)
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
	all, err := s.listActive(ctx, session, userID, current.Target)
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

func (s *SQLStore) Undo(ctx context.Context, userID, changeID int64, nowMs int64) (*Entry, error) {
	if userID <= 0 || changeID <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	var out *Entry
	err := s.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
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
			return errx.New(errx.ContentVersionConflict, "memory change already undone")
		}
		current, err := s.getOwnedAnyForUpdate(ctx, session, userID, row.EntryID)
		if err != nil {
			return err
		}
		if int64(current.Version) != row.ResultVersion {
			return errx.New(errx.ContentVersionConflict, "memory version conflict")
		}
		before := decodeEntry(row.BeforeJSON.String)
		switch row.Op {
		case OpAdd:
			res, err := session.ExecCtx(ctx, `UPDATE core_memory_entry SET deleted_at_ms=?, version=version+1, updated_at_ms=? WHERE id=? AND user_id=? AND version=? AND deleted_at_ms IS NULL`,
				nowMs, nowMs, row.EntryID, userID, current.Version)
			if err != nil {
				return err
			}
			if err := requireMemoryCAS(res); err != nil {
				return err
			}
			current.Deleted = true
			current.Version++
			current.UpdatedAtMs = nowMs
			out = current
		case OpReplace:
			if before == nil {
				return errx.NewWithCode(errx.SystemError)
			}
			if err := s.validateUndoRestore(ctx, session, userID, current, before); err != nil {
				return err
			}
			res, err := session.ExecCtx(ctx, `UPDATE core_memory_entry SET content=?, content_norm=?, version=version+1, deleted_at_ms=NULL, updated_at_ms=? WHERE id=? AND user_id=? AND version=? AND deleted_at_ms IS NULL`,
				before.Content, clipNorm(Normalize(before.Content)), nowMs, row.EntryID, userID, current.Version)
			if err != nil {
				return err
			}
			if err := requireMemoryCAS(res); err != nil {
				return err
			}
			before.Version = current.Version + 1
			before.UpdatedAtMs = nowMs
			before.Deleted = false
			out = before
		case OpRemove:
			if before == nil {
				return errx.NewWithCode(errx.SystemError)
			}
			if err := s.validateUndoRestore(ctx, session, userID, current, before); err != nil {
				return err
			}
			res, err := session.ExecCtx(ctx, `UPDATE core_memory_entry SET deleted_at_ms=NULL, content=?, content_norm=?, version=version+1, updated_at_ms=? WHERE id=? AND user_id=? AND version=? AND deleted_at_ms IS NOT NULL`,
				before.Content, clipNorm(Normalize(before.Content)), nowMs, row.EntryID, userID, current.Version)
			if err != nil {
				return err
			}
			if err := requireMemoryCAS(res); err != nil {
				return err
			}
			before.Version = current.Version + 1
			before.UpdatedAtMs = nowMs
			before.Deleted = false
			out = before
		default:
			return errx.NewWithCode(errx.ParamError)
		}
		res, err := session.ExecCtx(ctx, `UPDATE memory_change SET undone=1 WHERE id=? AND user_id=? AND undone=0`, changeID, userID)
		if err != nil {
			return err
		}
		return requireMemoryCAS(res)
	})
	return out, err
}

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

func (s *SQLStore) validateUndoRestore(ctx context.Context, session sqlx.Session, userID int64, current *Entry, before *Entry) error {
	duplicate, err := s.findByNorm(ctx, session, userID, current.Target, Normalize(before.Content))
	if err != nil {
		return err
	}
	if duplicate != nil && duplicate.ID != current.ID {
		return errx.New(errx.ContentVersionConflict, "memory content already exists")
	}
	all, err := s.listActive(ctx, session, userID, current.Target)
	if err != nil {
		return err
	}
	used := UsedRunes(all, current.Target)
	if !current.Deleted {
		used -= utf8.RuneCountInString(current.Content)
	}
	used += utf8.RuneCountInString(before.Content)
	if used > LimitFor(current.Target) {
		return errx.New(errx.ParamError, "memory capacity exceeded")
	}
	return nil
}

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

func (s *SQLStore) RecordFeedback(ctx context.Context, userID int64, requestID string, postID int64, reason string) error {
	_, err := s.conn.ExecCtx(ctx, `INSERT INTO recommendation_feedback (user_id, request_id, post_id, reason) VALUES (?, ?, ?, ?)`,
		userID, requestID, postID, reason)
	return err
}

type rowQuerier interface {
	QueryRowCtx(ctx context.Context, v any, query string, args ...any) error
	QueryRowsCtx(ctx context.Context, v any, query string, args ...any) error
	ExecCtx(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func (s *SQLStore) listActive(ctx context.Context, q rowQuerier, userID int64, target string) ([]Entry, error) {
	query := `SELECT id, user_id, target, content, version, created_at_ms, updated_at_ms FROM core_memory_entry WHERE user_id=? AND deleted_at_ms IS NULL`
	args := []any{userID}
	if target != "" {
		query += ` AND target=?`
		args = append(args, target)
	}
	query += ` ORDER BY target ASC, id ASC`
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

func (s *SQLStore) findByNorm(ctx context.Context, q rowQuerier, userID int64, target, norm string) (*Entry, error) {
	var row struct {
		ID          int64  `db:"id"`
		UserID      int64  `db:"user_id"`
		Target      string `db:"target"`
		Content     string `db:"content"`
		Version     int64  `db:"version"`
		CreatedAtMs int64  `db:"created_at_ms"`
		UpdatedAtMs int64  `db:"updated_at_ms"`
	}
	err := q.QueryRowCtx(ctx, &row, `SELECT id, user_id, target, content, version, created_at_ms, updated_at_ms
		FROM core_memory_entry WHERE user_id=? AND target=? AND content_norm=? AND deleted_at_ms IS NULL LIMIT 1`,
		userID, target, clipNorm(norm))
	if err == sqlx.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &Entry{ID: row.ID, UserID: row.UserID, Target: row.Target, Content: row.Content,
		Version: int32(row.Version), CreatedAtMs: row.CreatedAtMs, UpdatedAtMs: row.UpdatedAtMs}, nil
}

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

func (s *SQLStore) getOwnedAny(ctx context.Context, q rowQuerier, userID, id int64) (*Entry, error) {
	return s.getOwnedAnyQuery(ctx, q, userID, id, false)
}

func (s *SQLStore) getOwnedAnyForUpdate(ctx context.Context, q rowQuerier, userID, id int64) (*Entry, error) {
	return s.getOwnedAnyQuery(ctx, q, userID, id, true)
}

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

func (s *SQLStore) insertChange(ctx context.Context, session sqlx.Session, userID, entryID int64, op string, before, after *Entry, resultVersion int32, requestID string, nowMs int64) (int64, error) {
	res, err := session.ExecCtx(ctx, `INSERT INTO memory_change (user_id, entry_id, op, before_json, after_json, result_version, request_id, undone, created_at_ms)
		VALUES (?, ?, ?, ?, ?, ?, ?, 0, ?)`,
		userID, entryID, op, encodeEntry(before), encodeEntry(after), resultVersion, requestID, nowMs)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") {
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

func encodeEntry(entry *Entry) any {
	if entry == nil {
		return nil
	}
	raw, _ := json.Marshal(entry)
	return string(raw)
}

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

func clipNorm(v string) string {
	runes := []rune(v)
	if len(runes) > 512 {
		return string(runes[:512])
	}
	return v
}

func itoa(v int64) string {
	return strings.TrimSpace(strings.TrimLeft(jsonNumber(v), " "))
}

func jsonNumber(v int64) string {
	raw, _ := json.Marshal(v)
	return string(raw)
}
