package runtime

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"esx/app/assistant/internal/canonical"
	"esx/app/assistant/internal/llm"
	"esx/app/assistant/internal/memory"
	"esx/app/assistant/internal/prompt"
	"esx/app/assistant/internal/store"
	"esx/app/assistant/internal/tool"
	"esx/pkg/errx"

	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/metric"
)

var (
	agentLLMCalls = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: "esx", Subsystem: "assistant_agent", Name: "llm_calls_total",
		Help: "Assistant agent LLM calls", Labels: []string{"outcome"},
	})
	agentToolCalls = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: "esx", Subsystem: "assistant_agent", Name: "tool_calls_total",
		Help: "Assistant agent tool calls", Labels: []string{"tool", "outcome"},
	})
	agentQueueAge = metric.NewHistogramVec(&metric.HistogramVecOpts{
		Namespace: "esx", Subsystem: "assistant_agent", Name: "queue_age_seconds",
		Help: "Age of claimed queued runs in seconds", Buckets: []float64{0.05, 0.2, 0.5, 1, 2, 5, 15, 30, 60},
	})
	agentLeaseRecover = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: "esx", Subsystem: "assistant_agent", Name: "lease_recover_total",
		Help: "Expired lease recoveries", Labels: []string{"source"},
	})
	agentFirstToken = metric.NewHistogramVec(&metric.HistogramVecOpts{
		Namespace: "esx", Subsystem: "assistant_agent", Name: "first_token_seconds",
		Help:    "Time from claim to first token event",
		Buckets: []float64{0.1, 0.25, 0.5, 1, 2, 5, 10},
	})
)

type Engine struct {
	Store    store.Store
	Memory   memory.Store
	Tools    *tool.Registry
	LLM      llm.Client
	Notify   store.Notifier
	Window   int
	Provider int
}

func (e *Engine) Execute(ctx context.Context, run store.Run, recovered bool) {
	if recovered {
		agentLeaseRecover.Inc(run.Source)
	}
	if run.CreatedAtMs > 0 {
		agentQueueAge.ObserveFloat(float64(store.NowMs()-run.CreatedAtMs) / 1000)
	}
	logger := logx.WithContext(ctx)
	if err := e.run(ctx, run); err != nil {
		logger.Errorw("assistant-agent run failed", logx.Field("runId", run.ID), logx.Field("err", err.Error()))
		if fresh, getErr := e.Store.GetRun(ctx, run.ID); getErr == nil && fresh != nil &&
			(fresh.Status == store.StatusRunning || fresh.Status == store.StatusQueued) {
			_ = e.fail(ctx, *fresh, "RUN_FAILED", err.Error())
		}
	}
}

