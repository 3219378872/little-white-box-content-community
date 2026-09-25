package runtime

import (
	"context"
	"strings"
	"testing"

	"esx/app/assistant/internal/llm"
	"esx/app/assistant/internal/memory"
	"esx/app/assistant/internal/prompt"
	"esx/app/assistant/internal/store"
	"esx/app/assistant/internal/tool"

	"github.com/stretchr/testify/require"
)

func TestDeleteHistoryClearsPromptAndPreservesMemory(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore()
	memories := memory.NewMapStore()
	for _, target := range []string{"memory", "user"} {
		_, _, err := memories.Add(ctx, 1, target, "keep "+target, target, store.NowMs())
		require.NoError(t, err)
	}
	secret := "deleted conversation private marker"
	snapshot := prompt.BuildSnapshot(nil, []prompt.Turn{{Role: store.RoleUser, Content: secret}}, secret)
	session, err := st.CreateSession(ctx, store.Session{UserID: 1, Status: store.SessionOpen, PromptEpoch: 3, CompactSummary: secret, PromptSnapshot: prompt.EncodeSnapshot(snapshot), SuccessfulUserTurns: 9})
	require.NoError(t, err)
	other, err := st.CreateSession(ctx, store.Session{UserID: 2, PromptEpoch: 4, CompactSummary: "other user", PromptSnapshot: prompt.EncodeSnapshot(snapshot)})
	require.NoError(t, err)
	require.NoError(t, st.SaveThread(ctx, store.Thread{UserID: 1, SessionID: session.ID, LastMessageAtMs: store.NowMs()}))
	_, err = st.InsertMessage(ctx, store.Message{UserID: 1, SessionID: session.ID, Role: store.RoleUser, Content: secret, Visible: true, Compacted: true})
	require.NoError(t, err)
	accept := &Acceptor{Store: st, Memory: memories}
	require.NoError(t, accept.DeleteHistory(ctx, 1))
	messages, err := st.ListMessages(ctx, 1, session.ID, 0, 0, 50)
	require.NoError(t, err)
	require.Empty(t, messages)
	cleared, err := st.GetSession(ctx, session.ID)
	require.NoError(t, err)
	require.Empty(t, cleared.CompactSummary)
	require.Empty(t, cleared.PromptSnapshot)
	require.Equal(t, 4, cleared.PromptEpoch)
	require.Zero(t, cleared.SuccessfulUserTurns)
	preserved, err := st.GetSession(ctx, other.ID)
	require.NoError(t, err)
	require.Equal(t, other, *preserved)
	entries, err := memories.Active(ctx, 1)
	require.NoError(t, err)
	require.Len(t, entries, 2)
	accepted, err := accept.Accept(ctx, AcceptInput{UserID: 1, Message: "hello", RequestID: "after-delete", ConsentOK: true, ConsentVersion: 2})
	require.NoError(t, err)
	require.Equal(t, session.ID, accepted.SessionID)
	claimed, err := st.Claim(ctx, "test-worker", store.NowMs(), 60000)
	require.NoError(t, err)
	require.NotNil(t, claimed)
	var captured string
	model := scriptLLM{complete: func(_ context.Context, req llm.Request) (llm.Result, error) {
		for _, msg := range req.Messages {
			captured += msg.Content + "\n"
		}
		return llm.Result{Text: "hello"}, nil
	}}
	registry, err := tool.NewRegistry(tool.Clients{Store: st}, []string{tool.SearchPosts, tool.PresentSources})
	require.NoError(t, err)
	engine := &Engine{Store: st, Memory: memories, Tools: registry, LLM: model, Window: 128000}
	engine.Execute(ctx, *claimed, false)
	require.NotEmpty(t, captured)
	require.NotContains(t, captured, secret)
	require.Contains(t, captured, "keep memory")
	require.Contains(t, captured, "keep user")
}

