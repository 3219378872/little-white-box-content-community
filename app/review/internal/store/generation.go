package store

import (
	"context"
	"encoding/json"
	"strings"
)

// SeedGeneration 统计生效种子集合的变化（ADS-031）：每次确认、每次停用已生效的种子各计一次，
// 单调递增；changedAt 是最近一次此类变化的时间。从候选直接停用不改变生效集合，不计入。
func (s *Store) SeedGeneration(ctx context.Context) (changes int64, changedAt int64, err error) {
	var row struct {
		Confirmed int64 `db:"confirmed"`
		Retired   int64 `db:"retired"`
		ChangedAt int64 `db:"changed_at"`
	}
	err = s.conn.QueryRowCtx(ctx, &row, `SELECT COUNT(*) AS confirmed,
		COALESCE(SUM(CASE WHEN status = ? THEN 1 ELSE 0 END), 0) AS retired,
		COALESCE(MAX(updated_at_ms), 0) AS changed_at
		FROM review_seed WHERE confirmed_by > 0`, SeedRetired)
	return row.Confirmed + row.Retired, row.ChangedAt, err
}

// PolicyActivatedAt 返回政策版本首次以 active 模式被 worker 激活的时间（来自 policy.activate 审计）；
// 尚未激活时 found=false。审计 after_state 是按键排序的 JSON：{"configHash":..,"mode":..,"version":..}。
func (s *Store) PolicyActivatedAt(ctx context.Context, version string) (activatedAt int64, found bool, err error) {
	quoted, err := json.Marshal(version)
	if err != nil {
		return 0, false, err
	}
	pattern := `%"mode":"active","version":` + escapeLike(string(quoted)) + `}`
	var row struct {
		Count int64 `db:"count"`
		First int64 `db:"first"`
	}
	err = s.conn.QueryRowCtx(ctx, &row, `SELECT COUNT(*) AS count, COALESCE(MIN(created_at_ms), 0) AS first
		FROM audit_log WHERE object_type = 'policy' AND object_id = 0 AND action = 'policy.activate'
		AND after_state LIKE ?`, pattern)
	return row.First, row.Count > 0, err
}

func escapeLike(value string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(value)
}
