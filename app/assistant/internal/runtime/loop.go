package runtime

import (
	"context"
	"errors"
	"esx/app/assistant/internal/llm"
	"esx/app/assistant/internal/memory"
	"esx/app/assistant/internal/store"
	"esx/app/assistant/internal/tool"
	"time"

	"esx/pkg/logging"
	metric "esx/pkg/metrics"
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

// Engine 执行已被 worker 领取的 run：组装提示词、调用模型、执行工具并把结果写回 store。
type Engine struct {
	Store     store.Store
	Memory    memory.Store
	Tools     *tool.Registry
	LLM       llm.Client
	AuxLLM    llm.Client
	ReviewLLM llm.Client
	Window    int
	Provider  int
}

// Execute 在租约内跑完一个 run。persistCtx 用于写库，workCtx 在取消、失去租约或撤销授权时被提前取消，
// 使模型与工具调用尽快停止而收尾写入仍能完成。
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

	logger := logging.WithContext(persistCtx)
	// 接管过期租约时，先为上一个 worker 未结束的流补发 reset，避免客户端拼接两段回答。
	if recovered {
		if err := e.resetRecoveredStreams(persistCtx, run); err != nil {
			if !errors.Is(err, store.ErrLeaseLost) {
				logger.Errorw("assistant-agent reset recovered stream failed", logging.Field("runId", run.ID), logging.Field("err", err.Error()))
			}
			return
		}
	}
	// 失去租约或 run 已终结时不再写入；其余未预期错误把仍在运行的 run 标记为失败。
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
		logger.Errorw("assistant-agent run failed", logging.Field("runId", run.ID), logging.Field("err", err.Error()))
		if fresh, getErr := e.ownedRun(persistCtx, run); getErr == nil &&
			(fresh.Status == store.StatusRunning || fresh.Status == store.StatusQueued) {
			_ = e.fail(persistCtx, *fresh, "RUN_FAILED", "助手暂时无法完成这个请求")
		}
	}
}

// step 在租约栅栏内提交一次写入；租约丢失时返回 ErrLeaseLost。
func (e *Engine) step(ctx context.Context, run store.Run, fn func(context.Context, store.Store) error) error {
	return e.Store.RunStep(ctx, run.Fence(), fn)
}

// updateRun 在租约栅栏内保存 run 的进度与计量。
func (e *Engine) updateRun(ctx context.Context, run store.Run) error {
	return e.step(ctx, run, func(ctx context.Context, tx store.Store) error {
		return tx.UpdateRun(ctx, run)
	})
}

// appendEvent 在租约栅栏内追加公开事件，订阅方按 seq 轮询读取。
func (e *Engine) appendEvent(ctx context.Context, run store.Run, eventType string, payload store.EventPayload) (store.Event, error) {
	var event store.Event
	err := e.step(ctx, run, func(ctx context.Context, tx store.Store) error {
		var err error
		event, err = AppendEvent(ctx, tx, run, eventType, payload)
		return err
	})
	return event, err
}

// run 准备执行状态后循环迭代，直到某一轮报告结束或出错。
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
