package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"esx/app/assistant/rpc/internal/memory"
	"esx/app/assistant/rpc/xiaobaihe/assistant/pb"
	"esx/app/content/rpc/contentservice"
	"esx/app/recommend/rpc/recommendservice"

	"google.golang.org/grpc"
)

func TestRecommendPostsHardFiltersExcludedIDs(t *testing.T) {
	store := memory.NewMapStore()
	if err := store.Apply(context.Background(), 2, memory.Candidate{
		Layer: memory.LayerProfile, Dimension: "post", Value: "11", Score: -0.8,
		Source: memory.SourceExplicit, Confidence: 1,
	}, time.Now()); err != nil {
		t.Fatal(err)
	}
	recommend := &fakeRecommendService{
		feed: func(context.Context, *recommendservice.GetRecommendPostsReq) (*recommendservice.GetRecommendPostsResp, error) {
			return &recommendservice.GetRecommendPostsResp{Posts: []*recommendservice.RecommendPost{
				{PostId: 11, Reason: "skip"},
				{PostId: 12, Reason: "match"},
			}}, nil
		},
	}
	content := &fakeContentService{
		postsByIDs: func(_ context.Context, req *contentservice.GetPostsByIdsReq) (*contentservice.GetPostsByIdsResp, error) {
			out := []*contentservice.PostInfo{}
			for _, id := range req.PostIds {
				out = append(out, &contentservice.PostInfo{Id: id, Title: "t", Content: "c", Status: 1, Revision: 1})
			}
			return &contentservice.GetPostsByIdsResp{Posts: out}, nil
		},
	}
	registry, err := NewToolRegistry(Clients{Recommend: recommend, Content: content, Memory: store}, []string{ToolRecommendPosts})
	if err != nil {
		t.Fatal(err)
	}
	text, sources, err := registry.Call(context.Background(), &Session{UserID: 2}, ToolRecommendPosts, "c1", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(text, "[post:11]") || !strings.Contains(text, "[post:12]") {
		t.Fatalf("%s", text)
	}
	if len(sources) != 1 || sources[0].ID != "12" {
		t.Fatalf("%+v", sources)
	}
}

func TestRecommendPostsExcludesOpenTaskIDs(t *testing.T) {
	store := memory.NewMapStore()
	if err := store.Apply(context.Background(), 2, memory.Candidate{
		Layer: memory.LayerTask, Dimension: "task", Value: "11,13", Score: 1,
		Source: memory.SourceConversation, Confidence: 1,
	}, time.Now()); err != nil {
		t.Fatal(err)
	}
	recommend := &fakeRecommendService{
		feed: func(context.Context, *recommendservice.GetRecommendPostsReq) (*recommendservice.GetRecommendPostsResp, error) {
			return &recommendservice.GetRecommendPostsResp{Posts: []*recommendservice.RecommendPost{
				{PostId: 11, Reason: "skip"},
				{PostId: 12, Reason: "keep"},
			}}, nil
		},
	}
	content := &fakeContentService{
		postsByIDs: func(_ context.Context, req *contentservice.GetPostsByIdsReq) (*contentservice.GetPostsByIdsResp, error) {
			out := []*contentservice.PostInfo{}
			for _, id := range req.PostIds {
				out = append(out, &contentservice.PostInfo{Id: id, Title: "t", Content: "c", Status: 1, Revision: 1})
			}
			return &contentservice.GetPostsByIdsResp{Posts: out}, nil
		},
	}
	registry, err := NewToolRegistry(Clients{Recommend: recommend, Content: content, Memory: store}, []string{ToolRecommendPosts})
	if err != nil {
		t.Fatal(err)
	}
	text, sources, err := registry.Call(context.Background(), &Session{UserID: 2, Plan: QueryPlan{Intent: IntentContinueTask}}, ToolRecommendPosts, "c1", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(text, "[post:11]") || !strings.Contains(text, "[post:12]") {
		t.Fatalf("%s", text)
	}
	if len(sources) != 1 || sources[0].ID != "12" {
		t.Fatalf("%+v", sources)
	}
}

func TestRecommendPostsEmitsVerifiedCard(t *testing.T) {
	recommend := &fakeRecommendService{
		feed: func(context.Context, *recommendservice.GetRecommendPostsReq) (*recommendservice.GetRecommendPostsResp, error) {
			return &recommendservice.GetRecommendPostsResp{Posts: []*recommendservice.RecommendPost{
				{PostId: 12, Reason: "keep"},
			}}, nil
		},
	}
	content := &fakeContentService{
		postsByIDs: func(_ context.Context, req *contentservice.GetPostsByIdsReq) (*contentservice.GetPostsByIdsResp, error) {
			return &contentservice.GetPostsByIdsResp{Posts: []*contentservice.PostInfo{
				{Id: 12, Title: "已验证", Content: "body", Status: 1, Revision: 2},
			}}, nil
		},
	}
	registry, err := NewToolRegistry(Clients{Recommend: recommend, Content: content}, []string{ToolRecommendPosts})
	if err != nil {
		t.Fatal(err)
	}
	var events []*pb.ChatEvent
	session := &Session{UserID: 2, Emit: func(event *pb.ChatEvent) error {
		events = append(events, event)
		return nil
	}}
	if _, _, err := registry.Call(context.Background(), session, ToolRecommendPosts, "c1", `{}`); err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Type != pb.ChatEventType_CHAT_EVENT_TYPE_CARD || events[0].Card == nil || events[0].Card.CardType != "recommend" {
		t.Fatalf("%+v", events)
	}
	var payload []recommendCardItem
	if err := json.Unmarshal([]byte(events[0].Card.PayloadJson), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload) != 1 || payload[0].ID != 12 || payload[0].Title != "已验证" {
		t.Fatalf("%+v", payload)
	}

	session.Emit = nil
	if _, _, err := registry.Call(context.Background(), session, ToolRecommendPosts, "c2", `{}`); err != nil {
		t.Fatal(err)
	}
}

func (f *fakeRecommendService) GetRecommendPosts(ctx context.Context, req *recommendservice.GetRecommendPostsReq, _ ...grpc.CallOption) (*recommendservice.GetRecommendPostsResp, error) {
	if f.feed == nil {
		return &recommendservice.GetRecommendPostsResp{}, nil
	}
	return f.feed(ctx, req)
}