func (e *Engine) run(ctx context.Context, run store.Run) error {
	session, err := e.Store.GetSession(ctx, run.SessionID)
	if err != nil {
		return e.fail(ctx, run, "SESSION_MISSING", err.Error())
	}
	snap, ok := prompt.DecodeSnapshot(session.PromptSnapshot)
	if !ok {
		var entries []memory.Entry
		if e.Memory != nil {
			entries, _ = e.Memory.Active(ctx, run.UserID)
		}
		snap = prompt.BuildSnapshot(entries, nil, session.CompactSummary)
		session.PromptSnapshot = prompt.EncodeSnapshot(snap)
		_ = e.Store.UpdateSession(ctx, *session)
	}
	if err := e.ensureStarted(ctx, run); err != nil {
		return err
	}
	started := time.Now()
	firstToken := false
	var reviewLive []prompt.Turn
	registry := tool.ForSource(e.Tools, run.Source, tool.CurrentConsentVersion)
	if registry == nil {
		return e.fail(ctx, run, "TOOLS_UNAVAILABLE", "no tools")
	}
	session.ToolSnapshot = prompt.EncodeTools(registry.Definitions())
	_ = e.Store.UpdateSession(ctx, *session)

	for {
		fresh, err := e.Store.GetRun(ctx, run.ID)
		if err != nil {
			return err
		}
		run = *fresh
		if run.CancelRequested {
			return e.finish(ctx, run, store.StatusCancelled, store.EventError, store.EventPayload{ErrorCode: "CANCELLED", Text: "run cancelled"})
		}
		now := store.NowMs()
		if HardLimitExceeded(run, now) {
			return e.resourceLimit(ctx, run)
		}
		convergence, _ := RecordAlarms(ctx, e.Store, run, now)

		msgs, _ := e.Store.ListSessionMessages(ctx, run.UserID, run.SessionID, true)
		if e.Window == 0 && e.LLM != nil {
			e.Window = e.LLM.ContextWindowTokens()
		}
		if ShouldCompact(msgs, e.Window) {
			if err := e.compact(ctx, &run, session, msgs); err != nil {
				return err
			}
			session, _ = e.Store.GetSession(ctx, run.SessionID)
			snap, _ = prompt.DecodeSnapshot(session.PromptSnapshot)
			continue
		}

		history := HistoryTurns(visibleForPrompt(msgs))
		if run.Source == store.SourceMemoryReview {
			history = append(history, reviewLive...)
		} else if open := unmatchedToolCalls(history); len(open) > 0 {
			for _, call := range open {
				if err := e.execTool(ctx, &run, registry, call, &reviewLive); err != nil {
					return err
				}
				if run.CancelRequested {
					return e.finish(ctx, run, store.StatusCancelled, store.EventError, store.EventPayload{ErrorCode: "CANCELLED"})
				}
			}
			msgs, _ = e.Store.ListSessionMessages(ctx, run.UserID, run.SessionID, true)
			history = HistoryTurns(visibleForPrompt(msgs))
		}
		snap.History = history
		turns := prompt.Messages(snap)
		seen := make(map[int64]struct{}, len(msgs))
		for _, msg := range msgs {
			if !msg.Compacted {
				seen[msg.ID] = struct{}{}
			}
		}
		turns = append(turns, e.pendingUserTurns(ctx, run, seen)...)

		run.Phase = store.PhaseModelRequest
		run.LastActivityAtMs = now
		_ = e.Store.UpdateRun(ctx, run)

		if e.LLM == nil {
			return e.fail(ctx, run, "LLM_DISABLED", "model is not configured")
		}
		result, err := e.LLM.Complete(ctx, llm.Request{
			Messages:    turns,
			Tools:       registry.Definitions(),
			MaxTokens:   SingleOutputLimit(e.LLM.MaxOutputTokens()),
			Convergence: convergence,
		})
		if err != nil {
			agentLLMCalls.Inc("failure")
			logx.WithContext(ctx).Errorw("assistant LLM complete failed",
				logx.Field("runId", run.ID), logx.Field("err", err.Error()))
			return e.fail(ctx, run, "LLM_UNAVAILABLE", "model call failed")
		}
		agentLLMCalls.Inc("success")
		run.Rounds++
		run.InputTokens += result.Usage.PromptTokens
		run.OutputTokens += result.Usage.CompletionTokens
		run.CacheTokens += result.Usage.CacheTokens
		run.CostUSD += result.Usage.CostUSD
		run.LastActivityAtMs = store.NowMs()
		_ = e.Store.UpdateRun(ctx, run)

		if len(result.ToolCalls) == 0 {
			text := strings.TrimSpace(result.Text)
			if !firstToken {
				agentFirstToken.ObserveFloat(time.Since(started).Seconds())
				firstToken = true
			}
			_, _ = AppendEvent(ctx, e.Store, e.Notify, run, store.EventToken, store.EventPayload{Text: text})
			if run.Source != store.SourceMemoryReview {
				_, _ = e.Store.InsertMessage(ctx, store.Message{
					UserID: run.UserID, SessionID: run.SessionID, RunID: run.ID, Role: store.RoleAssistant,
					Kind: store.KindMessage, Content: text, APIContent: result.Raw, Visible: true,
					Unread: run.Source == store.SourceWatch, CreatedAtMs: store.NowMs(),
				})
				if run.Source == store.SourceWatch {
					thread, _ := e.Store.GetThread(ctx, run.UserID)
					if thread != nil {
						thread.UnreadCount++
						thread.LastMessagePreview = store.Preview(text, 80)
						thread.LastMessageAtMs = store.NowMs()
						_ = e.Store.SaveThread(ctx, *thread)
					}
				}
			}
			return e.finish(ctx, run, store.StatusDone, store.EventDone, store.EventPayload{Text: text})
		}

		calls := make([]llm.ToolCall, 0, len(result.ToolCalls))
		assistantTurn := prompt.Turn{Role: store.RoleAssistant, Content: strings.TrimSpace(result.Text)}
		for i, call := range result.ToolCalls {
			id := strings.TrimSpace(call.ID)
			if id == "" {
				id = "call_" + itoa(store.NowMs()) + "_" + itoa(int64(i+1))
			}
			args := canonical.UnwrapArgsJSON(call.Arguments)
			calls = append(calls, llm.ToolCall{ID: id, Name: call.Name, Arguments: args})
			assistantTurn.ToolCalls = append(assistantTurn.ToolCalls, prompt.ToolCall{ID: id, Name: call.Name, Arguments: args})
		}
		if err := e.recordTurn(ctx, run, assistantTurn, store.KindTool, false, &reviewLive); err != nil {
			return err
		}
		run.Phase = store.PhaseToolExecuting
		_ = e.Store.UpdateRun(ctx, run)
		for _, call := range calls {
			if err := e.execTool(ctx, &run, registry, call, &reviewLive); err != nil {
				return err
			}
			if run.CancelRequested {
				return e.finish(ctx, run, store.StatusCancelled, store.EventError, store.EventPayload{ErrorCode: "CANCELLED"})
			}
		}
	}
}

