package runtime

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"esx/app/assistant/internal/llm"
	"esx/app/assistant/internal/store"
	"esx/app/assistant/internal/tool"
	"esx/app/assistant/rpc/xiaobaihe/assistant/pb"
	"esx/pkg/errx"
)

const questionArgs = `{"questions":[{"id":"budget","text":"预算范围？","selection":"single","options":[{"id":"low","label":"较低"},{"id":"high","label":"较高"}]}]}`

func questionRun(t *testing.T) (*store.MemoryStore, *Engine, store.Run, *scriptedLLM) {
	t.Helper()
	ctx := context.Background()
	mem := store.NewMemoryStore()
	accept := &Acceptor{Store: mem}
	accepted, err := accept.Accept(ctx, AcceptInput{UserID: 1, Message: "帮我比较方案", RequestID: "question-test", ConsentOK: true, ConsentVersion: 2, ClientProtocolVersion: 2})
	if err != nil {
		t.Fatal(err)
	}
	run, err := mem.Claim(ctx, "worker", store.NowMs(), 60000)
	if err != nil || run == nil || run.ID != accepted.RunID {
		t.Fatalf("claim=%+v %v", run, err)
	}
	reg, err := tool.NewRegistry(tool.Clients{Store: mem}, []string{tool.AskQuestions})
	if err != nil {
		t.Fatal(err)
	}
	script := &scriptedLLM{replies: []llm.Result{{ToolCalls: []llm.ToolCall{{ID: "ask1", Name: tool.AskQuestions, Arguments: questionArgs}}}, {Text: "预算未知，先按不同条件比较。"}}}
	engine := &Engine{Store: mem, Tools: reg, LLM: script, Window: 128000}
	engine.Execute(ctx, *run, false)
	waiting, err := mem.GetRun(ctx, run.ID)
	if err != nil || waiting.Status != store.StatusWaitingInput {
		t.Fatalf("run=%+v err=%v", waiting, err)
	}
	return mem, engine, *waiting, script
}

func TestQuestionsYieldResumeAndIdempotency(t *testing.T) {
	mem, engine, run, script := questionRun(t)
	ctx := context.Background()
	if next, _ := mem.Claim(ctx, "other", store.NowMs(), 60000); next != nil {
		t.Fatalf("waiting run was claimed: %+v", next)
	}
	questions, _ := mem.ListQuestions(ctx, run.ID)
	q := questions[0]
	answers := []store.QuestionAnswer{{QuestionID: "budget", Disposition: "unknown"}}
	if _, err := AnswerQuestions(ctx, mem, nil, 2, run.ID, q.ID, "answer1", answers); err == nil {
		t.Fatal("other user accepted")
	}
	for range 2 {
		result, err := AnswerQuestions(ctx, mem, nil, 1, run.ID, q.ID, "answer1", answers)
		if err != nil || result.Status != "answered" {
			t.Fatalf("result=%+v err=%v", result, err)
		}
	}
	if _, err := AnswerQuestions(ctx, mem, nil, 1, run.ID, q.ID, "answer2", answers); err == nil {
		t.Fatal("different submission accepted")
	}
	resumed, err := mem.Claim(ctx, "worker2", store.NowMs(), 60000)
	if err != nil || resumed == nil {
		t.Fatal(err)
	}
	engine.Execute(ctx, *resumed, true)
	finished, _ := mem.GetRun(ctx, run.ID)
	if finished.Status != store.StatusDone {
		t.Fatalf("finished=%+v", finished)
	}
	if !hasToolResult(script.reqs[1].Messages, "ask1", "unknown") {
		t.Fatal("real answer missing from tool result")
	}
	for _, turn := range script.reqs[1].Messages {
		if turn.Role == store.RoleAssistant && turn.Content == "预算范围？\n- 较低\n- 较高" {
			t.Fatal("display projection polluted provider history")
		}
	}
}

func TestQuestionExpiryAndCancelNeverSelectAnswers(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		t.Run(map[bool]string{false: "expiry", true: "cancel"}[cancel], func(t *testing.T) {
			mem, _, run, _ := questionRun(t)
			ctx := context.Background()
			questions, _ := mem.ListQuestions(ctx, run.ID)
			q := questions[0]
			now := q.DeadlineMs
			if cancel {
				now = store.NowMs()
				if err := mem.RequestCancel(ctx, 1, run.ID); err != nil {
					t.Fatal(err)
				}
			}
			if err := ResolveWaiting(ctx, mem, nil, run.ID, now); err != nil {
				t.Fatal(err)
			}
			questions, _ = mem.ListQuestions(ctx, run.ID)
			if len(questions[0].Answers) != 0 || questions[0].Status == "pending" {
				t.Fatalf("question=%+v", questions[0])
			}
			if _, err := AnswerQuestions(ctx, mem, nil, 1, run.ID, q.ID, "late", []store.QuestionAnswer{{QuestionID: "budget", Disposition: "unknown"}}); err == nil {
				t.Fatal("late answer reopened old run")
			}
			thread, _ := mem.GetThread(ctx, 1)
			if thread.ActiveRunID != 0 {
				t.Fatal("expired run occupies thread")
			}
		})
	}
}

