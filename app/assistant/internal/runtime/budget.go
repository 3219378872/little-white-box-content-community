package runtime

import (
	"context"
	"fmt"
	"time"

	"esx/app/assistant/internal/llm"
	"esx/app/assistant/internal/prompt"
	"esx/app/assistant/internal/store"
)

const (
	HardRounds       = 500
	HardTools        = 1000
	HardIdle         = 30 * time.Minute
	HardAbsolute     = 6 * time.Hour
	HardOutputTokens = int64(1_000_000)
	MaxSingleOutput  = 65536

	WarnIdle   = 5 * time.Minute
	WarnRounds = 30
	WarnOutput = int64(100_000)
	CritIdle   = 20 * time.Minute
	CritRounds = 100
	CritOutput = int64(500_000)

	ReviewMaxRounds = 16
	ReviewMaxInput  = int64(600_000)
)

// SingleOutputLimit 取 provider 单次输出上限，未配置或超出系统上限时用系统上限。
func SingleOutputLimit(provider int) int {
	if provider <= 0 || provider > MaxSingleOutput {
		return MaxSingleOutput
	}
	return provider
}

// remainingOutputLimit 是本次请求可用的输出 token：单次上限与 run 剩余总额取小。
func remainingOutputLimit(run store.Run, provider int) int {
	return int(min(int64(SingleOutputLimit(provider)), max(int64(0), HardOutputTokens-run.OutputTokens)))
}

// reviewInputFits 预估记忆回顾本次请求后输入是否仍在回顾预算内；用户 run 不受此限。
func reviewInputFits(run store.Run, req llm.Request) bool {
	if run.Source != store.SourceMemoryReview {
		return true
	}
	estimate := EstimatePromptTokens(req.Messages) + EstimateTokens(string(prompt.EncodeTools(req.Tools))) + EstimateTokens(req.Convergence)
	return run.InputTokens+int64(estimate) <= ReviewMaxInput
}

// recordModelUsage 把一次模型调用的用量与费用累加到 run。
func recordModelUsage(run *store.Run, usage llm.Usage) {
	run.InputTokens += usage.PromptTokens
	run.OutputTokens += usage.CompletionTokens
	run.CacheTokens += usage.CacheTokens
	run.CacheWriteTokens += usage.CacheWriteTokens
	run.ReasoningTokens += usage.ReasoningTokens
	run.UsageEstimated = run.UsageEstimated || usage.Estimated
	run.CostUSD += usage.CostUSD
}

// Alarm 是某一预算维度达到告警阈值时注入给模型的收敛提示。
type Alarm struct {
	Level     string
	Dimension string
	Message   string
}

// EvaluateAlarms 按空闲时长、轮次与输出 token 计算告警；总时长目前只计算不告警。
func EvaluateAlarms(run store.Run, nowMs int64) []Alarm {
	idle := time.Duration(0)
	if run.LastActivityAtMs > 0 && nowMs > run.LastActivityAtMs {
		idle = time.Duration(nowMs-run.LastActivityAtMs) * time.Millisecond
	}
	elapsed := time.Duration(0)
	if run.StartedAtMs > 0 && nowMs > run.StartedAtMs {
		elapsed = time.Duration(nowMs-run.StartedAtMs) * time.Millisecond
	}
	_ = elapsed
	out := make([]Alarm, 0, 6)
	out = append(out, levelAlarms("time", idle, WarnIdle, CritIdle)...)
	out = append(out, countAlarms("rounds", run.Rounds, WarnRounds, CritRounds)...)
	out = append(out, tokenAlarms("output", run.OutputTokens, WarnOutput, CritOutput)...)
	return out
}

// levelAlarms 按时长阈值返回最高一级告警。
func levelAlarms(dim string, value, warn, crit time.Duration) []Alarm {
	if value >= crit {
		return []Alarm{{Level: "critical", Dimension: dim, Message: convergence(dim, "critical")}}
	}
	if value >= warn {
		return []Alarm{{Level: "warning", Dimension: dim, Message: convergence(dim, "warning")}}
	}
	return nil
}

// countAlarms 按计数阈值返回最高一级告警。
func countAlarms(dim string, value, warn, crit int) []Alarm {
	if value >= crit {
		return []Alarm{{Level: "critical", Dimension: dim, Message: convergence(dim, "critical")}}
	}
	if value >= warn {
		return []Alarm{{Level: "warning", Dimension: dim, Message: convergence(dim, "warning")}}
	}
	return nil
}

// tokenAlarms 按 token 阈值返回最高一级告警。
func tokenAlarms(dim string, value, warn, crit int64) []Alarm {
	if value >= crit {
		return []Alarm{{Level: "critical", Dimension: dim, Message: convergence(dim, "critical")}}
	}
	if value >= warn {
		return []Alarm{{Level: "warning", Dimension: dim, Message: convergence(dim, "warning")}}
	}
	return nil
}

// convergence 生成内部收敛提示，要求模型停止探索并尽快作答。
func convergence(dim, level string) string {
	return fmt.Sprintf("内部收敛提示：%s 已达 %s 阈值，停止探索性调用并尽快给出最终回答。该提示不得写入用户可见消息。", dim, level)
}

// HardLimitExceeded 判断 run 是否触及任一硬上限，触及后必须终止；记忆回顾另有更紧的上限。
func HardLimitExceeded(run store.Run, nowMs int64) bool {
	if run.Rounds >= HardRounds || run.ToolCalls >= HardTools || run.OutputTokens >= HardOutputTokens {
		return true
	}
	if run.LastActivityAtMs > 0 && nowMs-run.LastActivityAtMs >= HardIdle.Milliseconds() {
		return true
	}
	if run.StartedAtMs > 0 && nowMs-run.StartedAtMs >= HardAbsolute.Milliseconds() {
		return true
	}
	if run.Source == store.SourceMemoryReview {
		if run.Rounds >= ReviewMaxRounds || run.InputTokens >= ReviewMaxInput {
			return true
		}
	}
	return false
}

// RecordAlarms 记录本轮新触发的告警（每个维度与级别只记一次），返回需注入的第一条提示。
func RecordAlarms(ctx context.Context, st store.Store, run store.Run, nowMs int64) (string, error) {
	if st == nil {
		return "", nil
	}
	var injected string
	for _, alarm := range EvaluateAlarms(run, nowMs) {
		inserted, err := st.InsertAlert(ctx, store.Alert{RunID: run.ID, Level: alarm.Level, Dimension: alarm.Dimension, CreatedAtMs: nowMs})
		if err != nil {
			return injected, err
		}
		if inserted && injected == "" {
			injected = alarm.Message
		}
	}
	return injected, nil
}
