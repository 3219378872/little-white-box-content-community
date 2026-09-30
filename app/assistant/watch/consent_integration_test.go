//go:build integration

package watch

import (
	"context"
	"testing"
	"time"

	"esx/pkg/errx"
	sqlx "esx/pkg/sqlstore"
	"esx/pkg/testutil"
)

// Block only after the real consent SELECT FOR SHARE, so a concurrent revoker
// must serialize with the same transaction as the Watch write.
type watchSQLPauseConn struct {
	sqlx.SqlConn
	acquired, release chan struct{}
}

func (s watchSQLPauseConn) TransactCtx(ctx context.Context, fn func(context.Context, sqlx.Session) error) error {
	return s.SqlConn.TransactCtx(ctx, func(ctx context.Context, tx sqlx.Session) error {
		return fn(ctx, watchSQLPauseSession{Session: tx, acquired: s.acquired, release: s.release})
	})
}

type watchSQLPauseSession struct {
	sqlx.Session
	acquired, release chan struct{}
}

func (s watchSQLPauseSession) QueryRowCtx(ctx context.Context, v any, q string, args ...any) error {
	err := s.Session.QueryRowCtx(ctx, v, q, args...)
	if err == nil {
		close(s.acquired)
		select {
		case <-s.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return err
}

func TestSQLWatchConsentRevocationSerializesWithMutation(t *testing.T) {
	env := testutil.SetupTestEnv(t, "xbh_assistant", testutil.SchemaPath("xbh_assistant.sql"))
	t.Cleanup(env.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	for _, q := range []string{`CREATE DATABASE xbh_user`, `CREATE TABLE xbh_user.agent_capability_consent (user_id BIGINT PRIMARY KEY,granted TINYINT NOT NULL,consent_version INT NOT NULL)`, `INSERT INTO xbh_user.agent_capability_consent VALUES (1,1,2)`} {
		if _, err := env.DB.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	conn := sqlx.NewSqlConnFromDB(env.DB)
	acquired, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, err := NewSQLStore(watchSQLPauseConn{SqlConn: conn, acquired: acquired, release: release}).Create(ctx, Task{UserID: 1, ConditionType: KeywordNewPost, TargetType: "keyword", TargetText: "topic"})
		done <- err
	}()
	select {
	case <-acquired:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	revoked := make(chan error, 1)
	started := make(chan struct{})
	go func() {
		close(started)
		_, err := env.DB.ExecContext(ctx, `UPDATE xbh_user.agent_capability_consent SET granted=0 WHERE user_id=1`)
		revoked <- err
	}()
	<-started
	select {
	case err := <-revoked:
		close(release)
		t.Fatalf("revocation escaped share lock: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := <-revoked; err != nil {
		t.Fatal(err)
	}
	st := NewSQLStore(conn)
	tasks, err := st.ListTasks(ctx, 1)
	if err != nil || len(tasks) != 1 {
		t.Fatalf("committed tasks=%+v err=%v", tasks, err)
	}
	if err := st.Delete(ctx, 1, tasks[0].ID, tasks[0].Version); !errx.Is(err, errx.AgentNotAuthorized) {
		t.Fatalf("post-revocation delete=%v", err)
	}
	if _, err := st.UpdateEnabled(ctx, 1, tasks[0].ID, false, tasks[0].Version); !errx.Is(err, errx.AgentNotAuthorized) {
		t.Fatalf("post-revocation disable=%v", err)
	}
	if _, err := st.Create(ctx, Task{UserID: 1, ConditionType: KeywordNewPost, TargetType: "keyword", TargetText: "new"}); !errx.Is(err, errx.AgentNotAuthorized) {
		t.Fatalf("post-revocation create=%v", err)
	}
}
