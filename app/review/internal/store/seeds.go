package store

import (
	"context"
	"errors"
	"time"

	sqlx "esx/pkg/sqlstore"
)

const (
	SeedCandidate = "candidate"
	SeedActive    = "active"
	SeedRetired   = "retired"
)

// Seed 是 review_seed 一行（RVW-030）。
type Seed struct {
	ID           int64  `db:"id"`
	IssueCode    string `db:"issue_code"`
	Market       string `db:"market"`
	Language     string `db:"language"`
	Text         string `db:"text"`
	Status       string `db:"status"`
	SourceTaskID int64  `db:"source_task_id"`
	NominatedBy  int64  `db:"nominated_by"`
	ConfirmedBy  int64  `db:"confirmed_by"`
	RetiredBy    int64  `db:"retired_by"`
	IndexState   string `db:"index_state"`
	CreatedAtMs  int64  `db:"created_at_ms"`
	UpdatedAtMs  int64  `db:"updated_at_ms"`
}

const seedColumns = `id, issue_code, market, language, text, status, source_task_id, nominated_by,
	confirmed_by, retired_by, index_state, created_at_ms, updated_at_ms`

// 种子向量集合同步状态。
const (
	SeedIndexed = "indexed"
	SeedRemoved = "removed"
)

// nominateSeed 把人审结论中的文本提名为候选种子并写审计，需另一位管理员确认后才生效。
func (s *Store) nominateSeed(ctx context.Context, session sqlx.Session, task *Task, issue, text string, actor int64, now time.Time) error {
	id, err := s.nextID()
	if err != nil {
		return err
	}
	if _, err := session.ExecCtx(ctx, `INSERT INTO review_seed (`+seedColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0, 0, '', ?, ?)`,
		id, issue, task.Market, task.Language, truncate(text, 2000), SeedCandidate, task.ID, actor,
		now.UnixMilli(), now.UnixMilli()); err != nil {
		return err
	}
	return s.insertAudit(ctx, session, AuditEntry{
		ActorID: actor, Action: "seed.nominate", ObjectType: "review_seed", ObjectID: id,
		After: map[string]any{"status": SeedCandidate, "issue": issue, "taskId": task.ID},
	}, now)
}

// ListSeeds 按状态列出种子。
func (s *Store) ListSeeds(ctx context.Context, status string, limit int) ([]Seed, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var rows []Seed
	err := s.conn.QueryRowsCtx(ctx, &rows, `SELECT `+seedColumns+` FROM review_seed WHERE status = ?
		ORDER BY id DESC LIMIT ?`, status, limit)
	return rows, err
}

// ActiveSeeds 返回全部生效种子，供 Router 复核与集合同步。
func (s *Store) ActiveSeeds(ctx context.Context) ([]Seed, error) {
	var rows []Seed
	err := s.conn.QueryRowsCtx(ctx, &rows, `SELECT `+seedColumns+` FROM review_seed WHERE status = ? ORDER BY id`, SeedActive)
	return rows, err
}

// SeedStatuses 返回指定种子的权威状态，Router 检索后按此复核（停用后不再命中）。
func (s *Store) SeedStatuses(ctx context.Context, ids []int64) (map[int64]string, error) {
	out := map[int64]string{}
	if len(ids) == 0 {
		return out, nil
	}
	var rows []struct {
		ID     int64  `db:"id"`
		Status string `db:"status"`
	}
	if err := s.conn.QueryRowsCtx(ctx, &rows, `SELECT id, status FROM review_seed WHERE id IN (`+placeholders(len(ids))+`)`,
		anyArgs(ids)...); err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.ID] = row.Status
	}
	return out, nil
}

// TransitionSeed 确认或停用种子；同一人不能既提名又确认（RVW-030）。
func (s *Store) TransitionSeed(ctx context.Context, seedID, actor int64, to string, now time.Time) (*Seed, error) {
	var out *Seed
	err := s.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		var seed Seed
		err := session.QueryRowCtx(ctx, &seed, `SELECT `+seedColumns+` FROM review_seed WHERE id = ? FOR UPDATE`, seedID)
		if errors.Is(err, sqlx.ErrNotFound) {
			return ErrSeedNotFound
		}
		if err != nil {
			return err
		}
		before := seed.Status
		switch {
		case to == SeedActive && seed.Status == SeedCandidate:
			if seed.NominatedBy == actor {
				return ErrSameActor
			}
			seed.ConfirmedBy = actor
		case to == SeedRetired && (seed.Status == SeedActive || seed.Status == SeedCandidate):
			seed.RetiredBy = actor
		default:
			return ErrSeedTransition
		}
		seed.Status = to
		seed.UpdatedAtMs = now.UnixMilli()
		if _, err := session.ExecCtx(ctx, `UPDATE review_seed SET status = ?, confirmed_by = ?, retired_by = ?,
			updated_at_ms = ? WHERE id = ?`, seed.Status, seed.ConfirmedBy, seed.RetiredBy, seed.UpdatedAtMs, seed.ID); err != nil {
			return err
		}
		if err := s.insertAudit(ctx, session, AuditEntry{
			ActorID: actor, Action: "seed." + to, ObjectType: "review_seed", ObjectID: seed.ID,
			Before: map[string]any{"status": before}, After: map[string]any{"status": seed.Status},
		}, now); err != nil {
			return err
		}
		out = &seed
		return nil
	})
	return out, err
}

// SeedsToSync 返回需要写入或移出向量集合的种子。
func (s *Store) SeedsToSync(ctx context.Context, limit int) ([]Seed, error) {
	var rows []Seed
	err := s.conn.QueryRowsCtx(ctx, &rows, `SELECT `+seedColumns+` FROM review_seed
		WHERE (status = ? AND index_state <> ?) OR (status = ? AND index_state = ?)
		ORDER BY id LIMIT ?`, SeedActive, SeedIndexed, SeedRetired, SeedIndexed, limit)
	return rows, err
}

// MarkSeedIndex 记录集合同步结果；只在状态未变化时更新，避免覆盖并发停用。
func (s *Store) MarkSeedIndex(ctx context.Context, id int64, status, indexState string) error {
	_, err := s.conn.ExecCtx(ctx, `UPDATE review_seed SET index_state = ? WHERE id = ? AND status = ?`, indexState, id, status)
	return err
}

// ActiveSeedSummary 返回某市场的生效种子数与最近更新时间，用于召回覆盖判断与版本串。
func (s *Store) ActiveSeedSummary(ctx context.Context, market string) (count int64, latest int64, err error) {
	var row struct {
		Count  int64 `db:"count"`
		Latest int64 `db:"latest"`
	}
	err = s.conn.QueryRowCtx(ctx, &row, `SELECT COUNT(*) AS count, COALESCE(MAX(updated_at_ms), 0) AS latest
		FROM review_seed WHERE status = ? AND market = ?`, SeedActive, market)
	return row.Count, row.Latest, err
}
