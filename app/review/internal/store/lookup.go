package store

import (
	"context"
	"errors"

	sqlx "esx/pkg/sqlstore"
)

// CachedVerdict 是指纹复用表的一行（RVW-015）。
type CachedVerdict struct {
	Verdict         string `db:"verdict"`
	PolicyCodesJSON string `db:"policy_codes"`
	Source          string `db:"source"`
	DecisionID      int64  `db:"decision_id"`
}

// LookupVerdict 只在（快照哈希，市场，政策版本）完全一致时命中。
func (s *Store) LookupVerdict(ctx context.Context, hash, market, policyVersion string) (*CachedVerdict, error) {
	var v CachedVerdict
	err := s.conn.QueryRowCtx(ctx, &v, `SELECT verdict, policy_codes, source, decision_id FROM verdict_cache
		WHERE snapshot_hash = ? AND market = ? AND policy_version = ?`, hash, market, policyVersion)
	if errors.Is(err, sqlx.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// PolicyCodes 解码政策码。
func (v CachedVerdict) PolicyCodes() []string {
	return Decision{PolicyCodesJSON: v.PolicyCodesJSON}.PolicyCodes()
}

// ApprovedMedia 返回已在人工通过快照中出现过的图片哈希。
func (s *Store) ApprovedMedia(ctx context.Context, hashes []string) (map[string]bool, error) {
	out := map[string]bool{}
	if len(hashes) == 0 {
		return out, nil
	}
	var rows []string
	if err := s.conn.QueryRowsCtx(ctx, &rows, `SELECT sha256 FROM approved_media WHERE sha256 IN (`+placeholders(len(hashes))+`)`,
		anyArgs(hashes)...); err != nil {
		return nil, err
	}
	for _, h := range rows {
		out[h] = true
	}
	return out, nil
}

// TaskMediaAllowed 报告媒体是否属于任务快照（证件或素材），用于证据读取授权（RVW-023）。
func (s *Store) TaskSnapshotContent(ctx context.Context, taskID int64) (*Task, string, error) {
	task, err := s.GetTask(ctx, taskID)
	if err != nil {
		return nil, "", err
	}
	content, err := s.Snapshot(ctx, task.SnapshotHash)
	if err != nil {
		return nil, "", err
	}
	return task, content, nil
}
