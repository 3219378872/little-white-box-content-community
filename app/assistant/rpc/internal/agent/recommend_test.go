package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"esx/app/assistant/rpc/internal/memory"
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

func (f *fakeRecommendService) GetRecommendPosts(ctx context.Context, req *recommendservice.GetRecommendPostsReq, _ ...grpc.CallOption) (*recommendservice.GetRecommendPostsResp, error) {
	if f.feed == nil {
		return &recommendservice.GetRecommendPostsResp{}, nil
	}
	return f.feed(ctx, req)
}