func visibleForPrompt(msgs []store.Message) []store.Message {
	out := make([]store.Message, 0, len(msgs))
	for _, msg := range msgs {
		if msg.Compacted {
			continue
		}
		out = append(out, msg)
	}
	return out
}

func (e *Engine) pendingUserTurns(ctx context.Context, run store.Run, seen map[int64]struct{}) []prompt.Turn {
	out := make([]prompt.Turn, 0)
	if len(run.QueuedPayload) > 0 {
		var payload struct {
			Text      string `json:"text"`
			MessageID int64  `json:"message_id"`
		}
		if json.Unmarshal(run.QueuedPayload, &payload) == nil && payload.Text != "" {
			if payload.MessageID == 0 {
				out = append(out, prompt.Turn{Role: store.RoleUser, Content: payload.Text})
			} else if _, ok := seen[payload.MessageID]; !ok {
				out = append(out, prompt.Turn{Role: store.RoleUser, Content: payload.Text})
			}
		}
	}
	items, _ := e.Store.ListQueue(ctx, run.ID)
	for _, item := range items {
		if _, ok := seen[item.MessageID]; ok {
			continue
		}
		msg, err := e.Store.GetMessage(ctx, run.UserID, item.MessageID)
		if err == nil && msg != nil {
			out = append(out, prompt.Turn{Role: store.RoleUser, Content: msg.Content})
		}
	}
	return out
}

