package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"esx/app/assistant/internal/canonical"
	"esx/app/assistant/internal/llm"
	"esx/app/assistant/internal/memory"
	"esx/app/assistant/internal/prompt"
	"esx/app/assistant/internal/store"
	"esx/app/assistant/internal/tool"
	"esx/app/assistant/watch"
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

	errRunCancelled     = errors.New("assistant run cancelled")
	cancelWatchInterval = 50 * time.Millisecond
)

type Engine struct {
	Store    store.Store
	Memory   memory.Store
	Watch    watch.Store
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
	persistCtx := ctx
	workCtx, cancelWork := context.WithCancel(persistCtx)
	defer cancelWork()
	stopWatch := watchCancel(persistCtx, e.Store, run.ID, cancelWork)
	defer stopWatch()

	logger := logx.WithContext(persistCtx)
	if err := e.run(workCtx, persistCtx, run); err != nil {
		if errors.Is(err, errRunCancelled) {
			if fresh, getErr := e.Store.GetRun(persistCtx, run.ID); getErr == nil && fresh != nil &&
				!store.IsTerminalStatus(fresh.Status) {
				_ = e.cancel(persistCtx, *fresh)
			}
			return
		}
		logger.Errorw("assistant-agent run failed", logx.Field("runId", run.ID), logx.Field("err", err.Error()))
		if persistCtx.Err() != nil {
			return
		}
		if fresh, getErr := e.Store.GetRun(persistCtx, run.ID); getErr == nil && fresh != nil &&
			(fresh.Status == store.StatusRunning || fresh.Status == store.StatusQueued) {
			_ = e.fail(persistCtx, *fresh, "RUN_FAILED", err.Error())
		}
	}
}

func watchCancel(persistCtx context.Context, st store.Store, runID int64, cancelWork context.CancelFunc) func() {
	if st == nil {
		return func() {}
	}
	watchCtx, stop := context.WithCancel(persistCtx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(cancelWatchInterval)
		defer ticker.Stop()
		check := func() bool {
			run, err := st.GetRun(watchCtx, runID)
			return err == nil && run != nil && run.CancelRequested
		}
		if check() {
			cancelWork()
			return
		}
		for {
			select {
			case <-watchCtx.Done():
				return
			case <-ticker.C:
				if check() {
					cancelWork()
					return
				}
			}
		}
	}()
	return func() {
		stop()
		<-done
	}
}

func (e *Engine) cancelled(persistCtx context.Context, run *store.Run) bool {
	if run == nil || e.Store == nil {
		return false
	}
	fresh, err := e.Store.GetRun(persistCtx, run.ID)
	if err != nil || fresh == nil {
		return run.CancelRequested
	}
	if fresh.CancelRequested {
		run.CancelRequested = true
		return true
	}
	return run.CancelRequested
}

func (e *Engine) abortIfRequested(persistCtx context.Context, run store.Run) (bool, error) {
	if !e.cancelled(persistCtx, &run) {
		return false, nil
	}
	return true, e.cancel(persistCtx, run)
}

