//go:build integration

package runtime

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"esx/app/assistant/internal/llm"
	"esx/app/assistant/internal/store"
	"esx/app/assistant/internal/tool"
	"esx/pkg/testutil"
	"github.com/go-sql-driver/mysql"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

var errPresentationWrite = errors.New("injected presentation write failure")

type presentationFailStore struct{ store.Store }

func (s presentationFailStore) RunStep(ctx context.Context, fence store.LeaseFence, fn func(context.Context, store.Store) error) error {
	return s.Store.RunStep(ctx, fence, func(ctx context.Context, tx store.Store) error { return fn(ctx, presentationFailStore{tx}) })
}
func (presentationFailStore) SavePresentation(context.Context, store.AnswerPresentation) error {
	return errPresentationWrite
}

func TestResearchSQLTransactions(t *testing.T) {
	env := testutil.SetupTestEnv(t, "xbh_assistant", testutil.SchemaPath("xbh_assistant.sql"))
	t.Cleanup(env.Close)
	ctx := context.Background()
	config, err := mysql.ParseDSN(env.MySQLDSN)
	if err != nil {
		t.Fatal(err)
	}
	config.MultiStatements = true
	connection, err := sql.Open("mysql", config.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	// Verify both upgrade and repeated application in the dedicated test database.
	if _, err := connection.ExecContext(ctx, `DROP TABLE agent_question_request,assistant_message_presentation,agent_source_evidence; ALTER TABLE agent_run DROP COLUMN client_protocol_version; ALTER TABLE agent_run_event DROP INDEX idx_run_type_seq; ALTER TABLE agent_source_ledger MODIFY COLUMN authority_id VARCHAR(64) NOT NULL;`); err != nil {
		t.Fatal(err)
	}
	patch, err := os.ReadFile(testutil.SchemaPath("patches/20260905_agent_research.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := connection.ExecContext(ctx, string(patch)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := connection.ExecContext(ctx, `CREATE DATABASE IF NOT EXISTS xbh_user;
		CREATE TABLE IF NOT EXISTS xbh_user.agent_capability_consent (user_id BIGINT PRIMARY KEY,granted TINYINT NOT NULL,consent_version INT NOT NULL);
		INSERT INTO xbh_user.agent_capability_consent VALUES (1,1,2),(2,1,2);`); err != nil {
		t.Fatal(err)
	}
	st := store.NewSQLStore(sqlx.NewSqlConnFromDB(env.DB))
	accept := &Acceptor{Store: st}
	accepted, err := accept.Accept(ctx, AcceptInput{UserID: 1, Message: "比较方案", RequestID: "sql-question", ClientProtocolVersion: 2, ConsentOK: true, ConsentVersion: 2})
	if err != nil {
		t.Fatal(err)
	}
	run, err := st.Claim(ctx, "sql-worker", store.NowMs(), 60000)
	if err != nil || run == nil || run.ID != accepted.RunID {
		t.Fatalf("claim=%+v err=%v", run, err)
	}
	registry, err := tool.NewRegistry(tool.Clients{Store: st}, []string{tool.AskQuestions})
	if err != nil {
		t.Fatal(err)
	}
	script := &scriptedLLM{replies: []llm.Result{{ToolCalls: []llm.ToolCall{{ID: "sql-ask", Name: tool.AskQuestions, Arguments: questionArgs}}}, {Text: "保留未知条件。"}}}
	engine := &Engine{Store: st, Tools: registry, LLM: script, Window: 128000}
	engine.Execute(ctx, *run, false)
	questions, err := st.ListQuestions(ctx, run.ID)
	if err != nil || len(questions) != 1 || questions[0].Status != "pending" {
		t.Fatalf("questions=%+v err=%v", questions, err)
	}
	var success atomic.Int32
	var wg sync.WaitGroup
	for _, id := range []string{"answer-a", "answer-b"} {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			if _, err := AnswerQuestions(ctx, st, nil, 1, run.ID, questions[0].ID, id, []store.QuestionAnswer{{QuestionID: "budget", Disposition: "unknown"}}); err == nil {
				success.Add(1)
			}
		}(id)
	}
	wg.Wait()
	if success.Load() != 1 {
		t.Fatalf("accepted %d concurrent answers", success.Load())
	}
	resumed, err := st.Claim(ctx, "sql-resume", store.NowMs(), 60000)
	if err != nil || resumed == nil {
		t.Fatal(err)
	}
	engine.Execute(ctx, *resumed, true)
	finished, err := st.GetRun(ctx, run.ID)
	if err != nil || finished.Status != store.StatusDone {
		t.Fatalf("finished=%+v err=%v", finished, err)
	}
	longURL := "https://example.com/" + strings.Repeat("reference", 40)
	if _, err := st.InsertSource(ctx, store.Source{RunID: run.ID, Handle: "long-url", Kind: "web", AuthorityID: longURL, PayloadJSON: "{}", CreatedAtMs: store.NowMs()}); err != nil {
		t.Fatal(err)
	}
	sources, err := st.GetSources(ctx, run.ID, []string{"long-url"})
	if err != nil || len(sources) != 1 || sources[0].AuthorityID != longURL {
		t.Fatalf("long URL did not round-trip: %v", err)
	}

	accepted, err = accept.Accept(ctx, AcceptInput{UserID: 2, Message: "给出结论", RequestID: "sql-publish", ClientProtocolVersion: 2, ConsentOK: true, ConsentVersion: 2})
	if err != nil {
		t.Fatal(err)
	}
	run, err = st.Claim(ctx, "sql-publisher", store.NowMs(), 60000)
	if err != nil || run == nil || run.ID != accepted.RunID {
		t.Fatalf("claim=%+v err=%v", run, err)
	}
	call := llm.ToolCall{ID: "publish", Name: tool.PublishAnswer}
	if _, err := st.InsertToolCall(ctx, store.ToolCall{RunID: run.ID, CallID: call.ID, Tool: call.Name, ArgsJSON: "{}", CanonicalArgsDigest: "test", Status: "running", CreatedAtMs: store.NowMs()}); err != nil {
		t.Fatal(err)
	}
	answer := store.AnswerPresentation{Version: 1, RunID: run.ID, Blocks: []store.AnswerBlock{{ID: "b1", Kind: "limitation", Text: "资料不足，暂不作确定判断。", Citations: []store.AnswerCitation{}}}, Sources: []store.ResearchSource{}}
	broken := &Engine{Store: presentationFailStore{st}}
	if err := broken.publishAnswer(ctx, run, call, answer); !errors.Is(err, errPresentationWrite) {
		t.Fatalf("publish error=%v", err)
	}
	var visible int
	if err := env.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM assistant_message WHERE run_id=? AND role='assistant' AND visible=1`, run.ID).Scan(&visible); err != nil || visible != 0 {
		t.Fatalf("partial answer visible=%d err=%v", visible, err)
	}
	fresh, err := st.GetRun(ctx, run.ID)
	if err != nil || fresh.Status != store.StatusRunning {
		t.Fatalf("failed transaction changed run=%+v err=%v", fresh, err)
	}
	if err := (&Engine{Store: st}).publishAnswer(ctx, fresh, call, answer); !errors.Is(err, errRunTerminated) {
		t.Fatalf("publish retry=%v", err)
	}
	msgs, err := st.ListMessages(ctx, 2, fresh.SessionID, 0, 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	var reply store.Message
	for _, msg := range msgs {
		if msg.Role == store.RoleAssistant {
			reply = msg
		}
	}
	presentation, err := st.GetPresentation(ctx, reply.ID)
	if err != nil || presentation == nil || presentation.MessageID != reply.ID {
		t.Fatalf("presentation=%+v err=%v", presentation, err)
	}
	events, err := st.ListEventsAfter(ctx, fresh.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	committed, done := 0, 0
	for _, event := range events {
		if event.Type == store.EventAnswerCommitted {
			committed++
		}
		if event.Type == store.EventDone {
			done++
		}
	}
	if committed != 1 || done != 1 {
		t.Fatalf("commits=%d terminal=%d", committed, done)
	}
	if err := accept.DeleteHistory(ctx, 2); err != nil {
		t.Fatal(err)
	}
	if presentation, err := st.GetPresentation(ctx, reply.ID); err != nil || presentation != nil {
		t.Fatalf("deleted presentation=%+v err=%v", presentation, err)
	}
}
