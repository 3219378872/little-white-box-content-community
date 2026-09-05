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

func clientProtocol(run store.Run) int {
	if run.Source == store.SourceWatch {
		return 2
	}
	return max(1, run.ClientProtocolVersion)
}

func closePendingCalls(ctx context.Context, tx store.Store, run store.Run, status string) error {
	if run.Source == store.SourceMemoryReview {
		return nil
	}
	messages, err := tx.ListSessionMessages(ctx, run.UserID, run.SessionID, true)
	if err != nil {
		return err
	}
	pending := map[string]prompt.ToolCall{}
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
	ids := make([]string, 0, len(pending))
	for id := range pending {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		call := pending[id]
		text := string(mustJSON(map[string]string{"status": status, "reason": "run terminated before this call completed"}))
		turn := prompt.Turn{Role: store.RoleTool, ToolCallID: id, Name: call.Name, Content: text}
		if _, err := tx.InsertMessage(ctx, store.Message{UserID: run.UserID, SessionID: run.SessionID, RunID: run.ID, Role: store.RoleTool, Kind: store.KindTool, Content: text, APIContent: prompt.EncodeTurn(turn), Visible: false, CreatedAtMs: store.NowMs()}); err != nil {
			return err
		}
		existing, err := tx.GetToolCall(ctx, run.ID, id)
		if err != nil {
			return err
		}
		if existing != nil && existing.ResultJSON == "" {
			if err := tx.UpdateToolCall(ctx, store.ToolCall{RunID: run.ID, CallID: id, Status: status, ResultJSON: encodeToolResultJSONWithChanges(text, nil, nil)}); err != nil {
				return err
			}
		}
	}
	return nil
}

func requiresExclusiveRound(calls []llm.ToolCall) bool {
	for _, call := range calls {
		if call.Name == tool.AskQuestions || call.Name == tool.PublishAnswer {
			return true
		}
	}
	return false
}

func (e *Engine) publishAnswer(ctx context.Context, run *store.Run, call llm.ToolCall, answer store.AnswerPresentation) error {
	if run.Source == store.SourceWatch {
		if _, err := e.currentWatchHits(ctx, *run, decodeWatchRunPayload(run.QueuedPayload)); err != nil {
			if errors.Is(err, errNoVisibleWatchHits) {
				if err := e.dismissWatchRun(ctx, *run); err != nil {
					return err
				}
				return errRunTerminated
			}
			return err
		}
	}
	text := tool.AnswerText(&answer)
	run.ToolCalls++
	err := e.finishMessage(ctx, *run, store.StatusDone, store.EventDone, store.EventPayload{Answer: &answer, Text: text}, text,
		prompt.EncodeTurn(prompt.Turn{Role: store.RoleAssistant, Content: text}), false, "", func(ctx context.Context, tx store.Store) error {
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
		})
	if err != nil {
		return err
	}
	return errRunTerminated
}
