package config

import (
	"os"
	"strings"
	"testing"

	"github.com/zeromicro/go-zero/core/conf"
)

func TestConfigValidateRequiresCursorSecret(t *testing.T) {
	t.Setenv("RPC_INTERNAL_SECRET", "test-internal-secret")
	for _, secret := range []string{"", " \t\n"} {
		if err := (Config{CursorSecret: secret}).Validate(); err == nil {
			t.Fatalf("Validate() accepted blank CursorSecret %q", secret)
		}
	}

	if err := (Config{CursorSecret: "feed-cursor-secret", InternalSecret: "test-internal-secret"}).Validate(); err != nil {
		t.Fatalf("Validate() rejected configured CursorSecret: %v", err)
	}
}

func TestFeedYAMLRejectsMissingCursorSecret(t *testing.T) {
	t.Setenv("RPC_INTERNAL_SECRET", "test-internal-secret")
	previous, existed := os.LookupEnv("FEED_CURSOR_SECRET")
	if err := os.Unsetenv("FEED_CURSOR_SECRET"); err != nil {
		t.Fatalf("unset FEED_CURSOR_SECRET: %v", err)
	}
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv("FEED_CURSOR_SECRET", previous)
		} else {
			_ = os.Unsetenv("FEED_CURSOR_SECRET")
		}
	})

	var c Config
	err := conf.Load("../../etc/feed.yaml", &c, conf.UseEnv())
	if err == nil {
		t.Fatal("conf.Load accepted feed.yaml without FEED_CURSOR_SECRET")
	}
	if !strings.Contains(err.Error(), "FEED_CURSOR_SECRET") {
		t.Fatalf("conf.Load error = %q, want FEED_CURSOR_SECRET guidance", err)
	}
}

func TestFeedYAMLRejectsEmptyCursorSecret(t *testing.T) {
	t.Setenv("RPC_INTERNAL_SECRET", "test-internal-secret")
	t.Setenv("FEED_CURSOR_SECRET", "")

	var c Config
	err := conf.Load("../../etc/feed.yaml", &c, conf.UseEnv())
	if err == nil {
		t.Fatal("conf.Load accepted feed.yaml with an empty FEED_CURSOR_SECRET")
	}
	if !strings.Contains(err.Error(), "FEED_CURSOR_SECRET") {
		t.Fatalf("conf.Load error = %q, want FEED_CURSOR_SECRET guidance", err)
	}
}

func TestFeedYAMLLoadsCursorSecretFromEnvironment(t *testing.T) {
	t.Setenv("RPC_INTERNAL_SECRET", "test-internal-secret")
	t.Setenv("FEED_CURSOR_SECRET", "configured-feed-cursor-secret")

	var c Config
	if err := conf.Load("../../etc/feed.yaml", &c, conf.UseEnv()); err != nil {
		t.Fatalf("load feed.yaml: %v", err)
	}
	if c.CursorSecret != "configured-feed-cursor-secret" {
		t.Fatalf("CursorSecret = %q", c.CursorSecret)
	}
}
