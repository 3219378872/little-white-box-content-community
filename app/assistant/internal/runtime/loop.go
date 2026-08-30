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
	errRunRedirected    = errors.New("assistant run redirected")
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
	stopWatch := watchCancel(persistCtx, e.Store, run, cancelWork)
	defer stopWatch()

	logger := logx.WithContext(persistCtx)
	if err := e.run(workCtx, persistCtx, run); err != nil {
		if errors.Is(err, store.ErrLeaseLost) || persistCtx.Err() != nil {
			return
		}
		if errors.Is(err, errRunCancelled) {
			if fresh, getErr := e.Store.GetRun(persistCtx, run.ID); getErr == nil && fresh != nil &&
				!store.IsTerminalStatus(fresh.Status) {
				_ = e.cancel(persistCtx, *fresh)
			}
			return
		}
		logger.Errorw("assistant-agent run failed", logx.Field("runId", run.ID), logx.Field("err", err.Error()))
		if fresh, getErr := e.Store.GetRun(persistCtx, run.ID); getErr == nil && fresh != nil &&
			(fresh.Status == store.StatusRunning || fresh.Status == store.StatusQueued) {
			_ = e.fail(persistCtx, *fresh, "RUN_FAILED", err.Error())
		}
	}
}

func (e *Engine) step(ctx context.Context, run store.Run, fn func(context.Context, store.Store) error) error {
	return e.Store.RunStep(ctx, run.Fence(), fn)
}

func (e *Engine) updateRun(ctx context.Context, run store.Run) error {
	return e.step(ctx, run, func(ctx context.Context, tx store.Store) error {
		return tx.UpdateRun(ctx, run)
	})
}

func (e *Engine) appendEvent(ctx context.Context, run store.Run, eventType string, payload store.EventPayload) (store.Event, error) {
	var event store.Event
	err := e.step(ctx, run, func(ctx context.Context, tx store.Store) error {
		var err error
		event, err = AppendEvent(ctx, tx, nil, run, eventType, payload)
		return err
	})
	if err == nil && e.Notify != nil {
		_ = e.Notify.Wake(ctx, run.ID)
	}
	return event, err
}

