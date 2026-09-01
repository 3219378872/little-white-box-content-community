package index

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"esx/app/assistant/internal/store"
	"esx/app/assistant/internal/tool"
	"esx/pkg/errx"

	"github.com/elastic/go-elasticsearch/v8"
)

func TestSearchHistoryImplementsAllShapesAndExcludesLiveContext(t *testing.T) {
	ctx := context.Background()
	mem := store.NewMemoryStore()
	now := time.Now().UnixMilli()
	firstSession, err := mem.CreateSession(ctx, store.Session{UserID: 7, Status: store.SessionOpen, CreatedAtMs: now})
	if err != nil {
		t.Fatal(err)
	}
	firstMessages := make([]store.Message, 0, 13)
	for index := 1; index <= 13; index++ {
		message, insertErr := mem.InsertMessage(ctx, store.Message{
			UserID: 7, SessionID: firstSession.ID, Role: store.RoleUser, Kind: store.KindMessage,
			Content: fmt.Sprintf("session-one-%02d", index), Visible: true, Compacted: index < 13, CreatedAtMs: now + int64(index),
		})
		if insertErr != nil {
			t.Fatal(insertErr)
		}
		firstMessages = append(firstMessages, message)
	}
	secondSession, err := mem.CreateSession(ctx, store.Session{UserID: 7, Status: store.SessionClosed, CreatedAtMs: now + 100})
	if err != nil {
		t.Fatal(err)
	}
	secondFirst, _ := mem.InsertMessage(ctx, store.Message{
		UserID: 7, SessionID: secondSession.ID, Role: store.RoleUser, Kind: store.KindMessage,
		Content: "session-two-first", Visible: true, Compacted: true, CreatedAtMs: now + 101,
	})
	secondLast, _ := mem.InsertMessage(ctx, store.Message{
		UserID: 7, SessionID: secondSession.ID, Role: store.RoleAssistant, Kind: store.KindMessage,
		Content: "session-two-last", Visible: true, Compacted: true, CreatedAtMs: now + 102,
	})
	otherUser, _ := mem.InsertMessage(ctx, store.Message{
		UserID: 8, SessionID: 99, Role: store.RoleUser, Kind: store.KindMessage,
		Content: "other-user-secret", Visible: true, Compacted: true, CreatedAtMs: now + 103,
	})

	anchor := firstMessages[6]
	live := firstMessages[12]
	var searchBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		if !strings.Contains(r.URL.Path, "_search") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		var body any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(body)
		searchBody = string(raw)
		_ = json.NewEncoder(w).Encode(map[string]any{"hits": map[string]any{"hits": []map[string]any{
			{"_source": Document{UserID: 7, SessionID: firstSession.ID, MessageID: live.ID, Content: live.Content}},
			{"_source": Document{UserID: 7, SessionID: firstSession.ID, MessageID: anchor.ID, Content: anchor.Content}},
			{"_source": Document{UserID: 8, SessionID: 99, MessageID: otherUser.ID, Content: otherUser.Content}},
			{"_source": Document{UserID: 7, SessionID: secondSession.ID, MessageID: secondLast.ID, Content: secondLast.Content}},
		}}})
	}))
	defer server.Close()
	es, err := elasticsearch.NewClient(elasticsearch.Config{Addresses: []string{server.URL}})
	if err != nil {
		t.Fatal(err)
	}
	client := &Client{es: es, store: mem}
	session := &tool.Session{UserID: 7, SessionID: firstSession.ID, LiveMessageIDs: []int64{live.ID}}

	keywords, err := client.Search(ctx, session, tool.HistoryArgs{Shape: "keywords", Query: "session", Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(searchBody, "must_not") || !strings.Contains(searchBody, fmt.Sprintf("%d", live.ID)) {
		t.Fatalf("query did not exclude live message: %s", searchBody)
	}
	if strings.Contains(keywords, live.Content) || strings.Contains(keywords, otherUser.Content) ||
		!strings.Contains(keywords, anchor.Content) || !strings.Contains(keywords, secondLast.Content) {
		t.Fatalf("keywords=%s", keywords)
	}

	around, err := client.Search(ctx, session, tool.HistoryArgs{Shape: "around", MessageID: anchor.ID})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(around, "\n- [") != 11 || strings.Contains(around, fmt.Sprintf("\n- [%d]", firstMessages[0].ID)) || strings.Contains(around, live.Content) {
		t.Fatalf("around=%s", around)
	}
	if hidden, err := client.Search(ctx, session, tool.HistoryArgs{Shape: "around", MessageID: live.ID}); err != nil || hidden != noHistoryResult() {
		t.Fatalf("live around=%q err=%v", hidden, err)
	}

	sessionResult, err := client.Search(ctx, session, tool.HistoryArgs{Shape: "session", SessionID: firstSession.ID})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sessionResult, firstMessages[0].Content) || !strings.Contains(sessionResult, firstMessages[11].Content) || strings.Contains(sessionResult, live.Content) {
		t.Fatalf("session=%s", sessionResult)
	}

	recent, err := client.Search(ctx, session, tool.HistoryArgs{Shape: "recent", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	secondPosition := strings.Index(recent, secondFirst.Content)
	firstPosition := strings.Index(recent, firstMessages[0].Content)
	if secondPosition < 0 || firstPosition < 0 || secondPosition > firstPosition {
		t.Fatalf("recent=%s", recent)
	}

	for _, args := range []tool.HistoryArgs{
		{}, {Shape: "keywords"}, {Shape: "around"}, {Shape: "session"}, {Shape: "unknown"},
	} {
		if _, err := client.Search(ctx, session, args); !errx.Is(err, errx.ParamError) {
			t.Fatalf("args=%+v err=%v", args, err)
		}
	}
}
