package runtime

import (
	"context"
	"errors"
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
	"strings"
	"time"
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
	Store      store.Store
	Memory     memory.Store
	Watch      watch.Store
	Tools      *tool.Registry
	LLM        llm.Client
	AuxLLM     llm.Client
	ReviewLLM  llm.Client
	Notify     store.Notifier
	WatchPosts WatchPostVisibility
	Window     int
	Provider   int
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
		if errors.Is(err, errRunTerminated) || errors.Is(err, errRunWaiting) {
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
		suppressText := false
		if registry.Has(tool.PublishAnswer) {
			sources, sourceErr := e.Store.ListSources(persistCtx, run.ID)
			if sourceErr != nil {
				return sourceErr
			}
			suppressText = run.Source == store.SourceWatch || len(sources) > 0
		}
		result, err := e.completeModel(workCtx, persistCtx, run, modelClient, llm.Request{
			SuppressText: suppressText,
			Messages:     turns,
			Tools:        registry.Definitions(),
			MaxTokens:    SingleOutputLimit(modelClient.MaxOutputTokens()),
			Convergence:  convergence,
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
			if strings.TrimSpace(result.Text) != "" && run.Source == store.SourceUser {
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
			if suppressText {
				return e.fail(persistCtx, run, "ANSWER_VALIDATION_FAILED", "检索回答缺少结构化引用，未发布未经校验的结论")
			}
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
				RequestID: run.RequestID, Source: run.Source, ConsentVersion: run.ConsentVersion, ClientProtocolVersion: clientProtocol(run)}
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
		if len(calls) > 1 && requiresExclusiveRound(calls) {
			for _, call := range calls {
				digest, err := canonical.DigestArgs(call.Arguments)
				if err != nil {
					digest = "invalid:" + call.ID
				}
				if _, _, err := e.startToolStep(persistCtx, run, call, digest, false); err != nil {
					return err
				}
				problem := errx.New(errx.ParamError, "ask_questions and publish_answer require an exclusive tool round")
				if err := e.finishToolStep(persistCtx, &run, call, problem.Error(), problem, nil, nil, nil, true, "invalid", &reviewLive); err != nil {
					return err
				}
			}
			for _, call := range calls {
				if err := e.guardToolProgress(persistCtx, &run, registry, call, &reviewLive); err != nil {
					return err
				}
			}
			continue
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

func ObserveQueueAge(seconds float64) { agentQueueAge.ObserveFloat(seconds) }
