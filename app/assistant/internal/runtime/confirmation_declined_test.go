package runtime

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/kitex/client/callopt"
	"github.com/stretchr/testify/require"

	"esx/app/assistant/internal/llm"
	"esx/app/assistant/internal/store"
	"esx/app/assistant/internal/tool"
	"esx/app/content/rpc/contentservice"
)

// deletableContent owns post 9 at revision 7 for user 1 and counts delete attempts.
type deletableContent struct {
	contentservice.ContentService
	deletes int
}

func (c *deletableContent) GetPost(context.Context, *contentservice.GetPostReq, ...callopt.Option) (*contentservice.GetPostResp, error) {
	return &contentservice.GetPostResp{Post: &contentservice.PostInfo{Id: 9, AuthorId: 1, Revision: 7, Status: 1}}, nil
}

func (c *deletableContent) DeletePost(context.Context, *contentservice.DeletePostReq, ...callopt.Option) (*contentservice.DeletePostResp, error) {
	c.deletes++
	return &contentservice.DeletePostResp{}, nil
}

// parkedDeleteRun runs a delete_post request until it waits for confirmation.
func parkedDeleteRun(t *testing.T) (*store.MemoryStore, *Engine, *deletableContent, *scriptedLLM, store.Run) {
	t.Helper()
	ctx := context.Background()
	mem := store.NewMemoryStore()
	accepted, err := (&Acceptor{Store: mem}).Accept(ctx, AcceptInput{UserID: 1, Message: "删掉帖子 9", RequestID: "delete-flow", ConsentOK: true, ConsentVersion: 3, ClientProtocolVersion: 2})
	require.NoError(t, err)
	content := &deletableContent{}
	reg, err := tool.NewRegistry(tool.Clients{Store: mem, Content: content}, []string{tool.DeletePost})
	require.NoError(t, err)
	script := &scriptedLLM{replies: []llm.Result{
		{ToolCalls: []llm.ToolCall{{ID: "delete-call", Name: tool.DeletePost, Arguments: `{"post_id":9}`}}},
		{Text: "好的，帖子保留不动。"},
	}}
	engine := &Engine{Store: mem, Tools: reg, LLM: script, Window: 128000}
	run, err := mem.Claim(ctx, "worker", store.NowMs(), 60_000)
	require.NoError(t, err)
	require.Equal(t, accepted.RunID, run.ID)
	engine.Execute(ctx, *run, false)
	waiting, err := mem.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, store.StatusWaitingConfirm, waiting.Status)
	return mem, engine, content, script, *waiting
}

// resumeDeclined claims the requeued run, executes it and checks the declined delete reached the model.
func resumeDeclined(t *testing.T, mem *store.MemoryStore, engine *Engine, content *deletableContent, script *scriptedLLM, runID int64, outcome, text string) {
	t.Helper()
	ctx := context.Background()
	resumed, err := mem.Claim(ctx, "worker-2", store.NowMs(), 60_000)
	require.NoError(t, err)
	require.NotNil(t, resumed)
	require.Equal(t, runID, resumed.ID)
	engine.Execute(ctx, *resumed, false)

	require.Zero(t, content.deletes, "a declined confirmation must never delete")
	final, err := mem.GetRun(ctx, runID)
	require.NoError(t, err)
	require.Equal(t, store.StatusDone, final.Status, "the model must get to finish the run")

	require.Len(t, script.reqs, 2)
	var toolTurn string
	for _, turn := range script.reqs[1].Messages {
		if turn.Role == store.RoleTool && turn.ToolCallID == "delete-call" {
			toolTurn = turn.Content
		}
	}
	require.Equal(t, text, toolTurn)

	events, err := mem.ListEventsAfter(ctx, runID, 0)
	require.NoError(t, err)
	var summaries []string
	for _, event := range events {
		pbEvent := ToPB(event)
		require.NotEqual(t, "RUN_FAILED", pbEvent.ErrorCode)
		if event.Type == store.EventToolResult && pbEvent.ToolCall != nil && pbEvent.ToolCall.CallId == "delete-call" {
			summaries = append(summaries, pbEvent.ToolCall.Summary)
		}
	}
	require.Equal(t, []string{outcome}, summaries)
	calls, err := mem.ListToolCalls(ctx, runID)
	require.NoError(t, err)
	require.Len(t, calls, 1)
	require.Equal(t, outcome, calls[0].Status)
	require.True(t, strings.Contains(calls[0].ResultJSON, text))
}

func TestRejectedDeleteConfirmationReturnsToModel(t *testing.T) {
	mem, engine, content, script, run := parkedDeleteRun(t)
	require.NoError(t, Confirm(context.Background(), mem, 1, run.ID, "delete-call", false))
	resumeDeclined(t, mem, engine, content, script, run.ID, "rejected", "用户已拒绝删除该帖子，未执行删除。")
}

func TestExpiredDeleteConfirmationReturnsToModel(t *testing.T) {
	mem, engine, content, script, run := parkedDeleteRun(t)
	ctx := context.Background()
	require.NoError(t, ExpireConfirmationWaits(ctx, mem, store.NowMs()+(confirmationWait+time.Second).Milliseconds()))
	resumeDeclined(t, mem, engine, content, script, run.ID, "expired", "删除确认已过期，未执行删除。")
	// An expired confirmation stays invalid: a late approval cannot revive it.
	require.Error(t, Confirm(ctx, mem, 1, run.ID, "delete-call", true))
	require.Zero(t, content.deletes)
}
