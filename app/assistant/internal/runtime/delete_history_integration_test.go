//go:build integration

package runtime

import (
	"context"
	"errors"
	"testing"

	"esx/app/assistant/internal/prompt"
	"esx/app/assistant/internal/store"
	"esx/pkg/testutil"

	"github.com/stretchr/testify/require"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

func TestSQLDeleteHistoryOutboxFailureRollsBack(t *testing.T) {
	env := testutil.SetupTestEnv(t, "xbh_assistant", testutil.SchemaPath("xbh_assistant.sql"))
	t.Cleanup(env.Close)
	ctx := context.Background()
	for _, query := range []string{
		`CREATE DATABASE xbh_user`,
		`CREATE TABLE xbh_user.agent_capability_consent (user_id BIGINT PRIMARY KEY, granted TINYINT NOT NULL, consent_version INT NOT NULL)`,
		`INSERT INTO xbh_user.agent_capability_consent VALUES (7, 1, 2)`,
	} {
		_, err := env.DB.ExecContext(ctx, query)
		require.NoError(t, err)
	}
	st := store.NewSQLStore(sqlx.NewSqlConnFromDB(env.DB))
	accepted, err := (&Acceptor{Store: st}).Accept(ctx, AcceptInput{
		UserID: 7, RequestID: "history-outbox-failure", Message: "private history",
		ClientProtocolVersion: 2, ConsentOK: true, ConsentVersion: 2,
	})
	require.NoError(t, err)
	claimed, err := st.Claim(ctx, "history-worker", store.NowMs(), 60_000)
	require.NoError(t, err)
	require.NotNil(t, claimed)
	session, err := st.GetSession(ctx, accepted.SessionID)
	require.NoError(t, err)
	session.CompactSummary = "private compacted history"
	session.PromptSnapshot = prompt.EncodeSnapshot(prompt.BuildSnapshot(nil, nil, session.CompactSummary))
	require.NoError(t, st.UpdateSession(ctx, *session))
	want := errors.New("outbox unavailable")
	require.ErrorIs(t, (&Acceptor{Store: failingHistoryOutbox{Store: st, err: want}}).DeleteHistory(ctx, 7), want)
	messages, err := st.ListMessages(ctx, 7, accepted.SessionID, 0, 0, 100)
	require.NoError(t, err)
	require.NotEmpty(t, messages, "outbox failure must roll back history deletion")
	retained, err := st.GetSession(ctx, accepted.SessionID)
	require.NoError(t, err)
	require.Equal(t, *session, *retained, "prompt invalidation must roll back with the messages")
	active, err := st.GetRun(ctx, claimed.ID)
	require.NoError(t, err)
	require.Equal(t, store.StatusRunning, active.Status)
	require.False(t, active.CancelRequested, "failed deletion must not cancel the run")
	events, err := st.ListEventsAfter(ctx, claimed.ID, 0)
	require.NoError(t, err)
	require.Empty(t, events, "failed deletion must not commit a terminal event")
	require.NoError(t, (&Acceptor{Store: st}).DeleteHistory(ctx, 7))
	messages, err = st.ListMessages(ctx, 7, accepted.SessionID, 0, 0, 100)
	require.NoError(t, err)
	require.Empty(t, messages)
	cleared, err := st.GetSession(ctx, accepted.SessionID)
	require.NoError(t, err)
	require.Empty(t, cleared.PromptSnapshot)
	require.Empty(t, cleared.CompactSummary)
	require.Equal(t, session.PromptEpoch+1, cleared.PromptEpoch)
	require.ErrorIs(t, st.RunStep(ctx, claimed.Fence(), func(ctx context.Context, tx store.Store) error {
		return tx.UpdateSession(ctx, *session)
	}), store.ErrLeaseLost, "old worker cannot restore its pre-deletion prompt")
	pending, err := st.ListUnpublishedOutbox(ctx, 100)
	require.NoError(t, err)
	deletes := 0
	for _, event := range pending {
		if event.Op == store.IndexOpDelete {
			deletes++
		}
	}
	require.Equal(t, 1, deletes, "successful deletion must commit its index cleanup intent")
}
