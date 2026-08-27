package agent

import (
	"context"
	"strings"
	"testing"

	"esx/app/assistant/rpc/internal/memory"
)

func TestMemoryToolsCRUD(t *testing.T) {
	store := memory.NewMapStore()
	registry, err := NewToolRegistry(Clients{Memory: store}, []string{ToolAddMemory, ToolGetMemory, ToolDeleteMemory})
	if err != nil {
		t.Fatal(err)
	}
	session := &Session{UserID: 2}
	if _, _, err := registry.Call(context.Background(), session, ToolAddMemory, "c1", `{"value":"剧情","score":0.8}`); err != nil {
		t.Fatal(err)
	}
	text, _, err := registry.Call(context.Background(), session, ToolGetMemory, "c2", `{"layer":"profile"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "剧情") {
		t.Fatalf("%s", text)
	}
}