func (e *Engine) run(workCtx, persistCtx context.Context, run store.Run) error {
	session, err := e.Store.GetSession(persistCtx, run.SessionID)
	if err != nil {
		return e.fail(persistCtx, run, "SESSION_MISSING", err.Error())
	}
	snap, ok := prompt.DecodeSnapshot(session.PromptSnapshot)
	if !ok {
		var entries []memory.Entry
		if e.Memory != nil {
			entries, _ = e.Memory.Active(persistCtx, run.UserID)
		}
		snap = prompt.BuildSnapshot(entries, nil, session.CompactSummary)
		session.PromptSnapshot = prompt.EncodeSnapshot(snap)
		_ = e.Store.UpdateSession(persistCtx, *session)
	}
	if err := e.ensureStarted(persistCtx, run); err != nil {
		return err
	}
	started := time.Now()
	var reviewLive []prompt.Turn
	registry := tool.ForSource(e.Tools, run.Source, tool.CurrentConsentVersion)
	if registry == nil {
		return e.fail(persistCtx, run, "TOOLS_UNAVAILABLE", "no tools")
	}
	session.ToolSnapshot = prompt.EncodeTools(registry.Definitions())
	_ = e.Store.UpdateSession(persistCtx, *session)

	for {
		fresh, err := e.Store.GetRun(persistCtx, run.ID)
		if err != nil {
			return err
		}
		run = *fresh
		if aborted, abortErr := e.abortIfRequested(persistCtx, run); aborted {
			return abortErr
		}
		now := store.NowMs()
		if HardLimitExceeded(run, now) {
			return e.resourceLimit(persistCtx, run)
		}
		convergence, _ := RecordAlarms(persistCtx, e.Store, run, now)

		msgs, _ := e.Store.ListSessionMessages(persistCtx, run.UserID, run.SessionID, true)
		if e.Window == 0 && e.LLM != nil {
			e.Window = e.LLM.ContextWindowTokens()
		}
		if ShouldCompact(msgs, e.Window) {
			if err := e.compact(workCtx, persistCtx, &run, session, msgs); err != nil {
				if errors.Is(err, errRunCancelled) {
					return e.cancel(persistCtx, run)
				}
				return err
			}
			session, _ = e.Store.GetSession(persistCtx, run.SessionID)
			snap, _ = prompt.DecodeSnapshot(session.PromptSnapshot)
			continue
		}

		history := HistoryTurns(visibleForPrompt(msgs))
		if run.Source == store.SourceMemoryReview {
			history = append(history, reviewLive...)
		} else if open := unmatchedToolCalls(history); len(open) > 0 {
			for _, call := range open {
				if aborted, abortErr := e.abortIfRequested(persistCtx, run); aborted {
					return abortErr
				}
				if err := e.execTool(workCtx, persistCtx, &run, registry, call, &reviewLive); err != nil {
					if errors.Is(err, errRunCancelled) {
						return e.cancel(persistCtx, run)
					}
					return err
				}
			}
			msgs, _ = e.Store.ListSessionMessages(persistCtx, run.UserID, run.SessionID, true)
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
		pending, err := e.pendingUserTurns(persistCtx, run, seen)
		if err != nil {
			return err
		}
		turns = append(turns, pending...)

		run.Phase = store.PhaseModelRequest
		run.LastActivityAtMs = now
		_ = e.Store.UpdateRun(persistCtx, run)

		if e.LLM == nil {
			return e.fail(persistCtx, run, "LLM_DISABLED", "model is not configured")
		}
		result, err := e.LLM.Complete(workCtx, llm.Request{
			Messages:    turns,
			Tools:       registry.Definitions(),
			MaxTokens:   SingleOutputLimit(e.LLM.MaxOutputTokens()),
			Convergence: convergence,
		})
		if err != nil {
			if aborted, abortErr := e.abortIfRequested(persistCtx, run); aborted {
				return abortErr
			}
			if persistCtx.Err() != nil {
				return err
			}
			agentLLMCalls.Inc("failure")
			logx.WithContext(persistCtx).Errorw("assistant LLM complete failed",
				logx.Field("runId", run.ID), logx.Field("err", err.Error()))
			return e.fail(persistCtx, run, "LLM_UNAVAILABLE", "model call failed")
		}
		if aborted, abortErr := e.abortIfRequested(persistCtx, run); aborted {
			return abortErr
		}
		run.Rounds++
		run.InputTokens += result.Usage.PromptTokens
		run.OutputTokens += result.Usage.CompletionTokens
		run.CacheTokens += result.Usage.CacheTokens
		run.CostUSD += result.Usage.CostUSD
		run.LastActivityAtMs = store.NowMs()
		_ = e.Store.UpdateRun(persistCtx, run)
		if aborted, abortErr := e.abortIfRequested(persistCtx, run); aborted {
			return abortErr
		}
		if result.IncompleteReason != "" {
			agentLLMCalls.Inc("incomplete")
			return e.incomplete(persistCtx, run, result)
		}
		agentLLMCalls.Inc("success")

		if len(result.ToolCalls) == 0 {
			text := strings.TrimSpace(result.Text)
			result.Text = text
			agentFirstToken.ObserveFloat(time.Since(started).Seconds())
			if run.Source == store.SourceUser && text != "" {
				if _, err := AppendEvent(persistCtx, e.Store, e.Notify, run, store.EventToken, store.EventPayload{Text: text}); err != nil {
					return err
				}
			}
			return e.completeModelText(persistCtx, run, result)
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
		if err := e.recordTurn(persistCtx, run, assistantTurn, store.KindTool, false, &reviewLive); err != nil {
			return err
		}
		run.Phase = store.PhaseToolExecuting
		_ = e.Store.UpdateRun(persistCtx, run)
		for _, call := range calls {
			if aborted, abortErr := e.abortIfRequested(persistCtx, run); aborted {
				return abortErr
			}
			if err := e.execTool(workCtx, persistCtx, &run, registry, call, &reviewLive); err != nil {
				if errors.Is(err, errRunCancelled) {
					return e.cancel(persistCtx, run)
				}
				return err
			}
		}
	}
}

func (e *Engine) incomplete(ctx context.Context, run store.Run, result llm.Result) error {
	partial := strings.TrimSpace(result.Text)
	if partial != "" && run.Source == store.SourceUser {
		if _, err := AppendEvent(ctx, e.Store, e.Notify, run, store.EventToken, store.EventPayload{Text: partial}); err != nil {
			return err
		}
		if run.Source == store.SourceUser {
			if err := e.persistVisibleAssistant(ctx, run, partial, result.Raw, store.KindMessage); err != nil {
				return err
			}
		}
	}
	reason := "UNKNOWN"
	switch strings.ToLower(strings.TrimSpace(result.IncompleteReason)) {
	case "max_output_tokens":
		reason = "MAX_OUTPUT_TOKENS"
	case "content_filter":
		reason = "CONTENT_FILTER"
	}
	return e.finish(ctx, run, store.StatusError, store.EventError, store.EventPayload{
		ErrorCode: "LLM_INCOMPLETE_" + reason,
		Text:      "模型响应未完整完成",
		Partial:   partial,
	})
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

func (e *Engine) pendingUserTurns(ctx context.Context, run store.Run, seen map[int64]struct{}) ([]prompt.Turn, error) {
	out := make([]prompt.Turn, 0)
	if run.Source == store.SourceWatch {
		turn, err := e.watchInputTurn(ctx, run)
		if err != nil {
			return nil, err
		}
		out = append(out, turn)
	} else if len(run.QueuedPayload) > 0 {
		payload := decodeInputPayload(run.QueuedPayload)
		if payload.Text != "" {
			content := providerUserContent(payload.Text, payload.Attachments, payload.ContextPostID)
			if payload.MessageID == 0 {
				out = append(out, prompt.Turn{Role: store.RoleUser, Content: content})
			} else if _, ok := seen[payload.MessageID]; !ok {
				out = append(out, prompt.Turn{Role: store.RoleUser, Content: content})
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
			if turn, ok := turnFromMessage(*msg); ok {
				out = append(out, turn)
			} else {
				out = append(out, prompt.Turn{Role: store.RoleUser, Content: msg.Content})
			}
		}
	}
	return out, nil
}

func (e *Engine) execTool(workCtx, persistCtx context.Context, run *store.Run, registry *tool.Registry, call llm.ToolCall, reviewLive *[]prompt.Turn) error {
	call.Arguments = canonical.UnwrapArgsJSON(call.Arguments)
	digest, _ := canonical.DigestArgs(call.Arguments)
	_, _ = e.Store.InsertToolCall(persistCtx, store.ToolCall{
		RunID: run.ID, CallID: call.ID, Tool: call.Name, ArgsJSON: call.Arguments,
		CanonicalArgsDigest: digest, Status: "running", CreatedAtMs: store.NowMs(),
	})
	_, _ = AppendEvent(persistCtx, e.Store, e.Notify, *run, store.EventToolCall, store.EventPayload{
		ToolCall: &store.ToolInfo{CallID: call.ID, Tool: call.Name, Summary: call.Name, PayloadJSON: call.Arguments},
	})
	if journal, err := e.Store.GetJournal(persistCtx, run.UserID, run.RequestID, call.Name, digest); err == nil && journal != nil && journal.Status == store.JournalSuccess {
		agentToolCalls.Inc(call.Name, "replay")
		text := decodeToolResultText(journal.ResultJSON)
		_ = e.Store.UpdateToolCall(persistCtx, store.ToolCall{RunID: run.ID, CallID: call.ID, Status: "success", ResultJSON: journal.ResultJSON})
		if err := e.recordTurn(persistCtx, *run, prompt.Turn{Role: store.RoleTool, Content: text, ToolCallID: call.ID, Name: call.Name}, store.KindTool, false, reviewLive); err != nil {
			return err
		}
		_, _ = AppendEvent(persistCtx, e.Store, e.Notify, *run, store.EventToolResult, store.EventPayload{
			ToolCall: &store.ToolInfo{CallID: call.ID, Tool: call.Name, Summary: "replay", PayloadJSON: text},
			Text:     text,
		})
		return nil
	}
	if registry.HighRisk(call.Name) {
		if err := e.requireConfirm(workCtx, persistCtx, run, call, digest); err != nil {
			return err
		}
	}
	if e.cancelled(persistCtx, run) {
		return errRunCancelled
	}
	sess := e.toolSession(*run)
	text, cards, err := registry.Call(workCtx, sess, call.Name, call.ID, call.Arguments)
	outcome := "success"
	if err != nil {
		if errors.Is(err, context.Canceled) && e.cancelled(persistCtx, run) {
			return errRunCancelled
		}
		outcome = "unavailable"
		text = err.Error()
	}
	agentToolCalls.Inc(call.Name, outcome)
	run.ToolCalls++
	run.LastActivityAtMs = store.NowMs()
	_ = e.Store.UpdateRun(persistCtx, *run)
	resultJSON := encodeToolResultJSONWithChanges(text, err, sess.ChangeIDs)
	if sideEffect(call.Name) && err == nil {
		row, reserved, jerr := e.Store.ReserveJournal(persistCtx, store.Journal{
			UserID: run.UserID, RequestID: run.RequestID, Tool: call.Name, CanonicalArgsDigest: digest,
			Status: store.JournalSuccess, ResultJSON: resultJSON, CreatedAtMs: store.NowMs(),
		})
		if jerr == nil && reserved && row != nil {
			_ = e.Store.CompleteJournal(persistCtx, row.ID, store.JournalSuccess, resultJSON)
		}
	}
	_ = e.Store.UpdateToolCall(persistCtx, store.ToolCall{RunID: run.ID, CallID: call.ID, Status: outcome, ResultJSON: resultJSON})
	if recErr := e.recordTurn(persistCtx, *run, prompt.Turn{Role: store.RoleTool, Content: text, ToolCallID: call.ID, Name: call.Name}, store.KindTool, false, reviewLive); recErr != nil {
		return recErr
	}
	_, _ = AppendEvent(persistCtx, e.Store, e.Notify, *run, store.EventToolResult, store.EventPayload{
		ToolCall: &store.ToolInfo{CallID: call.ID, Tool: call.Name, Summary: call.Name, PayloadJSON: text},
		Text:     text,
	})
	for _, card := range cards {
		_, _ = AppendEvent(persistCtx, e.Store, e.Notify, *run, store.EventSourceCard, store.EventPayload{SourceCard: &card})
	}
	if e.cancelled(persistCtx, run) {
		return errRunCancelled
	}
	return nil
}

func (e *Engine) toolSession(run store.Run) *tool.Session {
	sess := &tool.Session{UserID: run.UserID, SessionID: run.SessionID, RunID: run.ID, RequestID: run.RequestID, Source: run.Source}
	payload := decodeInputPayload(run.QueuedPayload)
	sess.ContextPostID = payload.ContextPostID
	sess.Attachments = make([]tool.Attachment, 0, len(payload.Attachments))
	for _, item := range payload.Attachments {
		sess.Attachments = append(sess.Attachments, tool.Attachment{MediaID: item.MediaID, URL: item.URL})
	}
	return sess
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

func encodeToolResultJSONWithChanges(text string, callErr error, changeIDs []int64) string {
	payload := map[string]any{"ok": callErr == nil, "text": text}
	if len(changeIDs) > 0 {
		payload["change_ids"] = changeIDs
	}
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

func (e *Engine) requireConfirm(workCtx, persistCtx context.Context, run *store.Run, call llm.ToolCall, digest string) error {
	_, _ = e.Store.InsertConfirmation(persistCtx, store.Confirmation{
		UserID: run.UserID, SessionID: run.SessionID, RunID: run.ID, CallID: call.ID, Tool: call.Name,
		CanonicalArgsDigest: digest, Status: store.ConfirmPending, CreatedAtMs: store.NowMs(),
	})
	_, _ = AppendEvent(persistCtx, e.Store, e.Notify, *run, store.EventConfirmRequired, store.EventPayload{
		ToolCall: &store.ToolInfo{CallID: call.ID, Tool: call.Name, Summary: "确认删除帖子", PayloadJSON: call.Arguments},
	})
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		if e.cancelled(persistCtx, run) {
			return errRunCancelled
		}
		conf, _ := e.Store.GetConfirmation(persistCtx, run.ID, call.ID)
		if conf != nil && conf.Status == store.ConfirmApproved {
			return nil
		}
		if conf != nil && conf.Status == store.ConfirmRejected {
			return errx.New(errx.PermissionDenied, "delete_post rejected")
		}
		select {
		case <-workCtx.Done():
			if e.cancelled(persistCtx, run) {
				return errRunCancelled
			}
			return workCtx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	return errx.New(errx.ParamError, "delete_post confirmation expired")
}

func (e *Engine) compact(workCtx, persistCtx context.Context, run *store.Run, session *store.Session, msgs []store.Message) error {
	run.Phase = store.PhaseCompact
	_ = e.Store.UpdateRun(persistCtx, *run)
	msgs = liveMessages(msgs)
	keep := EstimateMessageTokens(msgs) / 5
	if keep < 1 {
		keep = 1
	}
	selected := SelectKeep(msgs, keep, unfinishedCallIDs(msgs))
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
		result, err := e.LLM.Complete(workCtx, llm.Request{
			Messages:     []prompt.Turn{{Role: store.RoleSystem, Content: "用中文压缩以下会话，不要引入新事实。"}, {Role: store.RoleUser, Content: b.String()}},
			DisableTools: true,
			MaxTokens:    512,
		})
		if err != nil {
			if errors.Is(err, context.Canceled) && e.cancelled(persistCtx, run) {
				return errRunCancelled
			}
		} else if strings.TrimSpace(result.Text) != "" {
			summary = result.Text
		}
	}
	if e.cancelled(persistCtx, run) {
		return errRunCancelled
	}
	keepIDs := make(map[int64]struct{}, len(selected))
	for _, msg := range selected {
		keepIDs[msg.ID] = struct{}{}
	}
	ids := make([]int64, 0, len(msgs))
	for _, msg := range msgs {
		if msg.Compacted {
			continue
		}
		if _, keep := keepIDs[msg.ID]; keep {
			continue
		}
		ids = append(ids, msg.ID)
	}
	if len(ids) > 0 {
		_ = e.Store.MarkMessagesCompacted(persistCtx, ids)
	}
	var entries []memory.Entry
	if e.Memory != nil {
		entries, _ = e.Memory.Active(persistCtx, run.UserID)
	}
	snap := prompt.BuildSnapshot(entries, HistoryTurns(selected), summary)
	session.PromptEpoch++
	session.PromptSnapshot = prompt.EncodeSnapshot(snap)
	session.CompactSummary = summary
	if err := e.Store.UpdateSession(persistCtx, *session); err != nil {
		return err
	}
	run.PromptEpoch = session.PromptEpoch
	run.Phase = store.PhaseModelRequest
	return e.Store.UpdateRun(persistCtx, *run)
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

func (e *Engine) cancel(ctx context.Context, run store.Run) error {
	run.CancelRequested = true
	return e.finish(ctx, run, store.StatusCancelled, store.EventError, store.EventPayload{ErrorCode: "CANCELLED", Text: "run cancelled"})
}

func (e *Engine) finish(ctx context.Context, run store.Run, status, eventType string, payload store.EventPayload) error {
	now := store.NowMs()
	err := e.Store.Transact(ctx, func(ctx context.Context, tx store.Store) error {
		if _, err := finishRunTx(ctx, tx, run, status, eventType, payload, now); err != nil {
			return err
		}
		thread, err := tx.LockThread(ctx, run.UserID)
		if err != nil {
			return err
		}
		if thread.ActiveRunID == run.ID {
			thread.ActiveRunID = 0
			thread.UpdatedAtMs = now
			if err := tx.SaveThread(ctx, *thread); err != nil {
				return err
			}
		}
		if run.Source == store.SourceWatch && status != store.StatusDone {
			watchPayload := decodeWatchRunPayload(run.QueuedPayload)
			if watchPayload.BucketID > 0 {
				return tx.ResetBucket(ctx, watchPayload.BucketID, run.ID)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	e.wake(ctx, run.ID)
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
