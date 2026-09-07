package runtime

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"esx/app/assistant/internal/llm"
	"esx/app/assistant/internal/store"
	"esx/app/assistant/internal/tool"
)

type overlappingLLM struct {
	scriptedLLM
	entered chan struct{}
	release chan struct{}
}

func (c *overlappingLLM) Complete(ctx context.Context, req llm.Request) (llm.Result, error) {
	c.entered <- struct{}{}
	select {
	case <-ctx.Done():
		return llm.Result{}, ctx.Err()
	case <-c.release:
	}
	for _, turn := range req.Messages {
		if turn.Role == store.RoleUser {
			return llm.Result{Text: "answer:" + turn.Content}, nil
		}
	}
	return llm.Result{}, fmt.Errorf("no user message")
}

func TestConcurrentExecutionsKeepRoundStateLocal(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	mem := store.NewMemoryStore()
	reg, err := tool.NewRegistry(tool.Clients{Store: mem}, []string{tool.PresentSources})
	if err != nil {
		t.Fatal(err)
	}
	acceptor := &Acceptor{Store: mem}
	for userID := int64(1); userID <= 2; userID++ {
		_, err := acceptor.Accept(ctx, AcceptInput{UserID: userID, Message: fmt.Sprintf("user-%d", userID), RequestID: fmt.Sprintf("request-%d", userID), ConsentOK: true, ConsentVersion: 2})
		if err != nil {
			t.Fatal(err)
		}
	}
	var runs []store.Run
	for range 2 {
		run, err := mem.Claim(ctx, "overlap-worker", store.NowMs(), 60_000)
		if err != nil || run == nil {
			t.Fatalf("claim: %v, %v", run, err)
		}
		runs = append(runs, *run)
	}
	client := &overlappingLLM{entered: make(chan struct{}, 2), release: make(chan struct{})}
	engine := &Engine{Store: mem, Tools: reg, LLM: client, Window: 128000}
	var wg sync.WaitGroup
	for _, run := range runs {
		wg.Go(func() { engine.Execute(ctx, run, false) })
	}
	for range runs {
		select {
		case <-client.entered:
		case <-ctx.Done():
			wg.Wait()
			t.Fatal("executions did not overlap")
		}
	}
	close(client.release)
	wg.Wait()
	for _, run := range runs {
		fresh, err := mem.GetRun(ctx, run.ID)
		if err != nil || fresh.Status != store.StatusDone {
			t.Fatalf("run: %+v, %v", fresh, err)
		}
		messages, err := mem.ListSessionMessages(ctx, run.UserID, run.SessionID, true)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, msg := range messages {
			if msg.Role == store.RoleAssistant && msg.Visible {
				found = true
				if msg.Content != fmt.Sprintf("answer:user-%d", run.UserID) {
					t.Fatalf("cross-run response: %+v", msg)
				}
			}
		}
		if !found {
			t.Fatalf("missing response for run %d", run.ID)
		}
	}
}

func TestMissingSessionTerminatesBeforeRoundPreparation(t *testing.T) {
	ctx := context.Background()
	mem := store.NewMemoryStore()
	_, run, _ := mustStartRun(t, mem, "missing session")
	run.SessionID = -1
	if err := mem.UpdateRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	client := &scriptedLLM{}
	engine := &Engine{Store: mem, LLM: client}
	engine.Execute(ctx, run, false)
	fresh, err := mem.GetRun(ctx, run.ID)
	if err != nil || fresh.Status != store.StatusError || fresh.ErrorCode != "SESSION_MISSING" {
		t.Fatalf("run: %+v, %v", fresh, err)
	}
	if len(client.reqs) != 0 {
		t.Fatal("model called after preparation terminated")
	}
}
