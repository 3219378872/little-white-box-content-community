package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"esx/app/assistant/internal/llm"
	"esx/app/assistant/internal/memory"
	"esx/app/assistant/internal/prompt"
	"esx/app/assistant/internal/store"
	"esx/app/assistant/internal/tool"
)

// Inject acceptance at the first durable step after Complete and its final
// input-version read. This exercises the real Execute loop, not a substitute.
type modelCommitBarrierStore struct {
	store.Store
	beforeCommit func()
}

func (s *modelCommitBarrierStore) RunStep(ctx context.Context, fence store.LeaseFence, fn func(context.Context, store.Store) error) error {
	if s.beforeCommit != nil {
		f := s.beforeCommit
		s.beforeCommit = nil
		f()
	}
	return s.Store.RunStep(ctx, fence, fn)
}

func TestModelResultCommitLosesToAcceptedRedirect(t *testing.T) {
	for _, tc := range []struct {
		name     string
		result   llm.Result
		problem  error
		toolName string
		stream   bool
	}{
		{name: "text", result: llm.Result{Text: "stale answer"}},
		{name: "incomplete", result: llm.Result{Text: "stale partial", IncompleteReason: "max_output_tokens"}},
		{name: "provider_error", result: llm.Result{Text: "stale partial"}, problem: errors.New("provider failure")},
		{name: "empty_error", problem: errors.New("provider failure")},
		{name: "streamed", result: llm.Result{Text: "stale streamed"}, stream: true},
		{name: "resource_limit", result: llm.Result{Text: "stale budget partial", Usage: llm.Usage{CompletionTokens: HardOutputTokens}}},
		{name: "memory", result: llm.Result{ToolCalls: []llm.ToolCall{{ID: "stale-call", Name: tool.AddMemory, Arguments: `{"target":"memory","content":"obsolete preference"}`}}}, toolName: tool.AddMemory},
		{name: "questions", result: llm.Result{ToolCalls: []llm.ToolCall{{ID: "stale-call", Name: tool.AskQuestions, Arguments: questionArgs}}}, toolName: tool.AskQuestions},
		{name: "structured_answer", result: llm.Result{ToolCalls: []llm.ToolCall{{ID: "stale-call", Name: tool.PublishAnswer, Arguments: `{"blocks":[]}`}}}, toolName: tool.PublishAnswer},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			st := store.NewMemoryStore()
			mem := memory.NewMapStore()
			accept := &Acceptor{Store: st}
			accepted, err := accept.Accept(ctx, AcceptInput{UserID: 1, Message: "original request", RequestID: "original", ClientProtocolVersion: 2, ConsentOK: true, ConsentVersion: 3})
			if err != nil {
				t.Fatal(err)
			}
			run, err := st.Claim(ctx, "worker", store.NowMs(), 60_000)
			if err != nil || run == nil {
				t.Fatalf("claim %v", err)
			}
			names := []string{tool.PresentSources}
			if tc.toolName != "" {
				names = append(names, tc.toolName)
			}
			registry, err := tool.NewRegistry(tool.Clients{Store: st, Memory: mem}, names)
			if err != nil {
				t.Fatal(err)
			}
			barrier := &modelCommitBarrierStore{Store: st}
			calls := 0
			engine := &Engine{Store: barrier, Memory: mem, Tools: registry, Window: 128000}
			respond := func(_ context.Context, req llm.Request, emit func(llm.Delta) error) (llm.Result, error) {
				calls++
				if calls == 1 {
					if emit != nil {
						if err := emit(llm.Delta{Text: tc.result.Text}); err != nil {
							return llm.Result{}, err
						}
					}
					barrier.beforeCommit = func() {
						out, err := accept.Accept(ctx, AcceptInput{UserID: 1, Message: "new requested answer", RequestID: "redirect", ClientProtocolVersion: 2, ConsentOK: true, ConsentVersion: 3})
						if err != nil || out.Disposition != store.DispositionRedirected || out.RunID != accepted.RunID {
							t.Fatalf("accept %+v %v", out, err)
						}
					}
					return tc.result, tc.problem
				}
				found := false
				for _, turn := range req.Messages {
					if strings.Contains(turn.Content, "new requested answer") {
						found = true
					}
				}
				if !found {
					t.Fatal("new model round omitted accepted input")
				}
				return llm.Result{Text: "fresh answer"}, nil
			}
			base := scriptLLM{complete: func(ctx context.Context, req llm.Request) (llm.Result, error) { return respond(ctx, req, nil) }}
			engine.LLM = base
			if tc.stream {
				engine.LLM = modelFenceStream{scriptLLM: base, stream: respond}
			}
			engine.Execute(ctx, *run, false)
			if calls != 2 {
				t.Fatalf("model calls=%d, want 2", calls)
			}
			fresh, err := st.GetRun(ctx, run.ID)
			if err != nil || fresh.Status != store.StatusDone || fresh.InputVersion != 2 {
				t.Fatalf("run %+v %v", fresh, err)
			}
			messages, err := st.ListSessionMessages(ctx, 1, run.SessionID, true)
			if err != nil {
				t.Fatal(err)
			}
			visible := []string{}
			for _, m := range messages {
				if m.Role == store.RoleAssistant {
					if len(m.APIContent) > 0 {
						turn, _ := prompt.DecodeTurn(m.APIContent)
						if len(turn.ToolCalls) > 0 {
							t.Fatal("stale tool intent committed")
						}
					}
					if m.Visible {
						visible = append(visible, m.Content)
					}
				}
			}
			if len(visible) != 1 || visible[0] != "fresh answer" {
				t.Fatalf("answers=%v", visible)
			}
			entries, err := mem.Active(ctx, 1)
			if err != nil || len(entries) != 0 {
				t.Fatalf("stale memory %+v %v", entries, err)
			}
			events, err := st.ListEventsAfter(ctx, run.ID, 0)
			if err != nil {
				t.Fatal(err)
			}
			terminals := 0
			reset := false
			for _, ev := range events {
				if ev.Type == store.EventDone || ev.Type == store.EventError {
					terminals++
				}
				var p store.EventPayload
				_ = json.Unmarshal(ev.PayloadJSON, &p)
				if ev.Type == store.EventResponseReset {
					reset = true
				}
				if ev.Type != store.EventToken && (strings.Contains(p.Text, "stale") || strings.Contains(p.Partial, "stale")) {
					t.Fatalf("stale event %+v", ev)
				}
			}
			if tc.stream && !reset {
				t.Fatal("losing stream not reset")
			}
			if terminals != 1 {
				t.Fatalf("terminal events=%d", terminals)
			}
		})
	}
}

