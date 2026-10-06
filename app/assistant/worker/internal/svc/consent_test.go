package svc

import (
	"testing"

	"esx/app/assistant/internal/llm"
	"esx/app/assistant/worker/internal/config"
)

func TestBuildLLMClientSkipsDisabledFallback(t *testing.T) {
	client, routeIDs, err := buildLLMClient(config.LLMConfig{
		Enabled: true, RouteID: "primary", Boundary: "default", WireAPI: llm.WireAPIResponses,
		Endpoint: "http://primary.test/v1", Model: "primary-model",
		TimeoutMs: 1000, MaxOutputTokens: 128, ContextWindowTokens: 4096,
		Fallbacks: []config.LLMRouteConfig{
			{Enabled: false, RouteID: "disabled"},
			{
				Enabled: true, RouteID: "fallback", Boundary: "default", WireAPI: llm.WireAPIChatCompletions,
				Endpoint: "http://fallback.test/v1", Model: "fallback-model",
				TimeoutMs: 1000, MaxOutputTokens: 128, ContextWindowTokens: 4096,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(routeIDs) != 2 || routeIDs[0] != "primary" || routeIDs[1] != "fallback" {
		t.Fatalf("route ids=%v", routeIDs)
	}
	capability := llm.Capability(client)
	if len(capability.FallbackRouteIDs) != 1 || capability.FallbackRouteIDs[0] != "fallback" {
		t.Fatalf("capability=%+v", capability)
	}
}