func (e *Engine) execTool(ctx context.Context, run *store.Run, registry *tool.Registry, call llm.ToolCall, reviewLive *[]prompt.Turn) error {
	call.Arguments = canonical.UnwrapArgsJSON(call.Arguments)
	digest, _ := canonical.DigestArgs(call.Arguments)
	_, _ = e.Store.InsertToolCall(ctx, store.ToolCall{
		RunID: run.ID, CallID: call.ID, Tool: call.Name, ArgsJSON: call.Arguments,
		CanonicalArgsDigest: digest, Status: "running", CreatedAtMs: store.NowMs(),
	})
	_, _ = AppendEvent(ctx, e.Store, e.Notify, *run, store.EventToolCall, store.EventPayload{
		ToolCall: &store.ToolInfo{CallID: call.ID, Tool: call.Name, Summary: call.Name, PayloadJSON: call.Arguments},
	})
	if journal, err := e.Store.GetJournal(ctx, run.UserID, run.RequestID, call.Name, digest); err == nil && journal != nil && journal.Status == store.JournalSuccess {
		agentToolCalls.Inc(call.Name, "replay")
		text := decodeToolResultText(journal.ResultJSON)
		_ = e.Store.UpdateToolCall(ctx, store.ToolCall{RunID: run.ID, CallID: call.ID, Status: "success", ResultJSON: encodeToolResultJSON(text, nil)})
		if err := e.recordTurn(ctx, *run, prompt.Turn{Role: store.RoleTool, Content: text, ToolCallID: call.ID, Name: call.Name}, store.KindTool, false, reviewLive); err != nil {
			return err
		}
		_, _ = AppendEvent(ctx, e.Store, e.Notify, *run, store.EventToolResult, store.EventPayload{
			ToolCall: &store.ToolInfo{CallID: call.ID, Tool: call.Name, Summary: "replay", PayloadJSON: text},
			Text:     text,
		})
		return nil
	}
	if registry.HighRisk(call.Name) {
		if err := e.requireConfirm(ctx, run, call, digest); err != nil {
			return err
		}
	}
	sess := &tool.Session{UserID: run.UserID, SessionID: run.SessionID, RunID: run.ID, RequestID: run.RequestID, Source: run.Source}
	text, cards, err := registry.Call(ctx, sess, call.Name, call.ID, call.Arguments)
	outcome := "success"
	if err != nil {
		outcome = "unavailable"
		text = err.Error()
	}
	agentToolCalls.Inc(call.Name, outcome)
	run.ToolCalls++
	run.LastActivityAtMs = store.NowMs()
	_ = e.Store.UpdateRun(ctx, *run)
	resultJSON := encodeToolResultJSON(text, err)
	if sideEffect(call.Name) && err == nil {
		row, reserved, jerr := e.Store.ReserveJournal(ctx, store.Journal{
			UserID: run.UserID, RequestID: run.RequestID, Tool: call.Name, CanonicalArgsDigest: digest,
			Status: store.JournalSuccess, ResultJSON: resultJSON, CreatedAtMs: store.NowMs(),
		})
		if jerr == nil && reserved && row != nil {
			_ = e.Store.CompleteJournal(ctx, row.ID, store.JournalSuccess, resultJSON)
		}
	}
	_ = e.Store.UpdateToolCall(ctx, store.ToolCall{RunID: run.ID, CallID: call.ID, Status: outcome, ResultJSON: resultJSON})
	if recErr := e.recordTurn(ctx, *run, prompt.Turn{Role: store.RoleTool, Content: text, ToolCallID: call.ID, Name: call.Name}, store.KindTool, false, reviewLive); recErr != nil {
		return recErr
	}
	_, _ = AppendEvent(ctx, e.Store, e.Notify, *run, store.EventToolResult, store.EventPayload{
		ToolCall: &store.ToolInfo{CallID: call.ID, Tool: call.Name, Summary: call.Name, PayloadJSON: text},
		Text:     text,
	})
	for _, card := range cards {
		_, _ = AppendEvent(ctx, e.Store, e.Notify, *run, store.EventSourceCard, store.EventPayload{SourceCard: &card})
	}
	return nil
}

func (e *Engine) recordTurn(ctx context.Context, run store.Run, turn prompt.Turn, kind string, visible bool, reviewLive *[]prompt.Turn) error {
	if run.Source == store.SourceMemoryReview {
		if reviewLive != nil {
			*reviewLive = append(*reviewLive, turn)
		}
		return nil
	}
	_, err := e.Store.InsertMessage(ctx, store.Message{
		UserID: run.UserID, SessionID: run.SessionID, RunID: run.ID,
		Role: turn.Role, Kind: kind, Content: turn.Content, APIContent: prompt.EncodeTurn(turn),
		Visible: visible, CreatedAtMs: store.NowMs(),
	})
	return err
}

