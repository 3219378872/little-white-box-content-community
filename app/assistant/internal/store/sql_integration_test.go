//go:build integration

package store

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	assistantmemory "esx/app/assistant/internal/memory"
	"esx/pkg/errx"
	"esx/pkg/testutil"

	sqlx "esx/pkg/sqlstore"
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
		Status: StatusQueued, Phase: PhaseQueued, ConsentVersion: 3, InputVersion: 1, CreatedAtMs: NowMs(),
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

func TestSQLRunPersistsNormalizedProviderUsage(t *testing.T) {
	assistantTestEnv.TruncateAll(t, "agent_run_event", "agent_run")
	ctx := context.Background()
	st := newAssistantTestStore()
	run, err := st.InsertRun(ctx, Run{
		UserID: 9, SessionID: 7, RequestID: "usage-roundtrip", Source: SourceUser,
		Status: StatusQueued, Phase: PhaseQueued, ConsentVersion: 3, InputVersion: 1,
		InputTokens: 100, OutputTokens: 20, CacheTokens: 40, CacheWriteTokens: 8,
		ReasoningTokens: 6, LastPromptTokens: 96, UsageEstimated: true, CostUSD: 0.0123, CreatedAtMs: NowMs(),
	})
	if err != nil {
		t.Fatal(err)
	}
	assertUsage := func(got *Run, cacheWrite, reasoning, lastPrompt int64, estimated bool) {
		t.Helper()
		if got.CacheWriteTokens != cacheWrite || got.ReasoningTokens != reasoning || got.LastPromptTokens != lastPrompt || got.UsageEstimated != estimated {
			t.Fatalf("usage roundtrip=%+v", got)
		}
	}
	loaded, err := st.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertUsage(loaded, 8, 6, 96, true)
	loaded.CacheWriteTokens = 12
	loaded.ReasoningTokens = 9
	loaded.LastPromptTokens = 144
	loaded.UsageEstimated = false
	if err := st.UpdateRun(ctx, *loaded); err != nil {
		t.Fatal(err)
	}
	loaded, err = st.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertUsage(loaded, 12, 9, 144, false)
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
		Status: StatusQueued, Phase: PhaseQueued, ConsentVersion: 3, InputVersion: 1, CreatedAtMs: NowMs(),
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

func TestSQLRetentionPurgesMessagesInBoundedBatches(t *testing.T) {
	assistantTestEnv.TruncateAll(t, "assistant_index_outbox", "assistant_message", "assistant_thread", "assistant_session")
	ctx := context.Background()
	st := newAssistantTestStore()
	now := time.Now().UTC().Truncate(time.Second)
	messageCutoff := now.Add(-365 * 24 * time.Hour).UnixMilli()

	session, err := st.CreateSession(ctx, Session{UserID: 41, Status: SessionOpen, CreatedAtMs: messageCutoff - 10})
	if err != nil {
		t.Fatal(err)
	}
	oldOne, err := st.InsertMessage(ctx, Message{UserID: 41, SessionID: session.ID, Role: RoleUser, Kind: KindMessage, Content: "expired-one", Visible: true, CreatedAtMs: messageCutoff - 2})
	if err != nil {
		t.Fatal(err)
	}
	oldTwo, err := st.InsertMessage(ctx, Message{UserID: 41, SessionID: session.ID, Role: RoleAssistant, Kind: KindMessage, Content: "expired-two", Visible: true, Unread: true, CreatedAtMs: messageCutoff - 1})
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := st.InsertMessage(ctx, Message{UserID: 42, SessionID: session.ID, Role: RoleUser, Kind: KindMessage, Content: "fresh", Visible: true, CreatedAtMs: messageCutoff + 1})
	if err != nil {
		t.Fatal(err)
	}
	for _, message := range []Message{oldOne, oldTwo, fresh} {
		if err := st.InsertOutbox(ctx, Outbox{UserID: message.UserID, MessageID: message.ID, Op: IndexOpUpsert, PayloadJSON: `{"content":"sensitive"}`, CreatedAtMs: now.UnixMilli()}); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.SaveThread(ctx, Thread{UserID: 41, SessionID: session.ID, UnreadCount: 1, LastMessageID: oldTwo.ID, LastMessagePreview: "expired-two", LastMessageAtMs: oldTwo.CreatedAtMs, UpdatedAtMs: now.UnixMilli()}); err != nil {
		t.Fatal(err)
	}

	for index, want := range []int{1, 1, 0} {
		got, err := st.PurgeExpiredMessages(ctx, messageCutoff, 1)
		if err != nil || got != want {
			t.Fatalf("message purge %d got=%d want=%d err=%v", index, got, want, err)
		}
	}
	var expiredMessages int64
	if err := st.exec.QueryRowCtx(ctx, &expiredMessages, `SELECT COUNT(*) FROM assistant_message WHERE id IN (?, ?)`, oldOne.ID, oldTwo.ID); err != nil || expiredMessages != 0 {
		t.Fatalf("expired messages=%d err=%v", expiredMessages, err)
	}
	if message, err := st.GetMessage(ctx, fresh.UserID, fresh.ID); err != nil || message.Content != "fresh" {
		t.Fatalf("fresh message=%+v err=%v", message, err)
	}
	var deleteOutbox int64
	if err := st.exec.QueryRowCtx(ctx, &deleteOutbox, `SELECT COUNT(*) FROM assistant_index_outbox
		WHERE message_id IN (?, ?) AND op=? AND payload_json IS NULL`, oldOne.ID, oldTwo.ID, IndexOpDelete); err != nil || deleteOutbox != 2 {
		t.Fatalf("delete outbox=%d err=%v", deleteOutbox, err)
	}
	var leakedPayload int64
	if err := st.exec.QueryRowCtx(ctx, &leakedPayload, `SELECT COUNT(*) FROM assistant_index_outbox
		WHERE message_id IN (?, ?) AND payload_json IS NOT NULL`, oldOne.ID, oldTwo.ID); err != nil || leakedPayload != 0 {
		t.Fatalf("expired outbox payloads=%d err=%v", leakedPayload, err)
	}
	thread, err := st.GetThread(ctx, 41)
	if err != nil || thread.LastMessageID != 0 || thread.LastMessagePreview != "" || thread.UnreadCount != 0 {
		t.Fatalf("thread=%+v err=%v", thread, err)
	}
}

func TestSQLRetentionMessageDeleteFailureRollsBackOutbox(t *testing.T) {
	assistantTestEnv.TruncateAll(t, "assistant_index_outbox", "assistant_message", "assistant_session")
	ctx := context.Background()
	st := newAssistantTestStore()
	session, err := st.CreateSession(ctx, Session{UserID: 51, Status: SessionOpen, CreatedAtMs: 1})
	if err != nil {
		t.Fatal(err)
	}
	message, err := st.InsertMessage(ctx, Message{UserID: 51, SessionID: session.ID, Role: RoleUser, Kind: KindMessage, Content: "must survive rollback", Visible: true, CreatedAtMs: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.InsertOutbox(ctx, Outbox{UserID: 51, MessageID: message.ID, Op: IndexOpUpsert, PayloadJSON: `{"content":"must survive rollback"}`, CreatedAtMs: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.exec.ExecCtx(ctx, `CREATE TRIGGER fail_retention_delete BEFORE DELETE ON assistant_message
		FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='injected retention failure'`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = st.exec.ExecCtx(context.Background(), `DROP TRIGGER IF EXISTS fail_retention_delete`) })

	if _, err := st.PurgeExpiredMessages(ctx, 2, 10); err == nil {
		t.Fatal("injected delete failure must fail purge")
	}
	if loaded, err := st.GetMessage(ctx, 51, message.ID); err != nil || loaded.Content != "must survive rollback" {
		t.Fatalf("message=%+v err=%v", loaded, err)
	}
	var rows int64
	if err := st.exec.QueryRowCtx(ctx, &rows, `SELECT COUNT(*) FROM assistant_index_outbox
		WHERE message_id=? AND op=? AND payload_json IS NOT NULL`, message.ID, IndexOpUpsert); err != nil || rows != 1 {
		t.Fatalf("original outbox rows=%d err=%v", rows, err)
	}
	if err := st.exec.QueryRowCtx(ctx, &rows, `SELECT COUNT(*) FROM assistant_index_outbox
		WHERE message_id=? AND op=?`, message.ID, IndexOpDelete); err != nil || rows != 0 {
		t.Fatalf("delete outbox rows=%d err=%v", rows, err)
	}
}

func TestSQLMemoryMutationsSerializeCapacityDedupeAndUndo(t *testing.T) {
	assistantTestEnv.TruncateAll(t, "memory_change", "core_memory_entry", "memory_target_lock")
	ctx := context.Background()
	mem := assistantmemory.NewSQLStore(sqlx.NewSqlConnFromDB(assistantTestEnv.DB), nil)

	t.Run("normalized duplicate", func(t *testing.T) {
		var wg sync.WaitGroup
		start := make(chan struct{})
		ids := make(chan int64, 2)
		errs := make(chan error, 2)
		for index, content := range []string{"Concurrent   Value", " concurrent value "} {
			wg.Add(1)
			go func(index int, content string) {
				defer wg.Done()
				<-start
				entry, _, err := mem.Add(ctx, 71, assistantmemory.TargetMemory, content, "dedupe-"+string(rune('a'+index)), NowMs())
				if err == nil {
					ids <- entry.ID
				}
				errs <- err
			}(index, content)
		}
		close(start)
		wg.Wait()
		close(ids)
		close(errs)
		var first int64
		for err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
		for id := range ids {
			if first == 0 {
				first = id
			} else if id != first {
				t.Fatalf("duplicate ids=%d/%d", first, id)
			}
		}
		var count int64
		if err := assistantTestEnv.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM core_memory_entry WHERE user_id=71 AND deleted_at_ms IS NULL`).Scan(&count); err != nil || count != 1 {
			t.Fatalf("entries=%d err=%v", count, err)
		}
	})

	t.Run("capacity", func(t *testing.T) {
		var wg sync.WaitGroup
		start := make(chan struct{})
		errs := make(chan error, 2)
		for index, content := range []string{strings.Repeat("甲", 1200), strings.Repeat("乙", 1200)} {
			wg.Add(1)
			go func(index int, content string) {
				defer wg.Done()
				<-start
				_, _, err := mem.Add(ctx, 72, assistantmemory.TargetMemory, content, "capacity-"+string(rune('a'+index)), NowMs())
				errs <- err
			}(index, content)
		}
		close(start)
		wg.Wait()
		close(errs)
		successes := 0
		for err := range errs {
			if err == nil {
				successes++
				continue
			}
			if !errx.Is(err, errx.ParamError) {
				t.Fatalf("unexpected capacity race error: %v", err)
			}
		}
		if successes != 1 {
			t.Fatalf("successful over-capacity writes=%d", successes)
		}
		entries, _, err := mem.List(ctx, 72, assistantmemory.TargetMemory)
		if err != nil || assistantmemory.UsedRunes(entries, assistantmemory.TargetMemory) > assistantmemory.CapacityMemory {
			t.Fatalf("entries=%d err=%v", assistantmemory.UsedRunes(entries, assistantmemory.TargetMemory), err)
		}
	})

	t.Run("undo cas", func(t *testing.T) {
		entry, changeID, err := mem.Add(ctx, 73, assistantmemory.TargetUser, "undo once", "undo-add", NowMs())
		if err != nil || entry.ID == 0 || changeID == 0 {
			t.Fatalf("entry=%+v change=%d err=%v", entry, changeID, err)
		}
		var wg sync.WaitGroup
		start := make(chan struct{})
		errs := make(chan error, 2)
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				_, err := mem.Undo(ctx, 73, changeID, NowMs())
				errs <- err
			}()
		}
		close(start)
		wg.Wait()
		close(errs)
		successes := 0
		conflicts := 0
		for err := range errs {
			if err == nil {
				successes++
				continue
			}
			if !errx.Is(err, errx.ContentVersionConflict) {
				t.Fatalf("unexpected undo race error: %v", err)
			}
			conflicts++
		}
		if successes != 1 || conflicts != 1 {
			t.Fatalf("successful undos=%d conflicts=%d", successes, conflicts)
		}
	})
}

func TestSQLInputAcceptanceLocksOpenRunBeforeThread(t *testing.T) {
	assistantTestEnv.TruncateAll(t, "assistant_thread", "agent_run_event", "agent_run")
	ctx := context.Background()
	st := newAssistantTestStore()
	now := NowMs()
	run, err := st.InsertRun(ctx, Run{
		UserID: 84, SessionID: 1, RequestID: "lock-order", Source: SourceUser,
		Status: StatusRunning, Phase: PhaseModelRequest, ConsentVersion: 3, InputVersion: 1,
		LeaseOwner: "worker-lock-order", LeaseGeneration: 1, LeaseUntilMs: now + 60_000,
		CreatedAtMs: now, LastActivityAtMs: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SaveThread(ctx, Thread{UserID: run.UserID, SessionID: run.SessionID, ActiveRunID: run.ID, UpdatedAtMs: now}); err != nil {
		t.Fatal(err)
	}

	workerLockedRun := make(chan struct{})
	workerContinue := make(chan struct{})
	workerDone := make(chan error, 1)
	go func() {
		workerDone <- st.RunStep(ctx, run.Fence(), func(ctx context.Context, tx Store) error {
			close(workerLockedRun)
			<-workerContinue
			_, lockErr := tx.LockThread(ctx, run.UserID)
			return lockErr
		})
	}()
	<-workerLockedRun

	acceptStarted := make(chan struct{})
	acceptDone := make(chan error, 1)
	go func() {
		close(acceptStarted)
		acceptDone <- st.Transact(ctx, func(ctx context.Context, tx Store) error {
			if _, cancelErr := tx.CancelOpenBackground(ctx, run.UserID, []string{SourceMemoryReview}); cancelErr != nil {
				return cancelErr
			}
			if _, lockErr := tx.LockThread(ctx, run.UserID); lockErr != nil {
				return lockErr
			}
			return tx.SetRunInput(ctx, run.ID, []byte(`{"text":"redirect"}`), NowMs())
		})
	}()
	<-acceptStarted
	time.Sleep(100 * time.Millisecond)
	close(workerContinue)

	for name, done := range map[string]<-chan error{"worker": workerDone, "accept": acceptDone} {
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("%s transaction: %v", name, err)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("%s transaction timed out", name)
		}
	}
}
