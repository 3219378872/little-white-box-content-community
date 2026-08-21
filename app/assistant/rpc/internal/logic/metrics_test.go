package logic

import (
	"context"
	"errors"
	"testing"

	"esx/app/assistant/rpc/internal/llm"
	"esx/pkg/errx"

	clientprometheus "github.com/prometheus/client_golang/prometheus"
	zeroprometheus "github.com/zeromicro/go-zero/core/prometheus"
)

func TestToolCallMetricOutcomeIsBounded(t *testing.T) {
	tests := []struct {
		name           string
		err            error
		toolContextErr error
		want           string
	}{
		{name: "timeout", err: errors.New("rpc failed"), toolContextErr: context.DeadlineExceeded, want: "timeout"},
		{name: "canceled", err: context.Canceled, want: "canceled"},
		{name: "not allowed", err: errx.NewWithCode(errx.PermissionDenied), want: "not_allowed"},
		{name: "invalid request", err: errx.NewWithCode(errx.ParamError), want: "invalid_request"},
		{name: "unavailable", err: errors.New("downstream failed"), want: "unavailable"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := toolCallMetricOutcome(test.err, test.toolContextErr); got != test.want {
				t.Fatalf("toolCallMetricOutcome() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestAssistantMetricsAreExported(t *testing.T) {
	zeroprometheus.Enable()
	assistantToolCallsTotal.Inc("search", "success")
	assistantLLMCallsTotal.Inc("failure")
	assistantFirstTokenSeconds.ObserveFloat(0.25)
	observeLLMUsage(llm.Result{Model: "model-v1", Usage: llm.Usage{
		PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15, CostUSD: 0.001,
	}})

	assertMetricFamilies(t, []string{
		"esx_assistant_rpc_tool_calls_total",
		"esx_assistant_rpc_llm_calls_total",
		"esx_assistant_rpc_first_token_seconds",
		"esx_assistant_rpc_llm_tokens_total",
		"esx_assistant_rpc_llm_cost_usd_total",
	})
}

func assertMetricFamilies(t *testing.T, expected []string) {
	t.Helper()
	families, err := clientprometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatal(err)
	}
	found := make(map[string]struct{}, len(families))
	for _, family := range families {
		found[family.GetName()] = struct{}{}
	}
	for _, name := range expected {
		if _, ok := found[name]; !ok {
			t.Errorf("metric family %q was not exported", name)
		}
	}
}
