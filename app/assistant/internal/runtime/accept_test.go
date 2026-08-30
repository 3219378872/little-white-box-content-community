package runtime

import (
	"context"
	"strings"
	"testing"

	"esx/app/assistant/internal/prompt"
	"esx/app/assistant/internal/store"
	"esx/pkg/errx"
)

func TestAcceptDispositionAndFIFO(t *testing.T) {
	mem := store.NewMemoryStore()
	a := &Acceptor{Store: mem, Notify: store.NewMemoryNotifier(), MaxRunes: 2000}
	ctx := context.Background()
	first, err := a.Accept(ctx, AcceptInput{UserID: 1, Message: "hello", RequestID: "r1", ConsentOK: true, ConsentVersion: 2})
	if err != nil || first.Disposition != store.DispositionStarted {
		t.Fatalf("started: %+v err=%v", first, err)
	}
	run, _ := mem.GetRun(ctx, first.RunID)
	run.Status = store.StatusRunning
	run.Phase = store.PhaseModelRequest
	_ = mem.UpdateRun(ctx, *run)
	thread, _ := mem.GetThread(ctx, 1)
	thread.ActiveRunID = run.ID
	_ = mem.SaveThread(ctx, *thread)

	second, err := a.Accept(ctx, AcceptInput{UserID: 1, Message: "redirect", RequestID: "r2", ConsentOK: true, ConsentVersion: 2})
	if err != nil || second.Disposition != store.DispositionRedirected {
		t.Fatalf("redirect: %+v err=%v", second, err)
	}
	run.Phase = store.PhaseToolExecuting
	_ = mem.UpdateRun(ctx, *run)
	third, err := a.Accept(ctx, AcceptInput{UserID: 1, Message: "steer", RequestID: "r3", ConsentOK: true, ConsentVersion: 2})
	if err != nil || third.Disposition != store.DispositionSteered {
		t.Fatalf("steer: %+v err=%v", third, err)
	}
	run.Phase = store.PhaseCompact
	_ = mem.UpdateRun(ctx, *run)
	for i := 0; i < store.MaxInputQueue; i++ {
		got, err := a.Accept(ctx, AcceptInput{UserID: 1, Message: "queued", RequestID: "q" + itoa(int64(i)), ConsentOK: true, ConsentVersion: 2})
		if err != nil || got.Disposition != store.DispositionQueued {
			t.Fatalf("queue %d: %+v err=%v", i, got, err)
		}
	}
	if _, err := a.Accept(ctx, AcceptInput{UserID: 1, Message: "overflow", RequestID: "full", ConsentOK: true, ConsentVersion: 2}); !errx.Is(err, errx.AgentQueueFull) {
		t.Fatalf("want queue full, got %v", err)
	}
	if _, err := a.Accept(ctx, AcceptInput{UserID: 1, Message: "nope", RequestID: "x", ConsentOK: false}); !errx.Is(err, errx.AgentNotAuthorized) {
		t.Fatalf("consent: %v", err)
	}
}

func TestAcceptPersistsAttachmentAndContextForReplay(t *testing.T) {
	mem := store.NewMemoryStore()
	a := &Acceptor{Store: mem}
	got, err := a.Accept(context.Background(), AcceptInput{
		UserID: 1, Message: "use this", RequestID: "context-1", ConsentOK: true, ConsentVersion: 2,
		Attachments: []Attachment{{MediaID: 91, URL: "https://media.example/91"}}, ContextPostID: 77,
	})
	if err != nil {
		t.Fatal(err)
	}
	msg, err := mem.GetMessage(context.Background(), 1, got.MessageID)
	if err != nil {
		t.Fatal(err)
	}
	turn, ok := prompt.DecodeTurn(msg.APIContent)
	if !ok || !strings.Contains(turn.Content, `"context_post_id":77`) || !strings.Contains(turn.Content, `"media_id":91`) {
		t.Fatalf("api_content=%s turn=%+v", msg.APIContent, turn)
	}
	run, _ := mem.GetRun(context.Background(), got.RunID)
	sess := (&Engine{}).toolSession(*run)
	if sess.ContextPostID != 77 || len(sess.Attachments) != 1 || sess.Attachments[0].MediaID != 91 {
		t.Fatalf("tool session=%+v", sess)
	}
}

func TestAcceptRetryReplaysRedirectResultWithoutDuplicateMessage(t *testing.T) {
	ctx := context.Background()
	mem := store.NewMemoryStore()
	acceptor := &Acceptor{Store: mem}
	first, err := acceptor.Accept(ctx, AcceptInput{
		UserID: 1, Message: "first", RequestID: "request-1", ConsentOK: true, ConsentVersion: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := mem.Claim(ctx, "worker", store.NowMs(), 60_000)
	if err != nil || run == nil || run.ID != first.RunID {
		t.Fatalf("claim=%+v err=%v", run, err)
	}
	run.Phase = store.PhaseModelRequest
	if err := mem.UpdateRun(ctx, *run); err != nil {
		t.Fatal(err)
	}
	thread, _ := mem.GetThread(ctx, 1)
	thread.ActiveRunID = run.ID
	if err := mem.SaveThread(ctx, *thread); err != nil {
		t.Fatal(err)
	}
	input := AcceptInput{UserID: 1, Message: "redirect", RequestID: "request-2", ConsentOK: true, ConsentVersion: 2}
	accepted, err := acceptor.Accept(ctx, input)
	if err != nil || accepted.Disposition != store.DispositionRedirected {
		t.Fatalf("accepted=%+v err=%v", accepted, err)
	}
	replayed, err := acceptor.Accept(ctx, input)
	if err != nil || replayed != accepted {
		t.Fatalf("replayed=%+v accepted=%+v err=%v", replayed, accepted, err)
	}
	messages, err := mem.ListSessionMessages(ctx, 1, first.SessionID, true)
	if err != nil || len(messages) != 2 {
		t.Fatalf("messages=%+v err=%v", messages, err)
	}
}
