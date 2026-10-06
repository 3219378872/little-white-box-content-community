package runtime

import (
	"context"
	"errors"
	"sort"

	"esx/app/assistant/internal/llm"
	"esx/app/assistant/internal/prompt"
	"esx/app/assistant/internal/store"
	"esx/app/assistant/internal/tool"
)

// clientProtocol 返回 run 的客户端协议版本，旧数据视为 v1。
func clientProtocol(run store.Run) int {
	return max(1, run.ClientProtocolVersion)
}

// closePendingCalls 为 run 终止时仍未完成的工具调用补写结果，使会话记录保持调用与结果成对。
func closePendingCalls(ctx context.Context, tx store.Store, run store.Run, status string) error {
	pending := map[string]prompt.ToolCall{}
	if run.Source != store.SourceMemoryReview {
		messages, err := tx.ListSessionMessages(ctx, run.UserID, run.SessionID, true)
		if err != nil {
			return err
		}
		for _, message := range messages {
			if message.RunID != run.ID || message.DeletedAtMs != 0 {
				continue
			}
			turn, ok := prompt.DecodeTurn(message.APIContent)
			if !ok {
				continue
			}
			for _, call := range turn.ToolCalls {
				pending[call.ID] = call
			}
			if turn.ToolCallID != "" {
				delete(pending, turn.ToolCallID)
			}
		}
	}
	text := string(mustJSON(map[string]string{"status": status, "reason": "run terminated before this call completed"}))
	ids := make([]string, 0, len(pending))
	for id := range pending {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		call := pending[id]
		turn := prompt.Turn{Role: store.RoleTool, ToolCallID: id, Name: call.Name, Content: text}
		if _, err := tx.InsertMessage(ctx, store.Message{UserID: run.UserID, SessionID: run.SessionID, RunID: run.ID, Role: store.RoleTool, Kind: store.KindTool, Content: text, APIContent: prompt.EncodeTurn(turn), Visible: false, CreatedAtMs: store.NowMs()}); err != nil {
			return err
		}
	}
	calls, err := tx.ListToolCalls(ctx, run.ID)
	if err != nil {
		return err
	}
	resultJSON := encodeToolResultJSONWithChanges(text, errors.New("run terminated before this call completed"), nil)
	for _, call := range calls {
		if call.ResultJSON != "" {
			continue
		}
		if err := tx.UpdateToolCall(ctx, store.ToolCall{RunID: run.ID, CallID: call.CallID, Status: status, ResultJSON: resultJSON}); err != nil {
			return err
		}
		if call.CanonicalArgsDigest == "" {
			continue
		}
		journal, err := tx.GetJournal(ctx, run.UserID, run.RequestID, call.Tool, call.CanonicalArgsDigest)
		if err != nil {
			return err
		}
		if journal != nil && journal.Status == store.JournalPending && journal.RunID == run.ID && journal.LeaseGeneration == run.LeaseGeneration {
			if err := tx.CompleteJournal(ctx, journal.ID, store.JournalError, resultJSON); err != nil {
				return err
			}
		}
	}
	return nil
}

// requiresExclusiveRound 报告本轮是否包含必须单独执行的工具（追问或发布回答）。
func requiresExclusiveRound(calls []llm.ToolCall) bool {
	for _, call := range calls {
		if call.Name == tool.AskQuestions || call.Name == tool.PublishAnswer {
			return true
		}
	}
	return false
}

// publishAnswer 以已校验的检索回答结束 run：回答作为终态消息发布，
// publish_answer 工具调用的结果在同一事务内补写。回答经 answer_committed 事件下发，不补发 token。
func (e *Engine) publishAnswer(ctx context.Context, run *store.Run, call llm.ToolCall, answer store.AnswerPresentation) error {
	text := tool.AnswerText(&answer)
	run.ToolCalls++
	err := e.finishMessage(ctx, *run, terminalOutcome{
		status: store.StatusDone, eventType: store.EventDone, payload: store.EventPayload{Answer: &answer, Text: text},
		message: text, apiContent: prompt.EncodeTurn(prompt.Turn{Role: store.RoleAssistant, Content: text}),
		before: func(ctx context.Context, tx store.Store) error {
			result := "回答和来源已发布。"
			if err := tx.UpdateToolCall(ctx, store.ToolCall{RunID: run.ID, CallID: call.ID, Status: "success", ResultJSON: encodeToolResultJSONWithChanges(result, nil, nil)}); err != nil {
				return err
			}
			turn := prompt.Turn{Role: store.RoleTool, Name: call.Name, ToolCallID: call.ID, Content: result}
			if _, err := tx.InsertMessage(ctx, store.Message{UserID: run.UserID, SessionID: run.SessionID, RunID: run.ID, Role: store.RoleTool, Kind: store.KindTool, Content: result, APIContent: prompt.EncodeTurn(turn), Visible: false, CreatedAtMs: store.NowMs()}); err != nil {
				return err
			}
			_, err := AppendEvent(ctx, tx, nil, *run, store.EventToolResult, store.EventPayload{ToolCall: &store.ToolInfo{CallID: call.ID, Tool: call.Name, Summary: "success"}})
			return err
		},
	})
	if err != nil {
		return err
	}
	return errRunTerminated
}
