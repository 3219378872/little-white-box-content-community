package runtime

import (
	"context"
	"fmt"
	"time"

	"esx/app/assistant/internal/store"
	"esx/pkg/errx"
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

type BudgetConfig struct {
	ProviderMaxOutput int
}

func SingleOutputLimit(provider int) int {
	if provider <= 0 || provider > MaxSingleOutput {
		return MaxSingleOutput
	}
	return provider
}

type Alarm struct {
	Level     string
	Dimension string
	Message   string
}

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

func levelAlarms(dim string, value, warn, crit time.Duration) []Alarm {
	if value >= crit {
		return []Alarm{{Level: "critical", Dimension: dim, Message: convergence(dim, "critical")}}
	}
	if value >= warn {
		return []Alarm{{Level: "warning", Dimension: dim, Message: convergence(dim, "warning")}}
	}
	return nil
}

func countAlarms(dim string, value, warn, crit int) []Alarm {
	if value >= crit {
		return []Alarm{{Level: "critical", Dimension: dim, Message: convergence(dim, "critical")}}
	}
	if value >= warn {
		return []Alarm{{Level: "warning", Dimension: dim, Message: convergence(dim, "warning")}}
	}
	return nil
}

func tokenAlarms(dim string, value, warn, crit int64) []Alarm {
	if value >= crit {
		return []Alarm{{Level: "critical", Dimension: dim, Message: convergence(dim, "critical")}}
	}
	if value >= warn {
		return []Alarm{{Level: "warning", Dimension: dim, Message: convergence(dim, "warning")}}
	}
	return nil
}

func convergence(dim, level string) string {
	return fmt.Sprintf("内部收敛提示：%s 已达 %s 阈值，停止探索性调用并尽快给出最终回答。该提示不得写入用户可见消息。", dim, level)
}

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

func ResourceLimitError() error {
	return errx.NewWithCode(errx.AgentResourceLimit)
}

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