func TestConcurrentQuestionAnswersAcceptOnce(t *testing.T) {
	mem, _, run, _ := questionRun(t)
	ctx := context.Background()
	questions, _ := mem.ListQuestions(ctx, run.ID)
	var wg sync.WaitGroup
	var success atomic.Int32
	for _, id := range []string{"a", "b"} {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			if _, err := AnswerQuestions(ctx, mem, nil, 1, run.ID, questions[0].ID, id, []store.QuestionAnswer{{QuestionID: "budget", Disposition: "skipped"}}); err == nil {
				success.Add(1)
			}
		}(id)
	}
	wg.Wait()
	if success.Load() != 1 {
		t.Fatalf("accepted=%d", success.Load())
	}
}

func TestQuestionAnswerValidation(t *testing.T) {
	questions := []store.Question{{ID: "q", Selection: "single", Options: []store.QuestionOption{{ID: "a"}, {ID: "b"}}}}
	for _, answer := range []store.QuestionAnswer{
		{QuestionID: "other", Disposition: "unknown"},
		{QuestionID: "q", Disposition: "answered"},
		{QuestionID: "q", Disposition: "unknown", SelectedOptionIDs: []string{"a"}},
		{QuestionID: "q", Disposition: "answered", SelectedOptionIDs: []string{"a", "b"}},
		{QuestionID: "q", Disposition: "answered", SelectedOptionIDs: []string{"forged"}},
	} {
		if _, err := ValidateAnswers(questions, []store.QuestionAnswer{answer}); err == nil {
			t.Fatalf("accepted %+v", answer)
		}
	}
}

func TestClearHistoryRemovesQuestionAndBlocksReplay(t *testing.T) {
	mem, _, run, _ := questionRun(t)
	ctx := context.Background()
	accept := &Acceptor{Store: mem}
	if err := accept.DeleteHistory(ctx, 1); err != nil {
		t.Fatal(err)
	}
	questions, err := mem.ListQuestions(ctx, run.ID)
	if err != nil || len(questions) != 0 {
		t.Fatalf("questions=%v err=%v", questions, err)
	}
	count := 0
	err = Subscribe(ctx, mem, nil, 1, run.ID, 0, func(*pb.RunEvent) error { count++; return nil })
	if !errx.Is(err, errx.NotFound) || count != 0 {
		t.Fatalf("deleted replay count=%d err=%v", count, err)
	}
}

func TestModernInputRetryRejectsChangedContext(t *testing.T) {
	ctx := context.Background()
	mem := store.NewMemoryStore()
	accept := &Acceptor{Store: mem}
	input := AcceptInput{UserID: 1, Message: "问题", RequestID: "modern-retry", ConsentOK: true, ConsentVersion: 2, ClientProtocolVersion: 2}
	first, err := accept.Accept(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := accept.Accept(ctx, input)
	if err != nil || first != second {
		t.Fatalf("retry=%+v err=%v", second, err)
	}
	input.ContextPostID = 9
	if _, err := accept.Accept(ctx, input); !errx.Is(err, errx.IdempotencyConflict) {
		t.Fatalf("changed command err=%v", err)
	}
}

func TestInteractiveRoundRejectsCompanionTools(t *testing.T) {
	ctx := context.Background()
	mem := store.NewMemoryStore()
	accept := &Acceptor{Store: mem}
	_, err := accept.Accept(ctx, AcceptInput{UserID: 1, Message: "问题", RequestID: "exclusive", ConsentOK: true, ConsentVersion: 2, ClientProtocolVersion: 2})
	if err != nil {
		t.Fatal(err)
	}
	run, err := mem.Claim(ctx, "worker", store.NowMs(), 60000)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := tool.NewRegistry(tool.Clients{Store: mem}, []string{tool.AskQuestions, tool.ReadSource})
	if err != nil {
		t.Fatal(err)
	}
	script := &scriptedLLM{replies: []llm.Result{{ToolCalls: []llm.ToolCall{{ID: "ask", Name: tool.AskQuestions, Arguments: questionArgs}, {ID: "read", Name: tool.ReadSource, Arguments: `{"handle":"not-executed"}`}}}, {Text: "请继续。"}}}
	(&Engine{Store: mem, Tools: registry, LLM: script, Window: 128000}).Execute(ctx, *run, false)
	questions, err := mem.ListQuestions(ctx, run.ID)
	if err != nil || len(questions) != 0 {
		t.Fatalf("unexpected question: %v %v", questions, err)
	}
	if len(script.reqs) != 2 || !hasToolResult(script.reqs[1].Messages, "ask", "exclusive") || !hasToolResult(script.reqs[1].Messages, "read", "exclusive") {
		t.Fatal("rejected round did not close every call")
	}
}