type modelFenceStream struct {
	scriptLLM
	stream func(context.Context, llm.Request, func(llm.Delta) error) (llm.Result, error)
}

func (s modelFenceStream) CompleteStream(ctx context.Context, req llm.Request, emit func(llm.Delta) error) (llm.Result, error) {
	return s.stream(ctx, req, emit)
}

func TestCommittedModelToolStepKeepsSteerRecoverable(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore()
	mem := memory.NewMapStore()
	accept := &Acceptor{Store: st}
	_, err := accept.Accept(ctx, AcceptInput{UserID: 1, Message: "remember preference", RequestID: "original", ConsentOK: true, ConsentVersion: 3})
	if err != nil {
		t.Fatal(err)
	}
	run, err := st.Claim(ctx, "worker", store.NowMs(), 60_000)
	if err != nil || run == nil {
		t.Fatal(err)
	}
	registry, err := tool.NewRegistry(tool.Clients{Store: st, Memory: mem}, []string{tool.AddMemory})
	if err != nil {
		t.Fatal(err)
	}
	e := &Engine{Store: st, Memory: mem, Tools: registry, Window: 128000}
	run.Phase = store.PhaseToolExecuting
	turn := prompt.Turn{Role: store.RoleAssistant, ToolCalls: []prompt.ToolCall{{ID: "committed", Name: tool.AddMemory, Arguments: `{"target":"memory","content":"committed preference"}`}}}
	if err := e.recordModelToolStep(ctx, *run, turn, nil); err != nil {
		t.Fatal(err)
	}
	out, err := accept.Accept(ctx, AcceptInput{UserID: 1, Message: "steered new input", RequestID: "steer", ConsentOK: true, ConsentVersion: 3})
	if err != nil || out.Disposition != store.DispositionSteered {
		t.Fatalf("steer %+v %v", out, err)
	}
	e.LLM = scriptLLM{complete: func(_ context.Context, req llm.Request) (llm.Result, error) {
		found := false
		for _, turn := range req.Messages {
			if strings.Contains(turn.Content, "steered new input") {
				found = true
			}
		}
		if !found {
			t.Fatal("steered input missing from next model round")
		}
		return llm.Result{Text: "handled new input"}, nil
	}}
	e.Execute(ctx, *run, true)
	entries, err := mem.Active(ctx, 1)
	if err != nil || len(entries) != 1 || entries[0].Content != "committed preference" {
		t.Fatalf("entries %+v %v", entries, err)
	}
	journals, err := st.ListSuccessfulJournal(ctx, 1, run.RequestID)
	if err != nil || len(journals) != 1 {
		t.Fatalf("journal %+v %v", journals, err)
	}
	fresh, _ := st.GetRun(ctx, run.ID)
	if fresh.Status != store.StatusDone || fresh.InputVersion != 2 {
		t.Fatalf("run %+v", fresh)
	}
}

func (modelFenceStream) RouteID() string         { return "primary" }
func (modelFenceStream) ModelName() string       { return "fixture" }
func (modelFenceStream) Boundary() string        { return "same" }
func (modelFenceStream) SupportsStreaming() bool { return true }
