package logic

import (
	"context"
	"errors"

	"esx/app/assistant/rpc/internal/llm"
	"esx/pkg/errx"

	"github.com/zeromicro/go-zero/core/metric"
)

var (
	assistantToolCallsTotal = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: "esx", Subsystem: "assistant_rpc", Name: "tool_calls_total",
		Help: "Assistant tool calls by tool and outcome", Labels: []string{"tool", "outcome"},
	})
	assistantLLMCallsTotal = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: "esx", Subsystem: "assistant_rpc", Name: "llm_calls_total",
		Help: "Assistant LLM calls by outcome", Labels: []string{"outcome"},
	})
	assistantFirstTokenSeconds = metric.NewHistogramVec(&metric.HistogramVecOpts{
		Namespace: "esx", Subsystem: "assistant_rpc", Name: "first_token_seconds",
		Help:    "Time from an accepted Assistant request to the first streamed token",
		Buckets: []float64{0.1, 0.25, 0.5, 1, 2, 5, 10},
	})
	assistantLLMTokensTotal = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: "esx", Subsystem: "assistant_rpc", Name: "llm_tokens_total",
		Help: "Assistant LLM token usage by configured model and token type", Labels: []string{"model", "type"},
	})
	assistantLLMCostUSDTotal = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: "esx", Subsystem: "assistant_rpc", Name: "llm_cost_usd_total",
		Help: "Estimated Assistant LLM cost in USD by configured model", Labels: []string{"model"},
	})
)

func observeLLMUsage(result llm.Result) {
	model := result.Model
	if model == "" {
		model = "unknown"
	}
	assistantLLMTokensTotal.Add(float64(result.Usage.PromptTokens), model, "prompt")
	assistantLLMTokensTotal.Add(float64(result.Usage.CompletionTokens), model, "completion")
	assistantLLMCostUSDTotal.Add(result.Usage.CostUSD, model)
}

func toolCallMetricOutcome(err, toolContextErr error) string {
	switch {
	case errors.Is(toolContextErr, context.DeadlineExceeded), errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(toolContextErr, context.Canceled), errors.Is(err, context.Canceled):
		return "canceled"
	case errx.Is(err, errx.PermissionDenied):
		return "not_allowed"
	case errx.Is(err, errx.ParamError):
		return "invalid_request"
	default:
		return "unavailable"
	}
}
