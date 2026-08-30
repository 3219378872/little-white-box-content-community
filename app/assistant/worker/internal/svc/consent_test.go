package svc

import (
	"testing"

	"esx/app/assistant/internal/llm"
	"esx/app/assistant/worker/internal/config"
	"esx/app/user/rpc/userservice"
)

func TestCurrentConsentGrantedRequiresLatestVersion(t *testing.T) {
	tests := []struct {
		consent *userservice.GetAgentCapabilityConsentResp
		want    bool
	}{
		{consent: nil, want: false},
		{consent: &userservice.GetAgentCapabilityConsentResp{Granted: false, ConsentVersion: 2, CurrentVersion: 2}, want: false},
		{consent: &userservice.GetAgentCapabilityConsentResp{Granted: true, ConsentVersion: 1, CurrentVersion: 2}, want: false},
		{consent: &userservice.GetAgentCapabilityConsentResp{Granted: true, ConsentVersion: 2, CurrentVersion: 2}, want: true},
		{consent: &userservice.GetAgentCapabilityConsentResp{Granted: true, ConsentVersion: 2, CurrentVersion: 3}, want: false},
	}
	for _, test := range tests {
		if got := currentConsentGranted(test.consent); got != test.want {
			t.Fatalf("consent=%+v got=%v want=%v", test.consent, got, test.want)
		}
	}
}

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
