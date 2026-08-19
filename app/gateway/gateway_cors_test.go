package main

import (
	"slices"
	"testing"
)

func TestCorsOriginsIncludeLocalHTTP2(t *testing.T) {
	t.Setenv("GATEWAY_CORS_ORIGINS", "")
	got := corsOrigins()
	for _, origin := range []string{
		"http://localhost:3002",
		"http://127.0.0.1:3002",
		"https://localhost:3443",
		"https://127.0.0.1:3443",
	} {
		if !slices.Contains(got, origin) {
			t.Errorf("corsOrigins() missing %q: %v", origin, got)
		}
	}
}