func unmatchedToolCalls(history []prompt.Turn) []llm.ToolCall {
	done := make(map[string]struct{})
	for _, turn := range history {
		if id := strings.TrimSpace(turn.ToolCallID); id != "" {
			done[id] = struct{}{}
		}
	}
	out := make([]llm.ToolCall, 0)
	seen := make(map[string]struct{})
	for _, turn := range history {
		for _, call := range turn.ToolCalls {
			id := strings.TrimSpace(call.ID)
			if id == "" {
				continue
			}
			if _, ok := done[id]; ok {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			out = append(out, llm.ToolCall{ID: id, Name: call.Name, Arguments: call.Arguments})
		}
	}
	return out
}

func encodeToolResultJSON(text string, callErr error) string {
	payload := map[string]any{"ok": callErr == nil, "text": text}
	if callErr != nil {
		payload["error"] = callErr.Error()
		if text == "" {
			payload["text"] = callErr.Error()
		}
	}
	raw, _ := json.Marshal(payload)
	return string(raw)
}

func decodeToolResultText(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var payload struct {
		Text string `json:"text"`
	}
	if json.Unmarshal([]byte(raw), &payload) == nil && payload.Text != "" {
		return payload.Text
	}
	var asString string
	if json.Unmarshal([]byte(raw), &asString) == nil && asString != "" {
		return asString
	}
	return raw
}

func sideEffect(name string) bool {
	switch name {
	case tool.CreatePost, tool.UpdatePost, tool.DeletePost, tool.AddMemory, tool.ReplaceMemory, tool.RemoveMemory, tool.BatchMemory,
		tool.CreateWatchTask, tool.UpdateWatchTask, tool.DeleteWatchTask:
		return true
	default:
		return false
	}
}

func (e *Engine) requireConfirm(ctx context.Context, run *store.Run, call llm.ToolCall, digest string) error {
	_, _ = e.Store.InsertConfirmation(ctx, store.Confirmation{
		UserID: run.UserID, SessionID: run.SessionID, RunID: run.ID, CallID: call.ID, Tool: call.Name,
		CanonicalArgsDigest: digest, Status: store.ConfirmPending, CreatedAtMs: store.NowMs(),
	})
	_, _ = AppendEvent(ctx, e.Store, e.Notify, *run, store.EventConfirmRequired, store.EventPayload{
		ToolCall: &store.ToolInfo{CallID: call.ID, Tool: call.Name, Summary: "确认删除帖子", PayloadJSON: call.Arguments},
	})
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		fresh, _ := e.Store.GetRun(ctx, run.ID)
		if fresh != nil && fresh.CancelRequested {
			run.CancelRequested = true
			return errx.NewWithCode(errx.AgentRunConflict)
		}
		conf, _ := e.Store.GetConfirmation(ctx, run.ID, call.ID)
		if conf != nil && conf.Status == store.ConfirmApproved {
			return nil
		}
		if conf != nil && conf.Status == store.ConfirmRejected {
			return errx.New(errx.PermissionDenied, "delete_post rejected")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	return errx.New(errx.ParamError, "delete_post confirmation expired")
}

func (e *Engine) compact(ctx context.Context, run *store.Run, session *store.Session, msgs []store.Message) error {
	run.Phase = store.PhaseCompact
	_ = e.Store.UpdateRun(ctx, *run)
	keep := EstimateMessageTokens(msgs) / 5
	if keep < 1 {
		keep = 1
	}
	selected := SelectKeep(msgs, keep, nil)
	summary := "压缩摘要：保留最近对话与未完成工具。"
	if e.LLM != nil {
		var b strings.Builder
		for _, msg := range msgs {
			if !msg.Visible {
				continue
			}
			b.WriteString(msg.Role)
			b.WriteString(": ")
			b.WriteString(store.Preview(msg.Content, 200))
			b.WriteByte('\n')
		}
		result, err := e.LLM.Complete(ctx, llm.Request{
			Messages:     []prompt.Turn{{Role: store.RoleSystem, Content: "用中文压缩以下会话，不要引入新事实。"}, {Role: store.RoleUser, Content: b.String()}},
			DisableTools: true,
			MaxTokens:    512,
		})
		if err == nil && strings.TrimSpace(result.Text) != "" {
			summary = result.Text
		}
	}
	keepIDs := make(map[int64]struct{}, len(selected))
	for _, msg := range selected {
		keepIDs[msg.ID] = struct{}{}
	}
	ids := make([]int64, 0, len(msgs))
	for _, msg := range msgs {
		if _, keep := keepIDs[msg.ID]; keep {
			continue
		}
		ids = append(ids, msg.ID)
	}
	if len(ids) > 0 {
		_ = e.Store.MarkMessagesCompacted(ctx, ids)
	}
	var entries []memory.Entry
	if e.Memory != nil {
		entries, _ = e.Memory.Active(ctx, run.UserID)
	}
	snap := prompt.BuildSnapshot(entries, HistoryTurns(selected), summary)
	session.PromptEpoch++
	session.PromptSnapshot = prompt.EncodeSnapshot(snap)
	session.CompactSummary = summary
	if err := e.Store.UpdateSession(ctx, *session); err != nil {
		return err
	}
	run.PromptEpoch = session.PromptEpoch
	run.Phase = store.PhaseModelRequest
	return e.Store.UpdateRun(ctx, *run)
}

func (e *Engine) ensureStarted(ctx context.Context, run store.Run) error {
	seq, err := e.Store.MaxEventSeq(ctx, run.ID)
	if err != nil {
		return err
	}
	if seq == 0 {
		_, err = AppendEvent(ctx, e.Store, e.Notify, run, store.EventRunStarted, store.EventPayload{})
	}
	return err
}

func (e *Engine) resourceLimit(ctx context.Context, run store.Run) error {
	journals, _ := e.Store.ListSuccessfulJournal(ctx, run.UserID, run.RequestID)
	summary := make([]string, 0, len(journals))
	for _, row := range journals {
		summary = append(summary, row.Tool)
	}
	return e.finish(ctx, run, store.StatusError, store.EventError, store.EventPayload{
		ErrorCode: "AGENT_RESOURCE_LIMIT", Text: "资源预算已耗尽", Journal: strings.Join(summary, ","),
	})
}

func (e *Engine) fail(ctx context.Context, run store.Run, code, text string) error {
	return e.finish(ctx, run, store.StatusError, store.EventError, store.EventPayload{ErrorCode: code, Text: text})
}

func (e *Engine) finish(ctx context.Context, run store.Run, status, eventType string, payload store.EventPayload) error {
	now := store.NowMs()
	run.Status = status
	run.Phase = store.PhaseDone
	run.EndedAtMs = now
	run.LastActivityAtMs = now
	if payload.ErrorCode != "" {
		run.ErrorCode = payload.ErrorCode
	}
	_ = e.Store.UpdateRun(ctx, run)
	_, _ = AppendEvent(ctx, e.Store, e.Notify, run, eventType, payload)
	thread, err := e.Store.GetThread(ctx, run.UserID)
	if err == nil && thread != nil && thread.ActiveRunID == run.ID {
		thread.ActiveRunID = 0
		thread.UpdatedAtMs = now
		_ = e.Store.SaveThread(ctx, *thread)
	}
	if run.Source == store.SourceUser && status == store.StatusDone {
		e.maybeScheduleReview(ctx, run)
	}
	return nil
}

func (e *Engine) maybeScheduleReview(ctx context.Context, run store.Run) {
	session, err := e.Store.GetSession(ctx, run.SessionID)
	if err != nil || session == nil {
		return
	}
	session.SuccessfulUserTurns++
	_ = e.Store.UpdateSession(ctx, *session)
	if session.SuccessfulUserTurns == 0 || session.SuccessfulUserTurns%10 != 0 {
		return
	}
	_, _ = e.Store.InsertRun(ctx, store.Run{
		UserID: run.UserID, SessionID: run.SessionID, RequestID: "review-" + itoa(store.NowMs()),
		Source: store.SourceMemoryReview, Status: store.StatusQueued, Phase: store.PhaseQueued,
		Priority: store.PriorityMemoryReview, PromptEpoch: session.PromptEpoch, CreatedAtMs: store.NowMs(),
	})
}

func ObserveQueueAge(seconds float64) { agentQueueAge.ObserveFloat(seconds) }
