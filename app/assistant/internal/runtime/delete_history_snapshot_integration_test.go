//go:build integration

package runtime

import (
	"context"
	"sync"
	"testing"

	"esx/app/assistant/internal/prompt"
	"esx/app/assistant/internal/store"
	"esx/pkg/testutil"

	"github.com/stretchr/testify/require"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

type historySnapshotStore struct {
	store.Store
	afterRead func()
	once      *sync.Once
}

func (s historySnapshotStore) Transact(ctx context.Context, fn func(context.Context, store.Store) error) error {
	return s.Store.Transact(ctx, func(ctx context.Context, tx store.Store) error {
		return fn(ctx, historySnapshotStore{Store: tx, afterRead: s.afterRead, once: s.once})
	})
}
func (s historySnapshotStore) GetInputCommand(ctx context.Context, u int64, r string) (*store.InputCommand, error) {
	v, err := s.Store.GetInputCommand(ctx, u, r)
	s.once.Do(s.afterRead)
	return v, err
}
func TestSQLDeleteHistoryBetweenAcceptSnapshotAndLocks(t *testing.T) {
	env := testutil.SetupTestEnv(t, "xbh_assistant", testutil.SchemaPath("xbh_assistant.sql"))
	t.Cleanup(env.Close)
	ctx := context.Background()
	for _, q := range []string{`CREATE DATABASE xbh_user`, `CREATE TABLE xbh_user.agent_capability_consent (user_id BIGINT PRIMARY KEY,granted TINYINT NOT NULL,consent_version INT NOT NULL)`, `INSERT INTO xbh_user.agent_capability_consent VALUES (7,1,2)`} {
		_, err := env.DB.ExecContext(ctx, q)
		require.NoError(t, err)
	}
	st := store.NewSQLStore(sqlx.NewSqlConnFromDB(env.DB))
	const secret = "erased private summary"
	session, err := st.CreateSession(ctx, store.Session{UserID: 7, Status: store.SessionClosed, PromptEpoch: 1, CompactSummary: secret, PromptSnapshot: prompt.EncodeSnapshot(prompt.BuildSnapshot(nil, nil, secret))})
	require.NoError(t, err)
	require.NoError(t, st.SaveThread(ctx, store.Thread{UserID: 7, SessionID: session.ID, LastMessageAtMs: store.NowMs()}))
	_, err = st.InsertMessage(ctx, store.Message{UserID: 7, SessionID: session.ID, Role: store.RoleUser, Content: secret, Visible: true, Compacted: true})
	require.NoError(t, err)
	wrapper := historySnapshotStore{Store: st, once: &sync.Once{}, afterRead: func() { require.NoError(t, (&Acceptor{Store: st}).DeleteHistory(ctx, 7)) }}
	_, err = (&Acceptor{Store: wrapper}).Accept(ctx, AcceptInput{UserID: 7, RequestID: "new-after-delete", Message: "hello", ClientProtocolVersion: 2, ConsentOK: true, ConsentVersion: 2})
	require.NoError(t, err)
	current, err := st.GetSession(ctx, session.ID)
	require.NoError(t, err)
	require.Empty(t, current.CompactSummary, "new Accept must not resurrect deleted compact summary from pre-lock RR snapshot")
	require.NotContains(t, string(current.PromptSnapshot), secret)
}
