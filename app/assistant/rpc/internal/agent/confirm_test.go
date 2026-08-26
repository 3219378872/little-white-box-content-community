package agent

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeConfirmRedis 是最小 Redis 语义替身：SETEX/GET/DEL/EXISTS。
type fakeConfirmRedis struct {
	mu    sync.Mutex
	store map[string]string
}

func newFakeConfirmRedis() *fakeConfirmRedis {
	return &fakeConfirmRedis{store: map[string]string{}}
}

func (f *fakeConfirmRedis) GetCtx(_ context.Context, key string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.store[key], nil
}

func (f *fakeConfirmRedis) SetexCtx(_ context.Context, key, value string, _ int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.store[key] = value
	return nil
}

func (f *fakeConfirmRedis) DelCtx(_ context.Context, keys ...string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	removed := 0
	for _, key := range keys {
		if _, ok := f.store[key]; ok {
			delete(f.store, key)
			removed++
		}
	}
	return removed, nil
}

func (f *fakeConfirmRedis) ExistsCtx(_ context.Context, key string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.store[key]
	return ok, nil
}

func TestConfirmBrokerApproveFlow(t *testing.T) {
	broker := NewRedisConfirmBroker(newFakeConfirmRedis(), "assistant:v2")
	ctx := context.Background()
	if err := broker.Open(ctx, 7, "req-1", "call-1", ToolDeletePost, "删除帖子 #9", time.Second); err != nil {
		t.Fatal(err)
	}
	if err := broker.Resolve(ctx, 7, "req-1", "call-1", true); err != nil {
		t.Fatal(err)
	}
	approved, err := broker.Wait(ctx, 7, "req-1", "call-1", time.Second)
	if err != nil || !approved {
		t.Fatalf("expected approved=true err=%v", err)
	}
	// 凭据一次性（AGNT-022）：裁决读取后 pending/decision 均被清理，重放拒绝。
	if err := broker.Resolve(ctx, 7, "req-1", "call-1", true); err != ErrConfirmExpired {
		t.Fatalf("replayed resolve must be rejected, got %v", err)
	}
}

func TestConfirmBrokerRejectsForeignUserAndTimeout(t *testing.T) {
	redis := newFakeConfirmRedis()
	broker := NewRedisConfirmBroker(redis, "assistant:v2")
	ctx := context.Background()
	if err := broker.Open(ctx, 7, "req-1", "call-1", ToolDeletePost, "s", time.Minute); err != nil {
		t.Fatal(err)
	}
	// 归属校验：其他用户不能裁决他人的确认（AGNT-022）。
	if err := broker.Resolve(ctx, 8, "req-1", "call-1", true); err != ErrConfirmExpired {
		t.Fatalf("foreign user must be rejected, got %v", err)
	}

	// 超时路径：pending 失效后 Wait 返回 ErrConfirmExpired。
	short := &redisConfirmBroker{redis: redis, prefix: "assistant:v2", pollEvery: 5 * time.Millisecond}
	if _, err := short.Wait(ctx, 7, "req-missing", "call-x", 30*time.Millisecond); err != ErrConfirmExpired {
		t.Fatalf("expired wait must fail with ErrConfirmExpired, got %v", err)
	}
	if !strings.Contains(ErrConfirmExpired.Error(), "expired") {
		t.Fatalf("unexpected error text %q", ErrConfirmExpired.Error())
	}
}
