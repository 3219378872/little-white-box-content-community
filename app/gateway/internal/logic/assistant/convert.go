package assistant

import (
	"encoding/json"
	"esx/app/assistant/rpc/assistantservice"
	"esx/app/gateway/internal/types"
)

// mapThread 把助手会话摘要映射为 REST 结构；追问状态以 JSON 透传后在此解码。
func mapThread(in *assistantservice.AssistantThread) types.AssistantThread {
	if in == nil {
		return types.AssistantThread{}
	}
	return types.AssistantThread{
		QuestionRequest: decodeResearch[types.AssistantQuestionRequest](in.QuestionRequestJson),
		SessionId:       in.SessionId, UnreadCount: in.UnreadCount, LastMessageId: in.LastMessageId,
		LastMessagePreview: in.LastMessagePreview, LastMessageAtMs: in.LastMessageAtMs,
		ActiveRunId: in.ActiveRunId, ActiveRunStatus: in.ActiveRunStatus, ActiveRunPhase: in.ActiveRunPhase,
	}
}

// mapMessage 把助手消息映射为 REST 结构，解码追问与回答展示两个 JSON 字段。
func mapMessage(in *assistantservice.AssistantMessage) types.AssistantMessage {
	if in == nil {
		return types.AssistantMessage{}
	}
	return types.AssistantMessage{
		QuestionRequest:    decodeResearch[types.AssistantQuestionRequest](in.QuestionRequestJson),
		AnswerPresentation: decodeResearch[types.AssistantAnswerPresentation](in.AnswerPresentationJson),
		Id:                 in.Id, SessionId: in.SessionId, RunId: in.RunId, Role: in.Role, Kind: in.Kind,
		Content: in.Content, Unread: in.Unread, CreatedAtMs: in.CreatedAtMs, ChangeId: in.ChangeId,
	}
}

// mapMemory 把记忆条目映射为 REST 结构；nil 返回零值。
func mapMemory(in *assistantservice.MemoryEntry) types.AssistantMemoryEntry {
	if in == nil {
		return types.AssistantMemoryEntry{}
	}
	return types.AssistantMemoryEntry{
		Id: in.Id, Target: in.Target, Content: in.Content, Version: in.Version,
		CreatedAtMs: in.CreatedAtMs, UpdatedAtMs: in.UpdatedAtMs,
	}
}

// mapRunEvent 把运行事件映射为 SSE 负载；工具调用与来源卡片仅在存在时输出。
func mapRunEvent(in *assistantservice.RunEvent) *types.AssistantRunEvent {
	if in == nil {
		return nil
	}
	out := &types.AssistantRunEvent{
		QuestionRequest:    decodeResearch[types.AssistantQuestionRequest](in.QuestionRequestJson),
		AnswerPresentation: decodeResearch[types.AssistantAnswerPresentation](in.AnswerPresentationJson),
		RunId:              in.RunId, Seq: in.Seq, Type: in.Type, Text: in.Text, Degraded: in.Degraded,
		ErrorCode: in.ErrorCode, SessionId: in.SessionId, ChangeId: in.ChangeId, StreamId: in.StreamId,
	}
	if in.ToolCall != nil {
		out.ToolCall = &types.AssistantToolCallInfo{
			CallId: in.ToolCall.CallId, Tool: in.ToolCall.Tool, Summary: in.ToolCall.Summary, PayloadJson: in.ToolCall.PayloadJson,
		}
	}
	if in.SourceCard != nil {
		out.SourceCard = &types.AssistantSourceCard{
			Handle: in.SourceCard.Handle, Kind: in.SourceCard.Kind, AuthorityId: in.SourceCard.AuthorityId,
			Title: in.SourceCard.Title, Revision: in.SourceCard.Revision, PayloadJson: in.SourceCard.PayloadJson,
		}
	}
	return out
}

// decodeResearch 解码 RPC 中以字符串透传的 JSON 字段；空串或格式错误都视为缺失。
func decodeResearch[T any](raw string) *T {
	if raw == "" {
		return nil
	}
	var out T
	if json.Unmarshal([]byte(raw), &out) != nil {
		return nil
	}
	return &out
}
