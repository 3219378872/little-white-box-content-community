package util

import (
	"errors"
	"strings"
	"testing"
)

func TestInitSnowflakeFromEnv(t *testing.T) {
	t.Run("未设置环境变量时使用默认值", func(t *testing.T) {
		if err := InitSnowflakeFromEnv(3, 1); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		id, err := NextID()
		if err != nil {
			t.Fatalf("NextID failed: %v", err)
		}
		if id <= 0 {
			t.Fatalf("id must be positive, got %d", id)
		}
	})

	t.Run("环境变量覆盖默认值", func(t *testing.T) {
		t.Setenv(EnvSnowflakeWorkerID, "5")
		t.Setenv(EnvSnowflakeDatacenterID, "2")
		if err := InitSnowflakeFromEnv(0, 1); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("非法格式报错并带变量名", func(t *testing.T) {
		t.Setenv(EnvSnowflakeWorkerID, "abc")
		err := InitSnowflakeFromEnv(0, 1)
		if err == nil || !strings.Contains(err.Error(), EnvSnowflakeWorkerID) {
			t.Fatalf("want error mentioning %s, got %v", EnvSnowflakeWorkerID, err)
		}
	})

	t.Run("越界 worker 返回 ErrInvalidWorkerID", func(t *testing.T) {
		t.Setenv(EnvSnowflakeWorkerID, "99")
		err := InitSnowflakeFromEnv(0, 1)
		if !errors.Is(err, ErrInvalidWorkerID) {
			t.Fatalf("want ErrInvalidWorkerID, got %v", err)
		}
	})

	t.Run("非法 datacenter 格式报错并带变量名", func(t *testing.T) {
		t.Setenv(EnvSnowflakeDatacenterID, "x")
		err := InitSnowflakeFromEnv(0, 1)
		if err == nil || !strings.Contains(err.Error(), EnvSnowflakeDatacenterID) {
			t.Fatalf("want error mentioning %s, got %v", EnvSnowflakeDatacenterID, err)
		}
	})
}
