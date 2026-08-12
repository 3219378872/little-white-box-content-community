package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

type fakeRedis struct {
	results []any
	err     error
	keys    [][]string
	args    [][]any
}

func (f *fakeRedis) EvalCtx(_ context.Context, _ string, keys []string, args ...any) (any, error) {
	f.keys = append(f.keys, append([]string(nil), keys...))
	f.args = append(f.args, append([]any(nil), args...))
	if f.err != nil {
		return nil, f.err
	}
	result := any(int64(1))
	if len(f.results) > 0 {
		result = f.results[0]
		f.results = f.results[1:]
	}
	return result, nil
}

func TestRedisStateAppendBindsOwnerAndPersistsMessage(t *testing.T) {
	redis := &fakeRedis{}
	state, err := NewRedisState(redis, "assistant:v2", 3600, 10, 60, 2)
	if err != nil {
		t.Fatal(err)
	}
	err = state.Append(context.Background(), 42, "conversation-1", Message{
		Role: "user", Content: "hello", RequestID: "request-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(redis.keys) != 1 || redis.keys[0][0] != "assistant:v2:conversation:conversation-1:owner" ||
		redis.keys[0][1] != "assistant:v2:conversation:conversation-1:messages" {
		t.Fatalf("unexpected keys: %#v", redis.keys)
	}
	payload, ok := redis.args[0][2].(string)
	if !ok || payload == "" {
		t.Fatalf("message payload was not persisted: %#v", redis.args)
	}
}

func TestRedisStateRejectsConversationOwnedByAnotherUser(t *testing.T) {
	state, err := NewRedisState(&fakeRedis{results: []any{int64(-1)}}, "assistant:v2", 3600, 10, 60, 2)
	if err != nil {
		t.Fatal(err)
	}
	err = state.Append(context.Background(), 42, "conversation-1", Message{
		Role: "user", Content: "hello", RequestID: "request-1",
	})
	if !errors.Is(err, ErrConversationOwnedByAnother) {
		t.Fatalf("error=%v want ownership error", err)
	}
}

func TestRedisStateMessagesReturnsPersistedHistory(t *testing.T) {
	message := Message{Role: "assistant", Content: "answer", RequestID: "req-1", Sources: []Reference{
		{Type: "post", ID: "9", Revision: 3},
	}}
	payload, err := json.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	redis := &fakeRedis{results: []any{[]any{[]byte(payload)}}}
	state, err := NewRedisState(redis, "assistant:v2", 3600, 10, 60, 2)
	if err != nil {
		t.Fatal(err)
	}
	messages, err := state.Messages(context.Background(), 42, "conversation-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || messages[0].RequestID != "req-1" ||
		len(messages[0].Sources) != 1 || messages[0].Sources[0].Revision != 3 {
		t.Fatalf("unexpected messages: %#v", messages)
	}
}

func TestRedisStateMessagesRejectsForeignOwner(t *testing.T) {
	state, err := NewRedisState(&fakeRedis{results: []any{int64(-1)}}, "assistant:v2", 3600, 10, 60, 2)
	if err != nil {
		t.Fatal(err)
	}
	_, err = state.Messages(context.Background(), 42, "conversation-1")
	if !errors.Is(err, ErrConversationOwnedByAnother) {
		t.Fatalf("error=%v want ownership error", err)
	}
}

func TestRedisStateQuotaUsesAtomicCount(t *testing.T) {
	redis := &fakeRedis{results: []any{int64(1), int64(2), int64(3)}}
	state, err := NewRedisState(redis, "assistant:v2", 3600, 10, 60, 2)
	if err != nil {
		t.Fatal(err)
	}
	for index, want := range []bool{true, true, false} {
		allowed, err := state.Allow(context.Background(), 42)
		if err != nil {
			t.Fatal(err)
		}
		if allowed != want {
			t.Fatalf("attempt %d allowed=%v want=%v", index+1, allowed, want)
		}
	}
}
