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

func TestExecuteChecksFinalOutputBudgetAndRetainsPartial(t *testing.T) {
	for _, completion := range []int64{0, 1, 20} {
		t.Run(itoa(completion), func(t *testing.T) {
			ctx := context.Background()
			st := store.NewMemoryStore()
			engine, run := newCancelTestEngine(t, st, scriptLLM{complete: func(_ context.Context, req llm.Request) (llm.Result, error) {
				if req.MaxTokens != 1 {
					t.Fatalf("remaining max_tokens=%d", req.MaxTokens)
				}
				return llm.Result{Text: "partial answer", Usage: llm.Usage{CompletionTokens: completion}}, nil
			}})
			run.OutputTokens = HardOutputTokens - 1
			if err := st.UpdateRun(ctx, run); err != nil {
				t.Fatal(err)
			}
			engine.Execute(ctx, run, false)
			fresh, _ := st.GetRun(ctx, run.ID)
			if completion == 0 {
				if fresh.Status != store.StatusDone {
					t.Fatalf("below budget: %+v", fresh)
				}
				return
			}
			if fresh.Status != store.StatusError || fresh.ErrorCode != "AGENT_RESOURCE_LIMIT" || fresh.OutputTokens != HardOutputTokens-1+completion {
				t.Fatalf("run=%+v", fresh)
			}
			messages, _ := st.ListSessionMessages(ctx, run.UserID, run.SessionID, true)
			if len(messages) != 1 || messages[0].Content != "partial answer" {
				t.Fatalf("partial messages=%+v", messages)
			}
			events, _ := st.ListEventsAfter(ctx, run.ID, 0)
			var terminal store.EventPayload
			if err := json.Unmarshal(events[len(events)-1].PayloadJSON, &terminal); err != nil {
				t.Fatal(err)
			}
			if terminal.Partial != "partial answer" || terminal.ErrorCode != "AGENT_RESOURCE_LIMIT" {
				t.Fatalf("terminal=%+v", terminal)
			}
		})
	}
}

func TestExecuteStopsBeforeToolsWhenModelCrossesBudget(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore()
	memories := memory.NewMapStore()
	engine, run := newCancelTestEngine(t, st, scriptLLM{complete: func(context.Context, llm.Request) (llm.Result, error) {
		return llm.Result{Usage: llm.Usage{CompletionTokens: 20}, ToolCalls: []llm.ToolCall{{ID: "must-not-write", Name: tool.AddMemory, Arguments: `{"target":"memory","content":"Should not be stored"}`}}}, nil
	}})
	run.OutputTokens = HardOutputTokens - 1
	if err := st.UpdateRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	registry, err := tool.NewRegistry(tool.Clients{Store: st, Memory: memories}, []string{tool.GetMemory, tool.AddMemory})
	if err != nil {
		t.Fatal(err)
	}
	engine.Tools = registry
	engine.Execute(ctx, run, false)
	entries, _ := memories.Active(ctx, run.UserID)
	calls, _ := st.ListToolCalls(ctx, run.ID)
	fresh, _ := st.GetRun(ctx, run.ID)
	if len(entries) != 0 || len(calls) != 0 || fresh.ErrorCode != "AGENT_RESOURCE_LIMIT" {
		t.Fatalf("entries=%+v calls=%+v run=%+v", entries, calls, fresh)
	}
}

func TestExecuteEnforcesToolBudgetWithinOneModelRound(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore()
	memories := memory.NewMapStore()
	engine, run := newCancelTestEngine(t, st, scriptLLM{complete: func(context.Context, llm.Request) (llm.Result, error) {
		return llm.Result{ToolCalls: []llm.ToolCall{
			{ID: "first", Name: tool.AddMemory, Arguments: `{"target":"memory","content":"First fact"}`},
			{ID: "second", Name: tool.AddMemory, Arguments: `{"target":"memory","content":"Second fact"}`},
		}}, nil
	}})
	run.ToolCalls = HardTools - 1
	if err := st.UpdateRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	registry, err := tool.NewRegistry(tool.Clients{Store: st, Memory: memories}, []string{tool.GetMemory, tool.AddMemory})
	if err != nil {
		t.Fatal(err)
	}
	engine.Tools = registry
	engine.Execute(ctx, run, false)
	entries, _ := memories.Active(ctx, run.UserID)
	fresh, _ := st.GetRun(ctx, run.ID)
	if len(entries) != 1 || entries[0].Content != "First fact" || fresh.ToolCalls != HardTools || fresh.ErrorCode != "AGENT_RESOURCE_LIMIT" {
		t.Fatalf("entries=%+v run=%+v", entries, fresh)
	}
	events, _ := st.ListEventsAfter(ctx, run.ID, 0)
	var terminal store.EventPayload
	if err := json.Unmarshal(events[len(events)-1].PayloadJSON, &terminal); err != nil {
		t.Fatal(err)
	}
	if terminal.Journal != tool.AddMemory {
		t.Fatalf("side effect summary=%+v", terminal)
	}
}