func watchCancel(persistCtx context.Context, st store.Store, claimed store.Run, cancelWork context.CancelFunc) func() {
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
			run, err := st.GetRun(watchCtx, claimed.ID)
			if err == nil && run != nil && run.CancelRequested {
				return true
			}
			version, granted, consentErr := st.AgentConsent(watchCtx, claimed.UserID)
			if consentErr != nil {
				return true
			}
			if !granted || version != claimed.ConsentVersion {
				_ = st.RequestCancel(watchCtx, claimed.UserID, claimed.ID)
				return true
			}
			return false
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
	if err != nil {
		run.CancelRequested = true
		return true
	}
	if fresh == nil {
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

func (e *Engine) requireFrozenConsent(ctx context.Context, run *store.Run) error {
	if run == nil {
		return nil
	}
	version, granted, err := e.Store.AgentConsent(ctx, run.UserID)
	if err != nil {
		return err
	}
	if !granted || version <= 0 || version != run.ConsentVersion {
		run.CancelRequested = true
		return errRunCancelled
	}
	return nil
}

func (e *Engine) run(workCtx, persistCtx context.Context, run store.Run) error {
	if err := e.requireFrozenConsent(persistCtx, &run); err != nil {
		return err
	}
	session, err := e.Store.GetSession(persistCtx, run.SessionID)
	if err != nil {
		return e.fail(persistCtx, run, "SESSION_MISSING", err.Error())
	}
	snap, ok := prompt.DecodeSnapshot(session.PromptSnapshot)
	if !ok {
		var entries []memory.Entry
		if e.Memory != nil {
			entries, err = e.Memory.Active(persistCtx, run.UserID)
			if err != nil {
				return err
			}
		}
		snap = prompt.BuildSnapshot(entries, nil, session.CompactSummary)
		session.PromptSnapshot = prompt.EncodeSnapshot(snap)
		if err := e.step(persistCtx, run, func(ctx context.Context, tx store.Store) error {
			return tx.UpdateSession(ctx, *session)
		}); err != nil {
			return err
		}
	}
	if err := e.ensureStarted(persistCtx, run); err != nil {
		return err
	}
	started := time.Now()
	var reviewLive []prompt.Turn
	registry := tool.ForSource(e.Tools, run.Source, run.ConsentVersion)
	if registry == nil {
		return e.fail(persistCtx, run, "TOOLS_UNAVAILABLE", "no tools")
	}
	session.ToolSnapshot = prompt.EncodeTools(registry.Definitions())
	if err := e.step(persistCtx, run, func(ctx context.Context, tx store.Store) error {
		return tx.UpdateSession(ctx, *session)
	}); err != nil {
		return err
	}

	for {
		fresh, err := e.Store.GetRun(persistCtx, run.ID)
		if err != nil {
			return err
		}
		run = *fresh
		if err := e.requireFrozenConsent(persistCtx, &run); err != nil {
			return err
		}
		if aborted, abortErr := e.abortIfRequested(persistCtx, run); aborted {
			return abortErr
		}
		now := store.NowMs()
		if HardLimitExceeded(run, now) {
			return e.resourceLimit(persistCtx, run)
		}
		var convergence string
		if err := e.step(persistCtx, run, func(ctx context.Context, tx store.Store) error {
			var err error
			convergence, err = RecordAlarms(ctx, tx, run, now)
			return err
		}); err != nil {
			return err
		}

		msgs, err := e.Store.ListSessionMessages(persistCtx, run.UserID, run.SessionID, true)
		if err != nil {
			return err
		}
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
			session, err = e.Store.GetSession(persistCtx, run.SessionID)
			if err != nil {
				return err
			}
			var decoded bool
			snap, decoded = prompt.DecodeSnapshot(session.PromptSnapshot)
			if !decoded {
				return errors.New("compacted prompt snapshot is invalid")
			}
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
			msgs, err = e.Store.ListSessionMessages(persistCtx, run.UserID, run.SessionID, true)
			if err != nil {
				return err
			}
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
		pending, queuedThrough, err := e.pendingUserTurns(persistCtx, run, seen)
		if err != nil {
			return err
		}
		turns = append(turns, pending...)
		if queuedThrough > 0 {
			if err := e.step(persistCtx, run, func(ctx context.Context, tx store.Store) error {
				return tx.DeleteQueueThrough(ctx, run.ID, queuedThrough)
			}); err != nil {
				return err
			}
		}

		run.Phase = store.PhaseModelRequest
		run.LastActivityAtMs = now
		if err := e.updateRun(persistCtx, run); err != nil {
			return err
		}

		if e.LLM == nil {
			return e.fail(persistCtx, run, "LLM_DISABLED", "model is not configured")
		}
		result, err := e.completeModel(workCtx, persistCtx, run, llm.Request{
			Messages:    turns,
			Tools:       registry.Definitions(),
			MaxTokens:   SingleOutputLimit(e.LLM.MaxOutputTokens()),
			Convergence: convergence,
		})
		if err != nil {
			if errors.Is(err, errRunRedirected) {
				continue
			}
			if errors.Is(err, errRunCancelled) {
				return e.cancel(persistCtx, run)
			}
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
			sess := &tool.Session{UserID: run.UserID, SessionID: run.SessionID, RunID: run.ID,
				RequestID: run.RequestID, Source: run.Source, ConsentVersion: run.ConsentVersion}
			preparedOK := false
			var prepareErr error
			if prepared, prepErr := registry.Prepare(workCtx, sess, call.Name, args); prepErr == nil {
				args = prepared
				preparedOK = true
			} else {
				prepareErr = prepErr
			}
			calls = append(calls, llm.ToolCall{ID: id, Name: call.Name, Arguments: args, Prepared: preparedOK, PrepareError: prepareErr})
			assistantTurn.ToolCalls = append(assistantTurn.ToolCalls, prompt.ToolCall{ID: id, Name: call.Name, Arguments: args, Prepared: preparedOK})
		}
		run.Phase = store.PhaseToolExecuting
		if err := e.recordModelToolStep(persistCtx, run, assistantTurn, &reviewLive); err != nil {
			return err
		}
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
	reason := "UNKNOWN"
	switch strings.ToLower(strings.TrimSpace(result.IncompleteReason)) {
	case "max_output_tokens":
		reason = "MAX_OUTPUT_TOKENS"
	case "content_filter":
		reason = "CONTENT_FILTER"
	}
	payload := store.EventPayload{
		ErrorCode: "LLM_INCOMPLETE_" + reason,
		Text:      "模型响应未完整完成",
		Partial:   partial,
	}
	if partial != "" && run.Source == store.SourceUser {
		return e.finishWithMessage(ctx, run, store.StatusError, store.EventError, payload, partial, result.Raw)
	}
	return e.finish(ctx, run, store.StatusError, store.EventError, payload)
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

func (e *Engine) completeModel(workCtx, persistCtx context.Context, run store.Run, req llm.Request) (llm.Result, error) {
	if err := e.step(persistCtx, run, func(context.Context, store.Store) error { return nil }); err != nil {
		return llm.Result{}, err
	}
	before, err := e.Store.GetRun(persistCtx, run.ID)
	if err != nil {
		return llm.Result{}, err
	}
	if before.CancelRequested {
		return llm.Result{}, errRunCancelled
	}
	if before.LeaseOwner != run.LeaseOwner || before.LeaseGeneration != run.LeaseGeneration {
		return llm.Result{}, store.ErrLeaseLost
	}
	if before.InputVersion != run.InputVersion {
		return llm.Result{}, errRunRedirected
	}
	callCtx, cancelCall := context.WithCancel(workCtx)
	stop := watchInputChange(persistCtx, e.Store, run.ID, run.InputVersion, cancelCall)
	result, err := e.LLM.Complete(callCtx, req)
	stop()
	fresh, getErr := e.Store.GetRun(persistCtx, run.ID)
	if getErr != nil {
		return llm.Result{}, getErr
	}
	if fresh.CancelRequested {
		return llm.Result{}, errRunCancelled
	}
	if fresh.LeaseOwner != run.LeaseOwner || fresh.LeaseGeneration != run.LeaseGeneration {
		return llm.Result{}, store.ErrLeaseLost
	}
	if fresh.InputVersion != run.InputVersion {
		return llm.Result{}, errRunRedirected
	}
	return result, err
}

func watchInputChange(ctx context.Context, st store.Store, runID, inputVersion int64, cancel context.CancelFunc) func() {
	watchCtx, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(cancelWatchInterval)
		defer ticker.Stop()
		for {
			select {
			case <-watchCtx.Done():
				return
			case <-ticker.C:
				run, err := st.GetRun(watchCtx, runID)
				if err == nil && run != nil && (run.CancelRequested || run.InputVersion != inputVersion) {
					cancel()
					return
				}
			}
		}
	}()
	return func() {
		stop()
		<-done
		cancel()
	}
}

func (e *Engine) recordModelToolStep(ctx context.Context, run store.Run, turn prompt.Turn, reviewLive *[]prompt.Turn) error {
	if run.Source == store.SourceMemoryReview {
		if reviewLive != nil {
			*reviewLive = append(*reviewLive, turn)
		}
		return e.updateRun(ctx, run)
	}
	return e.step(ctx, run, func(ctx context.Context, tx store.Store) error {
		if err := tx.UpdateRun(ctx, run); err != nil {
			return err
		}
		_, err := tx.InsertMessage(ctx, store.Message{
			UserID: run.UserID, SessionID: run.SessionID, RunID: run.ID,
			Role: turn.Role, Kind: store.KindTool, Content: turn.Content, APIContent: prompt.EncodeTurn(turn),
			Visible: false, CreatedAtMs: store.NowMs(),
		})
		return err
	})
}

func (e *Engine) pendingUserTurns(ctx context.Context, run store.Run, seen map[int64]struct{}) ([]prompt.Turn, int64, error) {
	out := make([]prompt.Turn, 0)
	if run.Source == store.SourceWatch {
		turn, err := e.watchInputTurn(ctx, run)
		if err != nil {
			return nil, 0, err
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
	items, err := e.Store.ListQueue(ctx, run.ID)
	if err != nil {
		return nil, 0, err
	}
	var queuedThrough int64
	for _, item := range items {
		if item.ID > queuedThrough {
			queuedThrough = item.ID
		}
		if _, ok := seen[item.MessageID]; ok {
			continue
		}
		msg, err := e.Store.GetMessage(ctx, run.UserID, item.MessageID)
		if err != nil {
			return nil, 0, err
		}
		if msg != nil {
			if turn, ok := turnFromMessage(*msg); ok {
				out = append(out, turn)
			} else {
				out = append(out, prompt.Turn{Role: store.RoleUser, Content: msg.Content})
			}
		}
	}
	return out, queuedThrough, nil
}

func (e *Engine) execTool(workCtx, persistCtx context.Context, run *store.Run, registry *tool.Registry, call llm.ToolCall, reviewLive *[]prompt.Turn) error {
	sess := e.toolSession(*run)
	prepErr := call.PrepareError
	if !call.Prepared && prepErr == nil {
		prepared, err := registry.Prepare(workCtx, sess, call.Name, call.Arguments)
		prepErr = err
		if prepErr == nil {
			call.Arguments = prepared
			call.Prepared = true
		} else {
			call.Arguments = canonical.UnwrapArgsJSON(call.Arguments)
		}
	} else {
		call.Arguments = canonical.UnwrapArgsJSON(call.Arguments)
	}
	digest, digestErr := canonical.DigestArgs(call.Arguments)
	if digestErr != nil {
		digest = "invalid:" + call.ID
	}

	journal, reserved, err := e.startToolStep(persistCtx, *run, call, digest, prepErr == nil && sideEffect(call.Name))
	if err != nil {
		return err
	}
	if journal != nil && journal.Status == store.JournalSuccess {
		agentToolCalls.Inc(call.Name, "replay")
		text := decodeToolResultText(journal.ResultJSON)
		sess.ChangeIDs = decodeToolResultChangeIDs(journal.ResultJSON)
		return e.finishToolStep(persistCtx, run, call, text, nil, nil, sess.ChangeIDs, journal, false, "replay", reviewLive)
	}
	if journal != nil && !reserved && journal.Status == store.JournalPending {
		return errors.New("side effect command is already in progress")
	}
	if prepErr != nil {
		text := prepErr.Error()
		return e.finishToolStep(persistCtx, run, call, text, prepErr, nil, nil, journal, true, "invalid", reviewLive)
	}
	sess.Recovery = journal != nil && journal.Takeover
	if registry.HighRisk(call.Name) {
		if err := e.requireConfirm(workCtx, persistCtx, run, call, digest); err != nil {
			return err
		}
		if journal == nil || !journal.Takeover {
			rechecked, err := registry.Prepare(workCtx, sess, call.Name, call.Arguments)
			if err != nil {
				return err
			}
			recheckedDigest, err := canonical.DigestArgs(rechecked)
			if err != nil || recheckedDigest != digest {
				return errx.New(errx.ContentVersionConflict, "delete_post changed after confirmation")
			}
			call.Arguments = rechecked
		}
	}
	if e.cancelled(persistCtx, run) {
		return errRunCancelled
	}
	if err := e.requireFrozenConsent(persistCtx, run); err != nil {
		return err
	}
	var (
		text    string
		cards   []store.SourceRef
		callErr error
	)
	invoke := func() {
		text, cards, callErr = registry.Call(workCtx, sess, call.Name, call.ID, call.Arguments)
	}
	if sideEffect(call.Name) {
		if err := e.step(persistCtx, *run, func(context.Context, store.Store) error {
			invoke()
			return nil
		}); err != nil {
			return err
		}
	} else {
		if err := e.step(persistCtx, *run, func(context.Context, store.Store) error { return nil }); err != nil {
			return err
		}
		invoke()
	}
	outcome := "success"
	if callErr != nil {
		if errors.Is(callErr, context.Canceled) && e.cancelled(persistCtx, run) {
			return errRunCancelled
		}
		outcome = "unavailable"
		text = callErr.Error()
	}
	agentToolCalls.Inc(call.Name, outcome)
	if err := e.finishToolStep(persistCtx, run, call, text, callErr, cards, sess.ChangeIDs, journal, true, outcome, reviewLive); err != nil {
		return err
	}
	if run.Source == store.SourceWatch && repeatedWatchRead(persistCtx, e.Store, run.ID, call.Name, digest) {
		// A provider that keeps re-issuing the same read cannot make progress
		// (large Snowflake IDs are a common cause). Deliver a truthful degraded
		// notification rather than burning the run budget indefinitely.
		return e.completeWatch(persistCtx, *run,
			"检测到一条新的关注动态，但当前无法完成内容核验，暂不展示具体详情。", nil)
	}
	if e.cancelled(persistCtx, run) {
		return errRunCancelled
	}
	return nil
}

func repeatedWatchRead(ctx context.Context, st store.Store, runID int64, toolName, digest string) bool {
	if st == nil || runID <= 0 || digest == "" {
		return false
	}
	calls, err := st.ListToolCalls(ctx, runID)
	if err != nil {
		return false
	}
	count := 0
	for _, call := range calls {
		if call.Tool == toolName && call.CanonicalArgsDigest == digest && call.Status != "running" {
			count++
		}
	}
	return count >= 2
}

func (e *Engine) toolSession(run store.Run) *tool.Session {
	sess := &tool.Session{
		UserID: run.UserID, SessionID: run.SessionID, RunID: run.ID, RequestID: run.RequestID,
		Source: run.Source, ConsentVersion: run.ConsentVersion, Fence: run.Fence(),
	}
	if run.Source == store.SourceWatch {
		watchPayload := decodeWatchRunPayload(run.QueuedPayload)
		sess.WatchPostIDs = append([]int64(nil), watchPayload.PostIDs...)
	} else {
		payload := decodeInputPayload(run.QueuedPayload)
		sess.ContextPostID = payload.ContextPostID
		sess.Attachments = make([]tool.Attachment, 0, len(payload.Attachments))
		for _, item := range payload.Attachments {
			sess.Attachments = append(sess.Attachments, tool.Attachment{MediaID: item.MediaID, URL: item.URL})
		}
	}
	return sess
}

func (e *Engine) startToolStep(
	ctx context.Context,
	run store.Run,
	call llm.ToolCall,
	digest string,
	reserveSideEffect bool,
) (*store.Journal, bool, error) {
	var (
		journal  *store.Journal
		reserved bool
		newCall  bool
	)
	now := store.NowMs()
	err := e.step(ctx, run, func(ctx context.Context, tx store.Store) error {
		existing, err := tx.GetToolCall(ctx, run.ID, call.ID)
		if err != nil {
			return err
		}
		if existing == nil {
			if _, err := tx.InsertToolCall(ctx, store.ToolCall{
				RunID: run.ID, CallID: call.ID, Tool: call.Name, ArgsJSON: call.Arguments,
				CanonicalArgsDigest: digest, Status: "running", CreatedAtMs: now,
			}); err != nil {
				return err
			}
			newCall = true
			if _, err := AppendEvent(ctx, tx, nil, run, store.EventToolCall, store.EventPayload{
				ToolCall: &store.ToolInfo{CallID: call.ID, Tool: call.Name, Summary: call.Name, PayloadJSON: call.Arguments},
			}); err != nil {
				return err
			}
		} else if existing.Tool != call.Name || existing.CanonicalArgsDigest != digest {
			return errx.New(errx.PermissionDenied, "tool call identity changed during recovery")
		}
		if !reserveSideEffect {
			return nil
		}
		journal, reserved, err = tx.ReserveJournal(ctx, store.Journal{
			UserID: run.UserID, RequestID: run.RequestID, Tool: call.Name, CanonicalArgsDigest: digest,
			RunID: run.ID, LeaseGeneration: run.LeaseGeneration, Status: store.JournalPending,
			CreatedAtMs: now, UpdatedAtMs: now,
		})
		return err
	})
	if err == nil && newCall && e.Notify != nil {
		_ = e.Notify.Wake(ctx, run.ID)
	}
	return journal, reserved, err
}

func (e *Engine) finishToolStep(
	ctx context.Context,
	run *store.Run,
	call llm.ToolCall,
	text string,
	callErr error,
	cards []store.SourceRef,
	changeIDs []int64,
	journal *store.Journal,
	countCall bool,
	outcome string,
	reviewLive *[]prompt.Turn,
) error {
	if countCall {
		run.ToolCalls++
	}
	run.LastActivityAtMs = store.NowMs()
	resultJSON := encodeToolResultJSONWithChanges(text, callErr, changeIDs)
	turn := prompt.Turn{Role: store.RoleTool, Content: text, ToolCallID: call.ID, Name: call.Name}
	err := e.step(ctx, *run, func(ctx context.Context, tx store.Store) error {
		if err := tx.UpdateRun(ctx, *run); err != nil {
			return err
		}
		if journal != nil {
			status := store.JournalSuccess
			if callErr != nil {
				status = store.JournalError
			}
			if err := tx.CompleteJournal(ctx, journal.ID, status, resultJSON); err != nil {
				return err
			}
		}
		if err := tx.UpdateToolCall(ctx, store.ToolCall{
			RunID: run.ID, CallID: call.ID, Status: outcome, ResultJSON: resultJSON,
		}); err != nil {
			return err
		}
		if run.Source != store.SourceMemoryReview {
			if _, err := tx.InsertMessage(ctx, store.Message{
				UserID: run.UserID, SessionID: run.SessionID, RunID: run.ID,
				Role: turn.Role, Kind: store.KindTool, Content: turn.Content, APIContent: prompt.EncodeTurn(turn),
				Visible: false, CreatedAtMs: store.NowMs(),
			}); err != nil {
				return err
			}
		}
		if _, err := AppendEvent(ctx, tx, nil, *run, store.EventToolResult, store.EventPayload{
			ToolCall: &store.ToolInfo{CallID: call.ID, Tool: call.Name, Summary: outcome, PayloadJSON: text}, Text: text,
		}); err != nil {
			return err
		}
		for i := range cards {
			if _, err := AppendEvent(ctx, tx, nil, *run, store.EventSourceCard, store.EventPayload{SourceCard: &cards[i]}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if run.Source == store.SourceMemoryReview && reviewLive != nil {
		*reviewLive = append(*reviewLive, turn)
	}
	if e.Notify != nil {
		_ = e.Notify.Wake(ctx, run.ID)
	}
	return nil
}

func decodeToolResultChangeIDs(raw string) []int64 {
	var payload struct {
		ChangeIDs []int64 `json:"change_ids"`
	}
	if json.Unmarshal([]byte(raw), &payload) != nil {
		return nil
	}
	return payload.ChangeIDs
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
			out = append(out, llm.ToolCall{ID: id, Name: call.Name, Arguments: call.Arguments, Prepared: call.Prepared})
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
	targetRevision, err := expectedRevision(call.Arguments)
	if err != nil || targetRevision <= 0 {
		return errx.New(errx.ParamError, "delete_post confirmation requires a concrete revision")
	}
	created := false
	err = e.step(persistCtx, *run, func(ctx context.Context, tx store.Store) error {
		existing, err := tx.GetConfirmation(ctx, run.ID, call.ID)
		if err != nil {
			return err
		}
		if existing != nil {
			if existing.UserID != run.UserID || existing.SessionID != run.SessionID ||
				existing.Tool != call.Name || existing.CanonicalArgsDigest != digest || existing.TargetRevision != targetRevision {
				return errx.NewWithCode(errx.PermissionDenied)
			}
			return nil
		}
		if _, err := tx.InsertConfirmation(ctx, store.Confirmation{
			UserID: run.UserID, SessionID: run.SessionID, RunID: run.ID, CallID: call.ID, Tool: call.Name,
			CanonicalArgsDigest: digest, TargetRevision: targetRevision,
			Status: store.ConfirmPending, CreatedAtMs: store.NowMs(),
		}); err != nil {
			return err
		}
		created = true
		_, err = AppendEvent(ctx, tx, nil, *run, store.EventConfirmRequired, store.EventPayload{
			ToolCall: &store.ToolInfo{CallID: call.ID, Tool: call.Name, Summary: "确认删除帖子", PayloadJSON: call.Arguments},
		})
		return err
	})
	if err != nil {
		return err
	}
	if created && e.Notify != nil {
		_ = e.Notify.Wake(persistCtx, run.ID)
	}
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		if e.cancelled(persistCtx, run) {
			return errRunCancelled
		}
		conf, err := e.Store.GetConfirmation(persistCtx, run.ID, call.ID)
		if err != nil {
			return err
		}
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

func expectedRevision(argsJSON string) (int64, error) {
	var args struct {
		ExpectedRevision int64 `json:"expected_revision"`
	}
	if err := json.Unmarshal([]byte(canonical.UnwrapArgsJSON(argsJSON)), &args); err != nil {
		return 0, err
	}
	return args.ExpectedRevision, nil
}

func (e *Engine) compact(workCtx, persistCtx context.Context, run *store.Run, session *store.Session, msgs []store.Message) error {
	run.Phase = store.PhaseCompact
	if err := e.updateRun(persistCtx, *run); err != nil {
		return err
	}
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
	var entries []memory.Entry
	if e.Memory != nil {
		var err error
		entries, err = e.Memory.Active(persistCtx, run.UserID)
		if err != nil {
			return err
		}
	}
	snap := prompt.BuildSnapshot(entries, HistoryTurns(selected), summary)
	session.PromptEpoch++
	session.PromptSnapshot = prompt.EncodeSnapshot(snap)
	session.CompactSummary = summary
	run.PromptEpoch = session.PromptEpoch
	run.Phase = store.PhaseModelRequest
	return e.step(persistCtx, *run, func(ctx context.Context, tx store.Store) error {
		if len(ids) > 0 {
			if err := tx.MarkMessagesCompacted(ctx, ids); err != nil {
				return err
			}
		}
		if err := tx.UpdateSession(ctx, *session); err != nil {
			return err
		}
		return tx.UpdateRun(ctx, *run)
	})
}

func (e *Engine) ensureStarted(ctx context.Context, run store.Run) error {
	seq, err := e.Store.MaxEventSeq(ctx, run.ID)
	if err != nil {
		return err
	}
	if seq == 0 {
		_, err = e.appendEvent(ctx, run, store.EventRunStarted, store.EventPayload{})
	}
	return err
}

func (e *Engine) resourceLimit(ctx context.Context, run store.Run) error {
	journals, err := e.Store.ListSuccessfulJournal(ctx, run.UserID, run.RequestID)
	if err != nil {
		return err
	}
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
	return e.finishWithMessage(ctx, run, status, eventType, payload, "", nil)
}

func (e *Engine) finishWithMessage(
	ctx context.Context,
	run store.Run,
	status, eventType string,
	payload store.EventPayload,
	message string,
	apiContent []byte,
) error {
	now := store.NowMs()
	run.Status = status
	run.Phase = store.PhaseDone
	run.EndedAtMs = now
	run.LastActivityAtMs = now
	if status == store.StatusCancelled {
		run.CancelRequested = true
	}
	if payload.ErrorCode != "" {
		run.ErrorCode = payload.ErrorCode
	}
	err := e.step(ctx, run, func(ctx context.Context, tx store.Store) error {
		if err := tx.UpdateRun(ctx, run); err != nil {
			return err
		}
		thread, err := tx.LockThread(ctx, run.UserID)
		if err != nil {
			return err
		}
		if message != "" && run.Source != store.SourceMemoryReview {
			if _, err := AppendEvent(ctx, tx, nil, run, store.EventToken, store.EventPayload{Text: message}); err != nil {
				return err
			}
			msg, err := tx.InsertMessage(ctx, store.Message{
				UserID: run.UserID, SessionID: run.SessionID, RunID: run.ID, Role: store.RoleAssistant,
				Kind: store.KindMessage, Content: message, APIContent: apiContent, Visible: true,
				Unread: run.Source == store.SourceWatch, CreatedAtMs: now,
			})
			if err != nil {
				return err
			}
			if err := tx.InsertOutbox(ctx, store.Outbox{
				UserID: run.UserID, MessageID: msg.ID, Op: store.IndexOpUpsert,
				PayloadJSON: string(mustJSON(map[string]any{
					"userId": run.UserID, "sessionId": run.SessionID, "messageId": msg.ID,
					"role": store.RoleAssistant, "content": message, "createdAtMs": now,
				})), CreatedAtMs: now,
			}); err != nil {
				return err
			}
			thread.LastMessageID = msg.ID
			thread.LastMessagePreview = store.Preview(message, 80)
			thread.LastMessageAtMs = now
			if run.Source == store.SourceWatch {
				thread.UnreadCount++
			}
		}
		if thread.ActiveRunID == run.ID {
			thread.ActiveRunID = 0
		}
		thread.UpdatedAtMs = now
		if err := tx.SaveThread(ctx, *thread); err != nil {
			return err
		}
		if run.Source == store.SourceUser && status == store.StatusDone {
			session, err := tx.GetSession(ctx, run.SessionID)
			if err != nil {
				return err
			}
			session.SuccessfulUserTurns++
			if err := tx.UpdateSession(ctx, *session); err != nil {
				return err
			}
			if session.SuccessfulUserTurns%10 == 0 {
				if _, err := tx.InsertRun(ctx, store.Run{
					UserID: run.UserID, SessionID: run.SessionID, RequestID: "review-" + itoa(now),
					Source: store.SourceMemoryReview, Status: store.StatusQueued, Phase: store.PhaseQueued,
					Priority: store.PriorityMemoryReview, ConsentVersion: run.ConsentVersion, InputVersion: 1,
					PromptEpoch: session.PromptEpoch, CreatedAtMs: now, LastActivityAtMs: now,
				}); err != nil {
					return err
				}
			}
		}
		if run.Source == store.SourceWatch {
			bucketID := watchBucketID(run.QueuedPayload)
			if bucketID > 0 {
				if err := tx.FinishWatchDelivery(ctx, bucketID, run.UserID, run.ID, status == store.StatusDone, now); err != nil {
					return err
				}
			}
		}
		_, err = AppendEvent(ctx, tx, nil, run, eventType, payload)
		return err
	})
	if err != nil {
		return err
	}
	if e.Notify != nil {
		_ = e.Notify.Wake(ctx, run.ID)
	}
	return nil
}

func watchBucketID(payload []byte) int64 {
	var parsed struct {
		BucketID int64 `json:"bucket_id"`
	}
	if json.Unmarshal(payload, &parsed) != nil {
		return 0
	}
	return parsed.BucketID
}

func ObserveQueueAge(seconds float64) { agentQueueAge.ObserveFloat(seconds) }
