package agent

import (
	"context"
	"errors"

	"github.com/zeromicro/go-zero/core/metric"
)

var (
	metricAgentToolCallsTotal = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: "esx", Subsystem: "assistant_rpc", Name: "agent_tool_calls_total",
		Help: "Agent mode tool calls by tool and outcome", Labels: []string{"tool", "outcome"},
	})
	metricAgentConfirmsTotal = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: "esx", Subsystem: "assistant_rpc", Name: "agent_confirms_total",
		Help: "Agent high-risk confirmations by outcome", Labels: []string{"outcome"},
	})
)

// observeToolOutcome 统一记录工具调用结果（AGNT-052 审计的指标侧写）。
func observeToolOutcome(toolName string, err error) {
	outcome := "success"
	switch {
	case err == nil:
	case errors.Is(err, context.Canceled):
		outcome = "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		outcome = "timeout"
	default:
		outcome = "failed"
	}
	metricAgentToolCallsTotal.Add(1, toolName, outcome)
}
