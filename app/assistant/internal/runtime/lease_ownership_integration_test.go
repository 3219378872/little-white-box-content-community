//go:build integration

package runtime

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"esx/app/assistant/internal/llm"
	"esx/app/assistant/internal/store"
	"esx/app/assistant/internal/tool"
	"esx/pkg/testutil"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

type sqlExecutePauseStore struct {
	store.Store
	pause func(context.Context) error
}

func (s sqlExecutePauseStore) MaxEventSeq(ctx context.Context, runID int64) (int64, error) {
	seq, err := s.Store.MaxEventSeq(ctx, runID)
	if err != nil {
		return 0, err
	}
	return seq, s.pause(ctx)
}

func TestSQLExecuteKeepsClaimFenceAfterTakeover(t *testing.T) {
	env := testutil.SetupTestEnv(t, "xbh_assistant", testutil.SchemaPath("xbh_assistant.sql"))
	t.Cleanup(env.Close)
	ctx := context.Background()
	for _, query := range []string{
		`CREATE DATABASE xbh_user`,
		`CREATE TABLE xbh_user.agent_capability_consent (user_id BIGINT PRIMARY KEY, granted TINYINT NOT NULL, consent_version INT NOT NULL)`,
	} {
		if _, err := env.DB.ExecContext(ctx, query); err != nil {
			t.Fatal(err)
		}
	}
	st := store.NewSQLStore(sqlx.NewSqlConnFromDB(env.DB))
	registry, err := tool.NewRegistry(tool.Clients{Store: st}, []string{tool.SearchPosts, tool.PresentSources})
	if err != nil {
		t.Fatal(err)
	}
	for index, tc := range []struct {
		name    string
		problem error
	}{
		{name: "normal_execution"},
		{name: "cancel_finalizer", problem: errRunCancelled},
		{name: "error_finalizer", problem: errors.New("snapshot storage unavailable")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			userID := int64(index + 1)
			if _, err := env.DB.ExecContext(ctx, `INSERT INTO xbh_user.agent_capability_consent VALUES (?, 1, 2)`, userID); err != nil {
				t.Fatal(err)
			}
			accepted, err := (&Acceptor{Store: st}).Accept(ctx, AcceptInput{
				UserID: userID, RequestID: tc.name, Message: "lease ownership test",
				ClientProtocolVersion: 2, ConsentOK: true, ConsentVersion: 2,
			})
			if err != nil {
				t.Fatal(err)
			}
			first, err := st.Claim(ctx, "sql-worker-a", store.NowMs(), 60_000)
			if err != nil || first == nil || first.ID != accepted.RunID {
				t.Fatalf("first claim=%+v err=%v", first, err)
			}
			var staleCalls atomic.Int32
			stale := &Engine{Store: st, Tools: registry, Window: 128000, LLM: scriptLLM{
				complete: func(context.Context, llm.Request) (llm.Result, error) {
					staleCalls.Add(1)
					return llm.Result{Text: "stale worker answer"}, nil
				},
			}}
			reached, resume, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
			release := sync.OnceFunc(func() { close(resume) })
			pause := func(pauseCtx context.Context) error {
				close(reached)
				select {
				case <-resume:
					return nil
				case <-pauseCtx.Done():
					return pauseCtx.Err()
				}
			}
			// Freeze snapshots and the start event before pausing Execute at its last preparation read.
			if _, err := stale.prepareExecution(ctx, *first); err != nil {
				t.Fatal(err)
			}
			stale.Store = sqlExecutePauseStore{Store: st, pause: func(ctx context.Context) error {
				if err := pause(ctx); err != nil {
					return err
				}
				return tc.problem
			}}
			executeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			t.Cleanup(func() {
				cancel()
				release()
				select {
				case <-done:
				case <-time.After(5 * time.Second):
					t.Error("stale Execute did not exit")
				}
			})
			go func() {
				defer close(done)
				stale.Execute(executeCtx, *first, false)
			}()
			select {
			case <-reached:
			case <-done:
				t.Fatal("stale Execute returned before the takeover boundary")
			case <-executeCtx.Done():
				t.Fatal("stale Execute did not reach the takeover boundary")
			}

			expired, err := env.DB.ExecContext(ctx, `UPDATE agent_run SET lease_until_ms=?
				WHERE id=? AND lease_owner=? AND lease_generation=? AND status='running'`,
				store.NowMs()-1, first.ID, first.LeaseOwner, first.LeaseGeneration)
			if err != nil {
				t.Fatal(err)
			}
			if affected, err := expired.RowsAffected(); err != nil || affected != 1 {
				t.Fatalf("expired rows=%d err=%v", affected, err)
			}
			second, err := st.Claim(ctx, "sql-worker-b", store.NowMs(), 60_000)
			if err != nil || second == nil || second.ID != first.ID || second.LeaseOwner != "sql-worker-b" || second.LeaseGeneration != first.LeaseGeneration+1 {
				t.Fatalf("replacement claim=%+v err=%v", second, err)
			}
			beforeEvents, err := st.ListEventsAfter(ctx, first.ID, 0)
			if err != nil {
				t.Fatal(err)
			}
			release()
			select {
			case <-done:
			case <-executeCtx.Done():
				t.Fatal("stale Execute did not stop after takeover")
			}
			fresh, err := st.GetRun(ctx, first.ID)
			if err != nil || !reflect.DeepEqual(fresh, second) || staleCalls.Load() != 0 {
				t.Fatalf("stale Execute changed replacement: calls=%d run=%+v want=%+v err=%v", staleCalls.Load(), fresh, second, err)
			}
			events, err := st.ListEventsAfter(ctx, first.ID, 0)
			if err != nil || !reflect.DeepEqual(events, beforeEvents) {
				t.Fatalf("stale Execute published events: got=%+v want=%+v err=%v", events, beforeEvents, err)
			}
			thread, err := st.GetThread(ctx, userID)
			if err != nil || thread == nil || thread.ActiveRunID != first.ID {
				t.Fatalf("stale Execute released active thread: thread=%+v err=%v", thread, err)
			}

			var replacementCalls int
			replacement := &Engine{Store: st, Tools: registry, Window: 128000, LLM: scriptLLM{
				complete: func(context.Context, llm.Request) (llm.Result, error) {
					replacementCalls++
					return llm.Result{Text: "replacement worker answer"}, nil
				},
			}}
			replacement.Execute(ctx, *second, true)
			fresh, err = st.GetRun(ctx, first.ID)
			if err != nil || fresh == nil || fresh.Status != store.StatusDone || fresh.Fence() != second.Fence() || fresh.CancelRequested || fresh.ErrorCode != "" || replacementCalls != 1 {
				t.Fatalf("replacement did not complete: calls=%d run=%+v err=%v", replacementCalls, fresh, err)
			}
			messages, err := st.ListSessionMessages(ctx, userID, first.SessionID, false)
			if err != nil {
				t.Fatal(err)
			}
			visibleAnswers := 0
			for _, message := range messages {
				if message.RunID == first.ID && message.Role == store.RoleAssistant && message.Visible {
					visibleAnswers++
					if message.Content != "replacement worker answer" {
						t.Fatalf("unexpected answer=%+v", message)
					}
				}
			}
			events, err = st.ListEventsAfter(ctx, first.ID, 0)
			if err != nil {
				t.Fatal(err)
			}
			terminalEvents := 0
			for _, event := range events {
				if event.Type == store.EventDone {
					terminalEvents++
				}
				if event.Type == store.EventError {
					t.Fatalf("unexpected error event=%+v", event)
				}
			}
			if visibleAnswers != 1 || terminalEvents != 1 {
				t.Fatalf("visible answers=%d terminal events=%d", visibleAnswers, terminalEvents)
			}
		})
	}
}
