package runtime

import (
	"context"
	"errors"
	"esx/app/assistant/internal/llm"
	"esx/app/assistant/internal/memory"
	"esx/app/assistant/internal/store"
	"esx/app/assistant/internal/tool"
	"esx/app/assistant/watch"
	"time"

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
		if err := e.resetRecoveredStreams(persistCtx, run); err != nil {
			if !errors.Is(err, store.ErrLeaseLost) {
				logger.Errorw("assistant-agent reset recovered stream failed", logx.Field("runId", run.ID), logx.Field("err", err.Error()))
			}
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
			if fresh, getErr := e.ownedRun(persistCtx, run); getErr == nil &&
				!store.IsTerminalStatus(fresh.Status) {
				_ = e.cancel(persistCtx, *fresh)
			}
			return
		}
		logger.Errorw("assistant-agent run failed", logx.Field("runId", run.ID), logx.Field("err", err.Error()))
		if fresh, getErr := e.ownedRun(persistCtx, run); getErr == nil &&
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
	execution, err := e.prepareExecution(persistCtx, run)
	if execution == nil || err != nil {
		return err
	}
	for {
		action, err := execution.iterate(workCtx, persistCtx)
		if err != nil || action == iterationFinished {
			return err
		}
	}
}

func ObserveQueueAge(seconds float64) { agentQueueAge.ObserveFloat(seconds) }
