package runtime

import (
	"context"
	"sync"
	"testing"
	"time"

	"esx/app/assistant/internal/llm"
	"esx/app/assistant/internal/prompt"
	"esx/app/assistant/internal/store"
	"esx/app/assistant/internal/tool"
)

type redirectLLM struct {
	mu           sync.Mutex
	calls        int
	firstStarted chan struct{}
	firstDone    chan struct{}
}

func (m *redirectLLM) Complete(ctx context.Context, _ llm.Request) (llm.Result, error) {
	m.mu.Lock()
	m.calls++
	call := m.calls
	m.mu.Unlock()
	if call == 1 {
		close(m.firstStarted)
		<-ctx.Done()
		close(m.firstDone)
		// A provider may race cancellation and still return a body. The runtime
		// must discard it after observing the newer input_version.
		return llm.Result{Text: "stale answer"}, nil
	}
	return llm.Result{Text: "fresh answer"}, nil
}

func (m *redirectLLM) SupportsTools() bool      { return true }
func (m *redirectLLM) WireAPI() string          { return llm.WireAPIResponses }
func (m *redirectLLM) MaxOutputTokens() int     { return 128 }
func (m *redirectLLM) ContextWindowTokens() int { return 128000 }

func TestRedirectCancelsAndDiscardsOldModelResponse(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	mem := store.NewMemoryStore()
	acceptor := &Acceptor{Store: mem, Notify: store.NewMemoryNotifier()}
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
	registry, err := tool.NewRegistry(tool.Clients{Store: mem}, []string{tool.PresentSources})
	if err != nil {
		t.Fatal(err)
	}
	model := &redirectLLM{firstStarted: make(chan struct{}), firstDone: make(chan struct{})}
	engine := &Engine{Store: mem, Tools: registry, LLM: model, Window: 128000}
	done := make(chan struct{})
	go func() {
		defer close(done)
		engine.Execute(ctx, *run, false)
	}()
	select {
	case <-model.firstStarted:
	case <-ctx.Done():
		t.Fatal("first model call did not start")
	}
	second, err := acceptor.Accept(ctx, AcceptInput{
		UserID: 1, Message: "second", RequestID: "request-2", ConsentOK: true, ConsentVersion: 2,
	})
	if err != nil || second.Disposition != store.DispositionRedirected || second.RunID != first.RunID {
		t.Fatalf("redirect=%+v err=%v", second, err)
	}
	select {
	case <-model.firstDone:
	case <-ctx.Done():
		t.Fatal("redirect did not cancel old model context")
	}
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("redirected run did not complete")
	}
	fresh, err := mem.GetRun(context.Background(), run.ID)
	if err != nil || fresh.Status != store.StatusDone || fresh.InputVersion != 2 {
		t.Fatalf("run=%+v err=%v", fresh, err)
	}
	messages, err := mem.ListSessionMessages(context.Background(), 1, run.SessionID, true)
	if err != nil {
		t.Fatal(err)
	}
	var stale, freshAnswers int
	for _, msg := range messages {
		if msg.Role != store.RoleAssistant || !msg.Visible {
			continue
		}
		if msg.Content == "stale answer" {
			stale++
		}
		if msg.Content == "fresh answer" {
			freshAnswers++
		}
	}
	if stale != 0 || freshAnswers != 1 {
		t.Fatalf("assistant messages stale=%d fresh=%d all=%+v", stale, freshAnswers, messages)
	}
}

type cancelAwareLLM struct {
	started chan struct{}
}

func (m cancelAwareLLM) Complete(ctx context.Context, _ llm.Request) (llm.Result, error) {
	close(m.started)
	<-ctx.Done()
	return llm.Result{}, ctx.Err()
}

func (cancelAwareLLM) SupportsTools() bool      { return true }
func (cancelAwareLLM) WireAPI() string          { return llm.WireAPIResponses }
func (cancelAwareLLM) MaxOutputTokens() int     { return 128 }
func (cancelAwareLLM) ContextWindowTokens() int { return 128000 }

func TestConsentRevocationCancelsInFlightRun(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	mem := store.NewMemoryStore()
	acceptor := &Acceptor{Store: mem}
	accepted, err := acceptor.Accept(ctx, AcceptInput{
		UserID: 1, Message: "long task", RequestID: "consent-run", ConsentOK: true, ConsentVersion: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := mem.Claim(ctx, "worker", store.NowMs(), 60_000)
	if err != nil || run == nil || run.ID != accepted.RunID {
		t.Fatalf("claim=%+v err=%v", run, err)
	}
	registry, err := tool.NewRegistry(tool.Clients{Store: mem}, []string{tool.PresentSources})
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	engine := &Engine{Store: mem, Tools: registry, LLM: cancelAwareLLM{started: started}, Window: 128000}
	done := make(chan struct{})
	go func() {
		defer close(done)
		engine.Execute(ctx, *run, false)
	}()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("model did not start")
	}
	mem.SetAgentConsent(1, 0)
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("revoked run did not stop")
	}
	fresh, err := mem.GetRun(context.Background(), run.ID)
	if err != nil || fresh.Status != store.StatusCancelled || !fresh.CancelRequested {
		t.Fatalf("run=%+v err=%v", fresh, err)
	}
}

func TestTerminalCommitProducesOneMessageOutboxAndTerminalEvent(t *testing.T) {
	mem := store.NewMemoryStore()
	ctx := context.Background()
	session, run, _ := mustStartRun(t, mem, "hello")
	registry, err := tool.NewRegistry(tool.Clients{Store: mem}, []string{tool.PresentSources})
	if err != nil {
		t.Fatal(err)
	}
	model := &scriptedLLM{replies: []llm.Result{{Text: "done", Raw: prompt.EncodeTurn(prompt.Turn{Role: store.RoleAssistant, Content: "done"})}}}
	engine := &Engine{Store: mem, Tools: registry, LLM: model, Window: 128000}
	engine.Execute(ctx, run, false)
	// A stale invocation cannot append another message or terminal event.
	engine.Execute(ctx, run, true)
	messages, _ := mem.ListSessionMessages(ctx, 1, session.ID, true)
	assistantMessages := 0
	for _, msg := range messages {
		if msg.Role == store.RoleAssistant && msg.Visible {
			assistantMessages++
		}
	}
	outbox, err := mem.ListUnpublishedOutbox(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	events, err := mem.ListEventsAfter(ctx, run.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	terminals := 0
	for _, event := range events {
		if event.Type == store.EventDone || event.Type == store.EventError {
			terminals++
		}
	}
	if assistantMessages != 1 || len(outbox) != 1 || terminals != 1 {
		t.Fatalf("messages=%d outbox=%d terminals=%d events=%+v", assistantMessages, len(outbox), terminals, events)
	}
}
