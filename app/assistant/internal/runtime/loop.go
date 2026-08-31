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
	errRunTerminated    = errors.New("assistant run terminated")
	errCompactNoGain    = errors.New("assistant compact did not reduce context")
	cancelWatchInterval = 50 * time.Millisecond
)

type Engine struct {
	Store     store.Store
	Memory    memory.Store
	Watch     watch.Store
	Tools     *tool.Registry
	LLM       llm.Client
	AuxLLM    llm.Client
	ReviewLLM llm.Client
	Notify    store.Notifier
	Window    int
	Provider  int
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
	if recovered {
		if err := e.resetRecoveredStreams(persistCtx, run); err != nil && !errors.Is(err, store.ErrLeaseLost) {
			logger.Errorw("assistant-agent reset recovered stream failed", logx.Field("runId", run.ID), logx.Field("err", err.Error()))
			return
		}
	}
	if err := e.run(workCtx, persistCtx, run); err != nil {
		if errors.Is(err, store.ErrLeaseLost) || persistCtx.Err() != nil {
			return
		}
		if errors.Is(err, errRunTerminated) {
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
	registry, modelClient, err := e.loadCapabilities(persistCtx, run, session)
	if err != nil {
		return err
	}
	if run.Source == store.SourceMemoryReview && e.ReviewLLM != nil {
		modelClient = e.ReviewLLM
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

		if err := e.ensureWatchInput(persistCtx, run); err != nil {
			return err
		}
		msgs, err := e.Store.ListSessionMessages(persistCtx, run.UserID, run.SessionID, true)
		if err != nil {
			return err
		}
		window := e.Window
		if modelClient != nil && modelClient.ContextWindowTokens() > 0 {
			window = modelClient.ContextWindowTokens()
		}
		if ShouldCompactWithAnchor(msgs, window, run.ID, run.LastPromptTokens) {
			if err := e.compact(workCtx, persistCtx, &run, session, msgs, modelClient); err != nil {
				if errors.Is(err, errRunCancelled) {
					return e.cancel(persistCtx, run)
				}
				if errors.Is(err, errCompactNoGain) {
					if finishErr := e.fail(persistCtx, run, "COMPACT_NO_GAIN", "会话压缩未能降低上下文"); finishErr != nil {
						return finishErr
					}
					return errRunTerminated
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
			registry, modelClient, err = e.loadCapabilities(persistCtx, run, session)
			if err != nil {
				return err
			}
			if run.Source == store.SourceMemoryReview && e.ReviewLLM != nil {
				modelClient = e.ReviewLLM
			}
			continue
		}

		history := promptHistory(msgs, run)
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
			history = promptHistory(msgs, run)
		}
		snap.History = history
		turns := prompt.Messages(snap)
		seen := make(map[int64]struct{}, len(msgs))
		for _, msg := range msgs {
			if !msg.Compacted {
				seen[msg.ID] = struct{}{}
			}
		}
		pending, queuedThrough, err := e.pendingUserTurns(persistCtx, run, seen, history)
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

		if modelClient == nil {
			return e.fail(persistCtx, run, "LLM_DISABLED", "model is not configured")
		}
		result, err := e.completeModel(workCtx, persistCtx, run, modelClient, llm.Request{
			Messages:    turns,
			Tools:       registry.Definitions(),
			MaxTokens:   SingleOutputLimit(modelClient.MaxOutputTokens()),
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
			if strings.TrimSpace(result.Text) != "" && run.Source != store.SourceMemoryReview {
				payload := store.EventPayload{ErrorCode: "LLM_UNAVAILABLE", Text: "模型调用失败", Partial: result.Text}
				return e.finishWithMessageEvent(persistCtx, run, store.StatusError, store.EventError, payload,
					result.Text, prompt.EncodeTurn(prompt.Turn{Role: store.RoleAssistant, Content: result.Text}), !result.Streamed, result.StreamID)
			}
			return e.fail(persistCtx, run, "LLM_UNAVAILABLE", "model call failed")
		}
		if aborted, abortErr := e.abortIfRequested(persistCtx, run); aborted {
			return abortErr
		}
		run.Rounds++
		run.InputTokens += result.Usage.PromptTokens
		run.OutputTokens += result.Usage.CompletionTokens
		run.CacheTokens += result.Usage.CacheTokens
		run.CacheWriteTokens += result.Usage.CacheWriteTokens
		run.ReasoningTokens += result.Usage.ReasoningTokens
		run.UsageEstimated = run.UsageEstimated || result.Usage.Estimated
		if result.Usage.PromptTokens > 0 {
			run.LastPromptTokens = result.Usage.PromptTokens
		}
		run.CostUSD += result.Usage.CostUSD
		run.LastActivityAtMs = store.NowMs()
		result.Text = prompt.SanitizeOutput(result.Text)
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

func (e *Engine) loadCapabilities(ctx context.Context, run store.Run, session *store.Session) (*tool.Registry, llm.Client, error) {
	if session == nil {
		return nil, nil, errors.New("assistant session is nil")
	}
	capability, ok := prompt.DecodeCapabilities(session.ToolSnapshot)
	changed := false
	if !ok {
		var buildErr error
		capability, buildErr = e.buildCapabilitySnapshot(run)
		if buildErr != nil {
			return nil, nil, e.fail(ctx, run, "TOOLS_UNAVAILABLE", buildErr.Error())
		}
		changed = true
	} else if capability.Version == 0 {
		capability.Version = prompt.CapabilitySnapshotVersion
		capability.Provider = llm.Capability(e.LLM)
		changed = true
	}
	if !capability.Provider.Tools || strings.TrimSpace(capability.Provider.RouteID) == "" {
		return nil, nil, e.fail(ctx, run, "PROVIDER_CAPABILITY_UNAVAILABLE", "frozen provider route is unavailable")
	}
	base := e.Tools.ResolveDefinitions(capability.Tools)
	registry := tool.ForSource(base, run.Source, run.ConsentVersion)
	if registry == nil || (run.Source == store.SourceMemoryReview && len(registry.Definitions()) == 0) {
		return nil, nil, e.fail(ctx, run, "TOOLS_UNAVAILABLE", "no frozen tools for run source")
	}
	client, routeOK := llm.SelectCapability(e.LLM, capability.Provider)
	if !routeOK {
		return nil, nil, e.fail(ctx, run, "PROVIDER_ROUTE_UNAVAILABLE", "frozen provider route is unavailable")
	}
	if changed {
		session.ToolSnapshot = prompt.EncodeCapabilities(capability)
		if err := e.step(ctx, run, func(ctx context.Context, tx store.Store) error {
			return tx.UpdateSession(ctx, *session)
		}); err != nil {
			return nil, nil, err
		}
	}
	return registry, client, nil
}

func (e *Engine) buildCapabilitySnapshot(run store.Run) (prompt.CapabilitySnapshot, error) {
	base := tool.ForSource(e.Tools, store.SourceUser, run.ConsentVersion)
	if base == nil {
		return prompt.CapabilitySnapshot{}, errors.New("no available tools")
	}
	provider := llm.Capability(e.LLM)
	if !provider.Tools || strings.TrimSpace(provider.RouteID) == "" {
		return prompt.CapabilitySnapshot{}, errors.New("provider capability is unavailable")
	}
	return prompt.CapabilitySnapshot{
		Version:  prompt.CapabilitySnapshotVersion,
		Tools:    base.Definitions(),
		Provider: provider,
	}, nil
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
		return e.finishWithMessageEvent(ctx, run, store.StatusError, store.EventError, payload, partial,
			prompt.EncodeTurn(prompt.Turn{Role: store.RoleAssistant, Content: partial}), !result.Streamed, result.StreamID)
	}
	return e.finish(ctx, run, store.StatusError, store.EventError, payload)
}

func keepsStreamedAnswer(names []string) bool {
	if len(names) == 0 {
		return false
	}
	for _, name := range names {
		if name != tool.PresentSources {
			return false
		}
	}
	return true
}

func toolCallNames(calls []llm.ToolCall) []string {
	names := make([]string, 0, len(calls))
	for _, call := range calls {
		names = append(names, call.Name)
	}
	return names
}

func turnToolNames(turn prompt.Turn) []string {
	names := make([]string, 0, len(turn.ToolCalls))
	for _, call := range turn.ToolCalls {
		names = append(names, call.Name)
	}
	return names
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

func (e *Engine) completeModel(workCtx, persistCtx context.Context, run store.Run, client llm.Client, req llm.Request) (llm.Result, error) {
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
	writer := newModelStreamWriter(e, persistCtx, run)
	req.AttemptPrefix = writer.prefix
	previousObserver := req.Observer
	req.Observer = func(event llm.AttemptEvent) error {
		if err := writer.Observe(event); err != nil {
			return err
		}
		if previousObserver != nil {
			return previousObserver(event)
		}
		return nil
	}
	var result llm.Result
	if streaming, ok := client.(llm.StreamingClient); ok {
		result, err = streaming.CompleteStream(callCtx, req, writer.Delta)
		if flushErr := writer.Finish(); err == nil && flushErr != nil {
			err = flushErr
		}
		if writer.Emitted() {
			result.Text = writer.Text()
			result.Streamed = true
			result.StreamID = writer.StreamID()
		}
		if err == nil && len(result.ToolCalls) > 0 && writer.Emitted() &&
			!keepsStreamedAnswer(toolCallNames(result.ToolCalls)) {
			if resetErr := writer.ResetWithRun(run); resetErr != nil {
				err = resetErr
			}
		}
	} else {
		result, err = client.Complete(callCtx, req)
		result.Text = prompt.SanitizeOutput(result.Text)
	}
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
		_ = writer.ResetWithRun(*fresh)
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
			Visible:     strings.TrimSpace(turn.Content) != "" && keepsStreamedAnswer(turnToolNames(turn)),
			CreatedAtMs: store.NowMs(),
		})
		return err
	})
}

func (e *Engine) pendingUserTurns(ctx context.Context, run store.Run, seen map[int64]struct{}, history []prompt.Turn) ([]prompt.Turn, int64, error) {
	out := make([]prompt.Turn, 0)
	if run.Source == store.SourceWatch {
		if !historyHasWatchInput(history) {
			turn, err := e.watchInputTurn(ctx, run)
			if err != nil {
				return nil, 0, err
			}
			out = append(out, turn)
		}
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

	journal, reserved, err := e.startToolStep(persistCtx, *run, call, digest, prepErr == nil && registry.SideEffect(call.Name))
	if err != nil {
		return err
	}
	if journal != nil && journal.Status == store.JournalSuccess {
		agentToolCalls.Inc(call.Name, "replay")
		text := decodeToolResultText(journal.ResultJSON)
		sess.ChangeIDs = decodeToolResultChangeIDs(journal.ResultJSON)
		if err := e.finishToolStep(persistCtx, run, call, text, nil, nil, sess.ChangeIDs, journal, false, "replay", reviewLive); err != nil {
			return err
		}
		return e.guardToolProgress(persistCtx, run, registry, call, reviewLive)
	}
	if journal != nil && !reserved && journal.Status == store.JournalPending {
		return errors.New("side effect command is already in progress")
	}
	if prepErr != nil {
		text := prepErr.Error()
		if err := e.finishToolStep(persistCtx, run, call, text, prepErr, nil, nil, journal, true, "invalid", reviewLive); err != nil {
			return err
		}
		return e.guardToolProgress(persistCtx, run, registry, call, reviewLive)
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
	if registry.SideEffect(call.Name) {
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
	if err := e.guardToolProgress(persistCtx, run, registry, call, reviewLive); err != nil {
		return err
	}
	if e.cancelled(persistCtx, run) {
		return errRunCancelled
	}
	return nil
}

func (e *Engine) guardToolProgress(ctx context.Context, run *store.Run, registry *tool.Registry, current llm.ToolCall, reviewLive *[]prompt.Turn) error {
	if run == nil || registry == nil || registry.Poller(current.Name) {
		return nil
	}
	calls, err := e.Store.ListToolCalls(ctx, run.ID)
	if err != nil {
		return err
	}
	var currentRow *store.ToolCall
	for i := range calls {
		if calls[i].CallID == current.ID {
			currentRow = &calls[i]
			break
		}
	}
	if currentRow == nil || currentRow.Status == "running" || currentRow.ResultJSON == "" {
		return nil
	}
	resultDigest, err := canonical.DigestArgs(currentRow.ResultJSON)
	if err != nil {
		resultDigest = strings.Join(strings.Fields(currentRow.ResultJSON), " ")
	}
	count := 0
	for _, call := range calls {
		if call.Tool != currentRow.Tool || call.CanonicalArgsDigest != currentRow.CanonicalArgsDigest || call.Status == "running" {
			continue
		}
		digest, digestErr := canonical.DigestArgs(call.ResultJSON)
		if digestErr != nil {
			digest = strings.Join(strings.Fields(call.ResultJSON), " ")
		}
		if digest == resultDigest {
			count++
		}
	}
	if count < 2 {
		return nil
	}
	if count == 2 {
		turn := prompt.Turn{Role: store.RoleSystem, Content: "工具无进展：相同工具、参数和结果已重复。请改变方法、根据现有结果作答，或明确说明限制；不要原样再次调用。"}
		if run.Source == store.SourceMemoryReview {
			if reviewLive != nil {
				*reviewLive = append(*reviewLive, turn)
			}
			return nil
		}
		return e.step(ctx, *run, func(ctx context.Context, tx store.Store) error {
			_, err := tx.InsertMessage(ctx, store.Message{
				UserID: run.UserID, SessionID: run.SessionID, RunID: run.ID, Role: store.RoleSystem,
				Kind: store.KindTool, Content: "", APIContent: prompt.EncodeTurn(turn), Visible: false, CreatedAtMs: store.NowMs(),
			})
			return err
		})
	}
	if err := e.fail(ctx, *run, "TOOL_NO_PROGRESS", "工具调用连续重复且没有进展"); err != nil {
		return err
	}
	return errRunTerminated
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

func (e *Engine) compact(workCtx, persistCtx context.Context, run *store.Run, session *store.Session, msgs []store.Message, mainClient llm.Client) error {
	run.Phase = store.PhaseCompact
	if err := e.updateRun(persistCtx, *run); err != nil {
		return err
	}
	msgs = liveMessages(msgs)
	keep := EstimateMessageTokens(msgs) / 5
	if keep < 1 {
		keep = 1
	}
	selected := SelectKeep(msgs, keep, unfinishedCallIDs(msgs), run.ID)
	keepIDs := make(map[int64]struct{}, len(selected))
	for _, msg := range selected {
		keepIDs[msg.ID] = struct{}{}
	}
	dropped := make([]store.Message, 0, len(msgs)-len(selected))
	for _, msg := range msgs {
		if _, keep := keepIDs[msg.ID]; !keep {
			dropped = append(dropped, msg)
		}
	}
	summary := "压缩摘要：较早对话未包含可保留的用户可见内容。"
	summaryClient := e.AuxLLM
	if summaryClient == nil {
		summaryClient = mainClient
	}
	if summaryClient != nil {
		budget := summaryClient.ContextWindowTokens() / 4
		if budget <= 0 || budget > 32_000 {
			budget = 32_000
		}
		if budget < 2_000 {
			budget = 2_000
		}
		input := SummaryInput(dropped, budget)
		if input != "" {
			result, err := summaryClient.Complete(workCtx, llm.Request{
				Messages:     []prompt.Turn{{Role: store.RoleSystem, Content: "用中文压缩以下会话，不要引入新事实。"}, {Role: store.RoleUser, Content: input}},
				DisableTools: true,
				MaxTokens:    512,
			})
			if err != nil {
				if errors.Is(err, context.Canceled) && e.cancelled(persistCtx, run) {
					return errRunCancelled
				}
				return err
			}
			if strings.TrimSpace(result.Text) == "" {
				return errCompactNoGain
			}
			summary = prompt.SanitizeOutput(result.Text)
			run.InputTokens += result.Usage.PromptTokens
			run.OutputTokens += result.Usage.CompletionTokens
			run.CacheTokens += result.Usage.CacheTokens
			run.CacheWriteTokens += result.Usage.CacheWriteTokens
			run.ReasoningTokens += result.Usage.ReasoningTokens
			run.UsageEstimated = run.UsageEstimated || result.Usage.Estimated
			run.CostUSD += result.Usage.CostUSD
		}
	}
	if e.cancelled(persistCtx, run) {
		return errRunCancelled
	}
	capability, err := e.buildCapabilitySnapshot(*run)
	if err != nil {
		return err
	}
	toolSnapshot := prompt.EncodeCapabilities(capability)
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
	beforeSnapshot, ok := prompt.DecodeSnapshot(session.PromptSnapshot)
	if !ok {
		return errors.New("assistant prompt snapshot is invalid before compact")
	}
	beforeSnapshot.History = HistoryTurns(msgs)
	beforeTokens := EstimatePromptTokens(prompt.Messages(beforeSnapshot)) + EstimateTokens(string(session.ToolSnapshot))
	if run.LastPromptTokens > int64(beforeTokens) {
		beforeTokens = int(run.LastPromptTokens)
	}
	afterTokens := EstimatePromptTokens(prompt.Messages(snap)) + EstimateTokens(string(toolSnapshot))
	window := e.Window
	if mainClient != nil && mainClient.ContextWindowTokens() > 0 {
		window = mainClient.ContextWindowTokens()
	}
	target := window / 2
	if target <= 0 {
		target = 64_000
	}
	if afterTokens >= beforeTokens || afterTokens >= target {
		return errCompactNoGain
	}
	session.PromptEpoch++
	session.PromptSnapshot = prompt.EncodeSnapshot(snap)
	session.ToolSnapshot = toolSnapshot
	session.CompactSummary = summary
	run.PromptEpoch = session.PromptEpoch
	run.LastPromptTokens = int64(afterTokens)
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
	return e.finishWithMessageEvent(ctx, run, status, eventType, payload, message, apiContent, true, "")
}

func (e *Engine) finishWithMessageEvent(
	ctx context.Context,
	run store.Run,
	status, eventType string,
	payload store.EventPayload,
	message string,
	apiContent []byte,
	emitToken bool,
	streamID string,
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
			if emitToken {
				if _, err := AppendEvent(ctx, tx, nil, run, store.EventToken, store.EventPayload{Text: message, StreamID: streamID}); err != nil {
					return err
				}
			}
			if len(apiContent) == 0 {
				apiContent = prompt.EncodeTurn(prompt.Turn{Role: store.RoleAssistant, Content: message})
			}
			if streamID != "" && payload.StreamID == "" {
				payload.StreamID = streamID
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
