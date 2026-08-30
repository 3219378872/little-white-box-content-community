package tool

import (
	"context"
	"testing"

	"esx/app/assistant/internal/memory"
)

func TestMemoryToolReturnsStructuredChangeIDs(t *testing.T) {
	memoryStore := memory.NewMapStore()
	registry, err := NewRegistry(Clients{Memory: memoryStore}, []string{AddMemory})
	if err != nil {
		t.Fatal(err)
	}
	session := &Session{UserID: 7, RequestID: "review-1"}
	if _, _, err := registry.Call(context.Background(), session, AddMemory, "call-1", `{"target":"memory","content":"偏好短答案"}`); err != nil {
		t.Fatal(err)
	}
	if len(session.ChangeIDs) != 1 || session.ChangeIDs[0] <= 0 {
		t.Fatalf("change ids=%v", session.ChangeIDs)
	}
}
