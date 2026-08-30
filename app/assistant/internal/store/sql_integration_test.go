//go:build integration

package store

import (
	"context"
	"errors"
	"os"
	"testing"

	"esx/pkg/testutil"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

var assistantTestEnv *testutil.TestEnv

func TestMain(m *testing.M) {
	assistantTestEnv = testutil.SetupTestEnvM("xbh_assistant", testutil.SchemaPath("xbh_assistant.sql"))
	code := m.Run()
	assistantTestEnv.Close()
	os.Exit(code)
}

func newAssistantTestStore() *SQLStore {
	return NewSQLStore(sqlx.NewSqlConnFromDB(assistantTestEnv.DB))
}

func TestSQLLeaseGenerationAndJournalTakeover(t *testing.T) {
	assistantTestEnv.TruncateAll(t, "agent_command_journal", "agent_run_event", "agent_run")
	ctx := context.Background()
	st := newAssistantTestStore()
	queued, err := st.InsertRun(ctx, Run{
		UserID: 1, SessionID: 1, RequestID: "sql-fence", Source: SourceUser,
		Status: StatusQueued, Phase: PhaseQueued, ConsentVersion: 2, InputVersion: 1, CreatedAtMs: NowMs(),
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := st.Claim(ctx, "worker-one", NowMs(), 60_000)
	if err != nil || first == nil || first.ID != queued.ID {
		t.Fatalf("first claim=%+v err=%v", first, err)
	}
	var journal *Journal
	if err := st.RunStep(ctx, first.Fence(), func(ctx context.Context, tx Store) error {
		var reserveErr error
		journal, _, reserveErr = tx.ReserveJournal(ctx, Journal{
			UserID: first.UserID, RequestID: first.RequestID, Tool: "update_post", CanonicalArgsDigest: "digest",
			RunID: first.ID, LeaseGeneration: first.LeaseGeneration, Status: JournalPending,
			CreatedAtMs: NowMs(), UpdatedAtMs: NowMs(),
		})
		return reserveErr
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.exec.ExecCtx(ctx, `UPDATE agent_run SET lease_until_ms=? WHERE id=?`, NowMs()-1, first.ID); err != nil {
		t.Fatal(err)
	}
	second, err := st.Claim(ctx, "worker-two", NowMs(), 60_000)
	if err != nil || second == nil || second.LeaseGeneration != first.LeaseGeneration+1 {
		t.Fatalf("second claim=%+v err=%v", second, err)
	}
	if err := st.RunStep(ctx, first.Fence(), func(context.Context, Store) error { return nil }); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("stale step err=%v", err)
	}
	if err := st.RunStep(ctx, second.Fence(), func(ctx context.Context, tx Store) error {
		taken, reserved, reserveErr := tx.ReserveJournal(ctx, Journal{
			UserID: second.UserID, RequestID: second.RequestID, Tool: "update_post", CanonicalArgsDigest: "digest",
			RunID: second.ID, LeaseGeneration: second.LeaseGeneration, Status: JournalPending,
			CreatedAtMs: NowMs(), UpdatedAtMs: NowMs(),
		})
		if reserveErr != nil {
			return reserveErr
		}
		if !reserved || taken == nil || !taken.Takeover || taken.ID != journal.ID {
			t.Fatalf("takeover=%+v reserved=%v", taken, reserved)
		}
		return tx.CompleteJournal(ctx, taken.ID, JournalSuccess, `{"ok":true}`)
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSQLTerminalFailureRollsBackRunMessageOutboxAndThread(t *testing.T) {
	assistantTestEnv.TruncateAll(t, "assistant_index_outbox", "assistant_message", "assistant_thread", "assistant_session", "agent_run_event", "agent_run")
	ctx := context.Background()
	st := newAssistantTestStore()
	session, err := st.CreateSession(ctx, Session{UserID: 3, Status: SessionOpen, CreatedAtMs: NowMs()})
	if err != nil {
		t.Fatal(err)
	}
	queued, err := st.InsertRun(ctx, Run{
		UserID: 3, SessionID: session.ID, RequestID: "terminal-rollback", Source: SourceUser,
		Status: StatusQueued, Phase: PhaseQueued, ConsentVersion: 2, InputVersion: 1, CreatedAtMs: NowMs(),
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := st.Claim(ctx, "worker", NowMs(), 60_000)
	if err != nil || run == nil || run.ID != queued.ID {
		t.Fatalf("claim=%+v err=%v", run, err)
	}
	if err := st.SaveThread(ctx, Thread{UserID: 3, SessionID: session.ID, ActiveRunID: run.ID, UpdatedAtMs: NowMs()}); err != nil {
		t.Fatal(err)
	}
	// Seed a terminal event so the transaction below fails at the unique
	// terminal constraint after staging all other terminal mutations.
	if _, err := st.InsertEvent(ctx, run.ID, EventDone, []byte(`{}`), NowMs()); err != nil {
		t.Fatal(err)
	}
	terminal := *run
	terminal.Status = StatusDone
	terminal.Phase = PhaseDone
	terminal.EndedAtMs = NowMs()
	err = st.RunStep(ctx, run.Fence(), func(ctx context.Context, tx Store) error {
		if err := tx.UpdateRun(ctx, terminal); err != nil {
			return err
		}
		message, err := tx.InsertMessage(ctx, Message{
			UserID: 3, SessionID: session.ID, RunID: run.ID, Role: RoleAssistant,
			Kind: KindMessage, Content: "must roll back", Visible: true, CreatedAtMs: NowMs(),
		})
		if err != nil {
			return err
		}
		if err := tx.InsertOutbox(ctx, Outbox{UserID: 3, MessageID: message.ID, Op: IndexOpUpsert, CreatedAtMs: NowMs()}); err != nil {
			return err
		}
		thread, err := tx.LockThread(ctx, 3)
		if err != nil {
			return err
		}
		thread.ActiveRunID = 0
		thread.LastMessageID = message.ID
		if err := tx.SaveThread(ctx, *thread); err != nil {
			return err
		}
		_, err = tx.InsertEvent(ctx, run.ID, EventError, []byte(`{}`), NowMs())
		return err
	})
	if err == nil {
		t.Fatal("duplicate terminal event must fail the transaction")
	}
	fresh, err := st.GetRun(ctx, run.ID)
	if err != nil || fresh.Status != StatusRunning {
		t.Fatalf("run=%+v err=%v", fresh, err)
	}
	messages, err := st.ListSessionMessages(ctx, 3, session.ID, true)
	if err != nil || len(messages) != 0 {
		t.Fatalf("messages=%+v err=%v", messages, err)
	}
	outbox, err := st.ListUnpublishedOutbox(ctx, 10)
	if err != nil || len(outbox) != 0 {
		t.Fatalf("outbox=%+v err=%v", outbox, err)
	}
	thread, err := st.GetThread(ctx, 3)
	if err != nil || thread.ActiveRunID != run.ID || thread.LastMessageID != 0 {
		t.Fatalf("thread=%+v err=%v", thread, err)
	}
}