func TestCompactionEnforcesCumulativeBudgetBeforeCommittingSnapshot(t *testing.T) {
	for _, outcome := range []string{"limit", "no_gain"} {
		t.Run(outcome, func(t *testing.T) {
			ctx := context.Background()
			st := store.NewMemoryStore()
			model := scriptLLM{complete: func(_ context.Context, req llm.Request) (llm.Result, error) {
				if outcome == "limit" && req.MaxTokens != 1 {
					t.Fatalf("max_tokens=%d", req.MaxTokens)
				}
				text := "short summary"
				if outcome == "no_gain" {
					text = strings.Repeat("summary ", 20_000)
				}
				return llm.Result{Text: text, Usage: llm.Usage{PromptTokens: 100, CompletionTokens: 20}}, nil
			}}
			engine, run := newCancelTestEngine(t, st, model)
			if outcome == "limit" {
				run.OutputTokens = HardOutputTokens - 1
			}
			if err := st.UpdateRun(ctx, run); err != nil {
				t.Fatal(err)
			}
			session, _ := st.GetSession(ctx, run.SessionID)
			session.CompactSummary = "prior decision"
			session.PromptSnapshot = prompt.EncodeSnapshot(prompt.BuildSnapshot(nil, nil, session.CompactSummary))
			before := string(session.PromptSnapshot)
			if err := st.UpdateSession(ctx, *session); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 10; i++ {
				if _, err := st.InsertMessage(ctx, store.Message{UserID: run.UserID, SessionID: run.SessionID, Role: store.RoleUser, Kind: store.KindMessage, Content: strings.Repeat("history ", 500), Visible: true}); err != nil {
					t.Fatal(err)
				}
			}
			messages, _ := st.ListSessionMessages(ctx, run.UserID, run.SessionID, true)
			err := engine.compact(ctx, ctx, &run, session, messages, model)
			want := errCompactNoGain
			if outcome == "limit" {
				want = errRunTerminated
			}
			if !errors.Is(err, want) {
				t.Fatalf("compact err=%v want=%v", err, want)
			}
			updated, _ := st.GetSession(ctx, run.SessionID)
			if string(updated.PromptSnapshot) != before {
				t.Fatal("failed compact changed snapshot")
			}
			fresh, _ := st.GetRun(ctx, run.ID)
			if fresh.InputTokens != 100 || fresh.OutputTokens != run.OutputTokens || fresh.Rounds != 1 {
				t.Fatalf("usage not committed: %+v", fresh)
			}
			if outcome == "limit" && fresh.ErrorCode != "AGENT_RESOURCE_LIMIT" {
				t.Fatalf("run=%+v", fresh)
			}
		})
	}
}

func TestReviewInputBudgetIsCheckedBeforeAndAfterModel(t *testing.T) {
	for _, remaining := range []int64{1, 10_000} {
		t.Run(itoa(remaining), func(t *testing.T) {
			ctx := context.Background()
			st := store.NewMemoryStore()
			calls := 0
			engine, run := newCancelTestEngine(t, st, scriptLLM{complete: func(context.Context, llm.Request) (llm.Result, error) {
				calls++
				return llm.Result{Text: "review", Usage: llm.Usage{PromptTokens: remaining + 1}}, nil
			}})
			run.Source = store.SourceMemoryReview
			run.InputTokens = ReviewMaxInput - remaining
			if err := st.UpdateRun(ctx, run); err != nil {
				t.Fatal(err)
			}
			registry, err := tool.NewRegistry(tool.Clients{Store: st, Memory: memory.NewMapStore()}, []string{tool.GetMemory})
			if err != nil {
				t.Fatal(err)
			}
			engine.Tools = registry
			engine.Execute(ctx, run, false)
			fresh, _ := st.GetRun(ctx, run.ID)
			wantCalls := 1
			if remaining == 1 {
				wantCalls = 0
			}
			if calls != wantCalls || fresh.ErrorCode != "AGENT_RESOURCE_LIMIT" {
				t.Fatalf("calls=%d run=%+v", calls, fresh)
			}
		})
	}
}

