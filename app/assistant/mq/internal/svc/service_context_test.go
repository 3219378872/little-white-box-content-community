package svc

import (
	"testing"

	"esx/app/assistant/mq/internal/config"
)

func TestNewServiceContextRequiresDataSource(t *testing.T) {
	_, err := NewServiceContext(config.Config{})
	if err == nil {
		t.Fatal("expected error")
	}
}
