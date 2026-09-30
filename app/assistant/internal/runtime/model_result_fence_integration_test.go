//go:build integration

package runtime

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"esx/app/assistant/internal/prompt"
	"esx/app/assistant/internal/store"
	sqlx "esx/pkg/sqlstore"
	"esx/pkg/testutil"
)

type acceptanceBarrierStore struct {
	store.Store
	afterReplayRead func()
	afterInput      func()
}

func (s acceptanceBarrierStore) Transact(ctx context.Context, fn func(context.Context, store.Store) error) error {
	return s.Store.Transact(ctx, func(ctx context.Context, tx store.Store) error {
		return fn(ctx, acceptanceBarrierStore{Store: tx, afterReplayRead: s.afterReplayRead, afterInput: s.afterInput})
	})
}
func (s acceptanceBarrierStore) GetInputCommand(ctx context.Context, userID int64, requestID string) (*store.InputCommand, error) {
	command, err := s.Store.GetInputCommand(ctx, userID, requestID)
	// Unlike consent's FOR SHARE read, this establishes a consistent snapshot.
	if err == nil && s.afterReplayRead != nil {
		s.afterReplayRead()
	}
	return command, err
}

func (s acceptanceBarrierStore) SetRunInput(ctx context.Context, id int64, payload []byte, now int64) error {
	err := s.Store.SetRunInput(ctx, id, payload, now)
	if err == nil && s.afterInput != nil {
		s.afterInput()
	}
	return err
}

func TestSQLModelResultAndAcceptanceLockOrders(t *testing.T) {
	env := testutil.SetupTestEnv(t, "xbh_assistant", testutil.SchemaPath("xbh_assistant.sql"))
	t.Cleanup(env.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	for _, q := range []string{`CREATE DATABASE xbh_user`, `CREATE TABLE xbh_user.agent_capability_consent (user_id BIGINT PRIMARY KEY, granted TINYINT NOT NULL, consent_version INT NOT NULL)`, `INSERT INTO xbh_user.agent_capability_consent VALUES (1,1,2),(2,1,2),(3,1,2)`} {
		if _, err := env.DB.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	st := store.NewSQLStore(sqlx.NewSqlConnFromDB(env.DB))
	for i, scenario := range []string{"accept_first", "tool_commit_first", "terminal_commit_first"} {
		t.Run(scenario, func(t *testing.T) {
			userID := int64(i + 1)
			accept := &Acceptor{Store: st}
			_, err := accept.Accept(ctx, AcceptInput{UserID: userID, RequestID: "original", Message: "original", ConsentOK: true, ConsentVersion: 2})
			if err != nil {
				t.Fatal(err)
			}
			run, err := st.Claim(ctx, "worker", store.NowMs(), 60_000)
			if err != nil || run == nil || run.UserID != userID {
				t.Fatalf("claim %+v %v", run, err)
			}
			run.Phase = store.PhaseModelRequest
			if err := st.UpdateRun(ctx, *run); err != nil {
				t.Fatal(err)
			}
			reached, release := make(chan struct{}), make(chan struct{})
			pause := func() {
				close(reached)
				select {
				case <-release:
				case <-ctx.Done():
				}
			}
			wrapper := acceptanceBarrierStore{Store: st}
			if scenario == "accept_first" {
				wrapper.afterInput = pause
			} else {
				wrapper.afterReplayRead = sync.OnceFunc(pause)
			}
			type result struct {
				out AcceptResult
				err error
			}
			accepted := make(chan result, 1)
			go func() {
				out, err := (&Acceptor{Store: wrapper}).Accept(ctx, AcceptInput{UserID: userID, RequestID: "new", Message: "new input", ConsentOK: true, ConsentVersion: 2})
				accepted <- result{out, err}
			}()
			select {
			case <-reached:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			engine := &Engine{Store: st}
			committed := make(chan error, 1)
			go func() {
				if scenario == "terminal_commit_first" {
					committed <- engine.finishWithMessage(ctx, *run, store.StatusDone, store.EventDone, store.EventPayload{Text: "winner"}, "winner", nil)
					return
				}
				candidate := *run
				candidate.Phase = store.PhaseToolExecuting
				committed <- engine.recordModelToolStep(ctx, candidate, prompt.Turn{Role: store.RoleAssistant, ToolCalls: []prompt.ToolCall{{ID: "committed", Name: "get_memory", Arguments: "{}"}}}, nil)
			}()
			var commitErr error
			if scenario == "accept_first" {
				close(release)
				commitErr = <-committed
			} else {
				commitErr = <-committed
				close(release)
			}
			got := <-accepted
			if got.err != nil {
				t.Fatal(got.err)
			}
			switch scenario {
			case "accept_first":
				if !errors.Is(commitErr, errRunRedirected) || got.out.Disposition != store.DispositionRedirected {
					t.Fatalf("commit %v accept %+v", commitErr, got.out)
				}
				current, _ := st.GetRun(ctx, run.ID)
				thread, _ := st.GetThread(ctx, userID)
				if current.InputVersion != 2 || current.Status != store.StatusRunning || thread.ActiveRunID != run.ID {
					t.Fatalf("redirect lost: run=%+v thread=%+v", current, thread)
				}
			case "tool_commit_first":
				if commitErr != nil || got.out.Disposition != store.DispositionSteered {
					t.Fatalf("commit %v accept %+v", commitErr, got.out)
				}
			default:
				if commitErr != nil || got.out.Disposition != store.DispositionStarted || got.out.RunID == run.ID {
					t.Fatalf("commit %v accept %+v", commitErr, got.out)
				}
			}
			// Keep subsequent cases isolated from the claim queue.
			if err := st.RequestCancelAll(ctx, userID); err != nil {
				t.Fatal(err)
			}
			if _, err := env.DB.ExecContext(ctx, `UPDATE agent_run SET status='cancelled' WHERE user_id=? AND status IN ('running','queued')`, userID); err != nil {
				t.Fatal(err)
			}
		})
	}
}