type budgetStream struct {
	scriptLLM
}

func (budgetStream) RouteID() string         { return "primary" }
func (budgetStream) ModelName() string       { return "budget-stream" }
func (budgetStream) Boundary() string        { return "same" }
func (budgetStream) SupportsStreaming() bool { return true }

func (budgetStream) CompleteStream(_ context.Context, req llm.Request, emit func(llm.Delta) error) (llm.Result, error) {
	if req.MaxTokens != 1 {
		return llm.Result{}, errors.New("output was not limited to the remaining budget")
	}
	if err := emit(llm.Delta{Text: "streamed partial"}); err != nil {
		return llm.Result{}, err
	}
	return llm.Result{Text: "streamed partial", Usage: llm.Usage{CompletionTokens: 20}}, nil
}

func TestOutputBudgetPreservesOneStreamedPartialWithoutDone(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore()
	engine, run := newCancelTestEngine(t, st, budgetStream{})
	run.OutputTokens = HardOutputTokens - 1
	if err := st.UpdateRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	engine.Execute(ctx, run, false)
	fresh, _ := st.GetRun(ctx, run.ID)
	if fresh.ErrorCode != "AGENT_RESOURCE_LIMIT" {
		t.Fatalf("run=%+v", fresh)
	}
	events, _ := st.ListEventsAfter(ctx, run.ID, 0)
	tokens, failures := 0, 0
	for _, event := range events {
		switch event.Type {
		case store.EventDone:
			t.Fatal("budget overrun emitted done")
		case store.EventToken:
			tokens++
		case store.EventError:
			failures++
			var payload store.EventPayload
			if err := json.Unmarshal(event.PayloadJSON, &payload); err != nil {
				t.Fatal(err)
			}
			if payload.Partial != "streamed partial" || payload.StreamID == "" {
				t.Fatalf("partial=%+v", payload)
			}
		}
	}
	if tokens != 1 || failures != 1 {
		t.Fatalf("tokens=%d errors=%d", tokens, failures)
	}
}

func TestResearchToolsCannotCommitSuccessAtHardToolLimit(t *testing.T) {
	for _, name := range []string{tool.PublishAnswer, tool.AskQuestions} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			st := store.NewMemoryStore()
			engine, run := newCancelTestEngine(t, st, &scriptedLLM{})
			run.ToolCalls = HardTools - 1
			if err := st.UpdateRun(ctx, run); err != nil {
				t.Fatal(err)
			}
			call := llm.ToolCall{ID: "last-call", Name: name, Arguments: `{}`}
			turn := prompt.Turn{Role: store.RoleAssistant, ToolCalls: []prompt.ToolCall{{ID: call.ID, Name: name, Arguments: call.Arguments}}}
			if err := engine.recordModelToolStep(ctx, run, turn, nil); err != nil {
				t.Fatal(err)
			}
			if _, _, err := engine.startToolStep(ctx, run, call, "digest", false); err != nil {
				t.Fatal(err)
			}
			var err error
			if name == tool.PublishAnswer {
				err = engine.publishAnswer(ctx, &run, call, store.AnswerPresentation{RunID: run.ID, Blocks: []store.AnswerBlock{{Kind: "context", Text: "partial context"}}})
			} else {
				err = engine.waitForQuestions(ctx, &run, call, store.QuestionRequest{RunID: run.ID})
			}
			if !errors.Is(err, errRunTerminated) {
				t.Fatalf("termination=%v", err)
			}
			fresh, _ := st.GetRun(ctx, run.ID)
			if fresh.ErrorCode != "AGENT_RESOURCE_LIMIT" || fresh.ToolCalls != HardTools {
				t.Fatalf("run=%+v", fresh)
			}
			events, _ := st.ListEventsAfter(ctx, run.ID, 0)
			for _, event := range events {
				if event.Type == store.EventAnswerCommitted || event.Type == store.EventQuestionsRequired || event.Type == store.EventDone {
					t.Fatalf("successful event at limit: %s", event.Type)
				}
			}
		})
	}
}
