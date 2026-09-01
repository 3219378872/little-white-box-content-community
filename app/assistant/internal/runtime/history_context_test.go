package runtime

import (
	"context"
	"reflect"
	"testing"

	"esx/app/assistant/internal/store"
)

func TestToolSessionCarriesOnlyCurrentLiveMessageIDs(t *testing.T) {
	ctx := context.Background()
	mem := store.NewMemoryStore()
	session, err := mem.CreateSession(ctx, store.Session{UserID: 9, Status: store.SessionOpen})
	if err != nil {
		t.Fatal(err)
	}
	compacted, _ := mem.InsertMessage(ctx, store.Message{UserID: 9, SessionID: session.ID, Role: store.RoleUser, Kind: store.KindMessage, Visible: true, Compacted: true})
	live, _ := mem.InsertMessage(ctx, store.Message{UserID: 9, SessionID: session.ID, Role: store.RoleAssistant, Kind: store.KindMessage, Visible: true})
	hiddenLive, _ := mem.InsertMessage(ctx, store.Message{UserID: 9, SessionID: session.ID, Role: store.RoleTool, Kind: store.KindTool, Visible: false})
	deleted, _ := mem.InsertMessage(ctx, store.Message{UserID: 9, SessionID: session.ID, Role: store.RoleUser, Kind: store.KindMessage, Visible: true, DeletedAtMs: 1})
	run := store.Run{UserID: 9, SessionID: session.ID, Source: store.SourceUser}
	toolSession := (&Engine{Store: mem}).toolSession(run)
	if err := (&Engine{Store: mem}).populateToolLiveMessageIDs(ctx, run, toolSession); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(toolSession.LiveMessageIDs, []int64{live.ID, hiddenLive.ID}) {
		t.Fatalf("live ids=%v compacted=%d deleted=%d", toolSession.LiveMessageIDs, compacted.ID, deleted.ID)
	}
}
