package prompt

import (
	"bytes"
	"testing"

	"esx/app/assistant/internal/memory"
)

func TestSnapshotReuseIgnoresLaterMemory(t *testing.T) {
	first := BuildSnapshot([]memory.Entry{
		{ID: 2, Target: memory.TargetUser, Content: "用户是设计师"},
		{ID: 1, Target: memory.TargetMemory, Content: "喜欢步行"},
	}, []Turn{{Role: "user", Content: "hi"}}, "")
	raw := EncodeSnapshot(first)
	decoded, ok := DecodeSnapshot(raw)
	if !ok {
		t.Fatal("decode snapshot")
	}
	if decoded.Memory[0].Target != memory.TargetMemory || decoded.Memory[0].ID != 1 {
		t.Fatalf("memory order: %+v", decoded.Memory)
	}
	later := BuildSnapshot([]memory.Entry{
		{ID: 1, Target: memory.TargetMemory, Content: "喜欢骑车"},
	}, nil, "")
	if bytes.Equal(EncodeSnapshot(later), raw) {
		t.Fatal("later memory should not match frozen snapshot")
	}
	msgs := Messages(decoded)
	if len(msgs) == 0 || msgs[0].Role != "system" {
		t.Fatalf("system message missing: %+v", msgs)
	}
	if !bytes.Contains([]byte(msgs[0].Content), []byte("喜欢步行")) {
		t.Fatalf("frozen memory missing: %s", msgs[0].Content)
	}
	if bytes.Contains([]byte(msgs[0].Content), []byte("喜欢骑车")) {
		t.Fatal("later memory leaked into reused snapshot")
	}
}