func TestDeleteHistoryFencesInFlightCompaction(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore()
	var accept *Acceptor
	model := scriptLLM{complete: func(_ context.Context, _ llm.Request) (llm.Result, error) {
		// Complete deletion while compaction is outside its fenced database step.
		require.NoError(t, accept.DeleteHistory(ctx, 1))
		return llm.Result{Text: "old private compact summary"}, nil
	}}
	engine, run := newCancelTestEngine(t, st, model)
	accept = &Acceptor{Store: st}
	session, err := st.GetSession(ctx, run.SessionID)
	require.NoError(t, err)
	session.PromptSnapshot = prompt.EncodeSnapshot(prompt.BuildSnapshot(nil, nil, ""))
	require.NoError(t, st.UpdateSession(ctx, *session))
	for i := 0; i < 10; i++ {
		_, err := st.InsertMessage(ctx, store.Message{UserID: run.UserID, SessionID: run.SessionID, Role: store.RoleUser, Content: strings.Repeat("old private conversation ", 200), Visible: true})
		require.NoError(t, err)
	}
	messages, err := st.ListSessionMessages(ctx, run.UserID, run.SessionID, true)
	require.NoError(t, err)
	require.ErrorIs(t, engine.compact(ctx, ctx, &run, session, messages, model), store.ErrLeaseLost)
	cleared, err := st.GetSession(ctx, run.SessionID)
	require.NoError(t, err)
	require.Empty(t, cleared.PromptSnapshot)
	require.Empty(t, cleared.CompactSummary)
	fresh, err := st.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, store.StatusCancelled, fresh.Status)
	require.ErrorIs(t, engine.completeModelText(ctx, run, llm.Result{Text: "stale answer"}), store.ErrLeaseLost)
	events, err := st.ListEventsAfter(ctx, run.ID, 0)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, store.EventError, events[0].Type)
}

func TestDeleteHistoryFencesEveryOpenRunAndKeepsOtherUsers(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore()
	session, err := st.CreateSession(ctx, store.Session{UserID: 1, Status: store.SessionOpen})
	require.NoError(t, err)
	require.NoError(t, st.SaveThread(ctx, store.Thread{UserID: 1, SessionID: session.ID}))
	var runs []store.Run
	for _, source := range []string{store.SourceUser, store.SourceWatch, store.SourceMemoryReview} {
		for _, status := range []string{store.StatusQueued, store.StatusRunning, store.StatusWaitingInput, store.StatusWaitingConfirm} {
			run, err := st.InsertRun(ctx, store.Run{UserID: 1, SessionID: session.ID, Source: source, Status: status, LeaseOwner: "old-worker", LeaseGeneration: 1, LeaseUntilMs: store.NowMs() + 60000})
			require.NoError(t, err)
			runs = append(runs, run)
			_, err = st.Enqueue(ctx, store.QueueItem{UserID: 1, RunID: run.ID, MessageID: 10})
			require.NoError(t, err)
			if status == store.StatusWaitingConfirm {
				_, err = st.InsertConfirmation(ctx, store.Confirmation{RunID: run.ID, UserID: 1, CallID: "delete", Status: store.ConfirmPending})
				require.NoError(t, err)
			}
		}
	}
	other, err := st.InsertRun(ctx, store.Run{UserID: 2, Source: store.SourceUser, Status: store.StatusRunning})
	require.NoError(t, err)
	accept := &Acceptor{Store: st}
	require.NoError(t, accept.DeleteHistory(ctx, 1))
	require.NoError(t, accept.DeleteHistory(ctx, 1), "repeated deletion is idempotent")
	for _, run := range runs {
		current, err := st.GetRun(ctx, run.ID)
		require.NoError(t, err)
		require.Equal(t, store.StatusCancelled, current.Status)
		require.True(t, current.CancelRequested)
		require.ErrorIs(t, st.RunStep(ctx, run.Fence(), func(context.Context, store.Store) error { t.Fatal("stale writer entered"); return nil }), store.ErrLeaseLost)
		queue, err := st.ListQueue(ctx, run.ID)
		require.NoError(t, err)
		require.Empty(t, queue)
		events, err := st.ListEventsAfter(ctx, run.ID, 0)
		require.NoError(t, err)
		require.Len(t, events, 1, "only one terminal event per run")
		if run.Status == store.StatusWaitingConfirm {
			confirmation, err := st.GetConfirmation(ctx, run.ID, "delete")
			require.NoError(t, err)
			require.Equal(t, store.ConfirmExpired, confirmation.Status)
		}
	}
	preserved, err := st.GetRun(ctx, other.ID)
	require.NoError(t, err)
	require.Equal(t, other, *preserved)
}
