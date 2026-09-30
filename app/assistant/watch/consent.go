package watch

import (
	"context"
	"esx/app/assistant/internal/consent"
	sqlx "esx/pkg/sqlstore"
)

// WithConsent is the shared mutation entry point for direct RPC and model tools.
// SQLStore also repeats this check under its mutation transaction, closing the
// revocation window between this dependency/authorization gate and the write.
func WithConsent(st Store, reader consent.Reader) Store {
	if st == nil {
		return nil
	}
	return authorizedStore{Store: st, reader: reader}
}

type authorizedStore struct {
	Store
	reader consent.Reader
}

func (s authorizedStore) Create(ctx context.Context, task Task) (Task, error) {
	if err := consent.Require(ctx, s.reader, task.UserID); err != nil {
		return Task{}, err
	}
	return s.Store.Create(ctx, task)
}
func (s authorizedStore) UpdateEnabled(ctx context.Context, userID, id int64, enabled bool, version int32) (Task, error) {
	if err := consent.Require(ctx, s.reader, userID); err != nil {
		return Task{}, err
	}
	return s.Store.UpdateEnabled(ctx, userID, id, enabled, version)
}
func (s authorizedStore) Delete(ctx context.Context, userID, id int64, version int32) error {
	if err := consent.Require(ctx, s.reader, userID); err != nil {
		return err
	}
	return s.Store.Delete(ctx, userID, id, version)
}

type sqlConsent struct{ sqlx.Session }

func (s sqlConsent) AgentConsent(ctx context.Context, userID int64) (int32, bool, error) {
	var row struct {
		Granted int64 `db:"granted"`
		Version int32 `db:"consent_version"`
	}
	err := s.QueryRowCtx(ctx, &row, `SELECT granted, consent_version FROM xbh_user.agent_capability_consent WHERE user_id=? LIMIT 1 FOR SHARE`, userID)
	if err == sqlx.ErrNotFound {
		return 0, false, nil
	}
	return row.Version, row.Granted == 1, err
}
func (s *SQLStore) mutateAuthorized(ctx context.Context, userID int64, fn func(*SQLStore) error) error {
	return s.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		// User consent revocation UPDATE conflicts with this row share lock. Hold it
		// through ownership/revision validation and the Watch write's commit.
		if err := consent.Require(ctx, sqlConsent{session}, userID); err != nil {
			return err
		}
		return fn(&SQLStore{conn: sqlx.NewSqlConnFromSession(session)})
	})
}
func (s *SQLStore) Create(ctx context.Context, task Task) (out Task, err error) {
	err = s.mutateAuthorized(ctx, task.UserID, func(tx *SQLStore) error { var err error; out, err = tx.create(ctx, task); return err })
	return
}
func (s *SQLStore) UpdateEnabled(ctx context.Context, userID, id int64, enabled bool, version int32) (out Task, err error) {
	err = s.mutateAuthorized(ctx, userID, func(tx *SQLStore) error {
		var err error
		out, err = tx.updateEnabled(ctx, userID, id, enabled, version)
		return err
	})
	return
}
func (s *SQLStore) Delete(ctx context.Context, userID, id int64, version int32) error {
	return s.mutateAuthorized(ctx, userID, func(tx *SQLStore) error { return tx.delete(ctx, userID, id, version) })
}
