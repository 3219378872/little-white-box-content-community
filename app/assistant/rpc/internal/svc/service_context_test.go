package svc

import (
	"testing"

	"esx/app/assistant/rpc/internal/config"
)

func TestNewServiceContextRejectsInvalidSafetyConfig(t *testing.T) {
	for name, safety := range map[string]config.SafetyConfig{
		"no blocked terms":   {Enabled: true, MaxScanRunes: 100},
		"no scan rune limit": {Enabled: true, BlockedTerms: []string{"制作炸弹"}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NewServiceContext(config.Config{Safety: safety}); err == nil {
				t.Fatal("an enabled but unusable safety filter must fail startup")
			}
		})
	}
}
