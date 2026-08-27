package agent

import (
	"context"
	"strings"
	"testing"

	"esx/app/content/rpc/contentservice"
	"esx/app/recommend/rpc/recommendservice"
	"esx/app/search/rpc/searchservice"
	"esx/pkg/errx"

	"google.golang.org/grpc"
)

func TestRRFMergeRanksSharedHitsFirst(t *testing.T) {
	t.Parallel()
	merged := rrfMerge([][]int64{
		{11, 12, 13},
		{13, 11, 14},
	}, 60)
	if len(merged) != 4 || merged[0] != 11 && merged[0] != 13 {
		t.Fatalf("unexpected merge order: %v", merged)
	}
	if merged[0] != 11 {
		t.Fatalf("id 11 appears in both lists at high ranks, got first=%d", merged[0])
	}
}

func TestSearchPostsMergesSimilarAndAttachesComments(t *testing.T) {
	search := &fakeSearchService{
		posts: func(_ context.Context, req *searchservice.SearchPostsReq) (*searchservice.SearchPostsResp, error) {
			if req.Keyword != "黑神话" {
				t.Fatalf("keyword=%q", req.Keyword)
			}
			if req.SortBy != 2 {
				t.Fatalf("community opinion should sort by newest, got %d", req.SortBy)
			}
			return &searchservice.SearchPostsResp{Posts: []*searchservice.PostSearchResult{
				{Id: 11, Title: "性能讨论", ContentHighlight: "帧数掉了"},
				{Id: 12, Title: "剧情", ContentHighlight: "一周目"},
			}}, nil
		},
	}
	recommend := &fakeRecommendService{
		similar: func(_ context.Context, req *recommendservice.GetSimilarPostsReq) (*recommendservice.GetSimilarPostsResp, error) {
			if req.PostId != 11 {
				t.Fatalf("seed=%d", req.PostId)
			}
			return &recommendservice.GetSimilarPostsResp{Posts: []*recommendservice.RecommendPost{
				{PostId: 13, Score: 0.9},
				{PostId: 11, Score: 0.8},
			}}, nil
		},
	}
	content := &fakeContentService{
		postsByIDs: func(_ context.Context, req *contentservice.GetPostsByIdsReq) (*contentservice.GetPostsByIdsResp, error) {
			posts := map[int64]*contentservice.PostInfo{
				11: {Id: 11, Title: "性能讨论", Content: "帧数掉了", Status: 1, Revision: 3},
				12: {Id: 12, Title: "剧情", Content: "一周目", Status: 1, Revision: 1},
				13: {Id: 13, Title: "相似帖", Content: "优化补丁", Status: 1, Revision: 2},
			}
			out := make([]*contentservice.PostInfo, 0, len(req.PostIds))
			for _, id := range req.PostIds {
				if post := posts[id]; post != nil {
					out = append(out, post)
				}
			}
			return &contentservice.GetPostsByIdsResp{Posts: out}, nil
		},
		commentList: func(_ context.Context, req *contentservice.GetCommentListReq) (*contentservice.GetCommentListResp, error) {
			if req.PostId != 11 {
				return &contentservice.GetCommentListResp{}, nil
			}
			return &contentservice.GetCommentListResp{Comments: []*contentservice.CommentInfo{
				{Id: 21, PostId: 11, Content: "1% Low 不行", Status: 1},
				{Id: 22, PostId: 11, Content: "已删", Status: 0},
			}}, nil
		},
	}
	registry, err := NewToolRegistry(Clients{
		Search: search, Content: content, Recommend: recommend,
	}, []string{ToolSearchPosts})
	if err != nil {
		t.Fatal(err)
	}
	session := &Session{Plan: QueryPlan{Intent: IntentCommunityOpinion, TimeRange: "recent"}, ContextPostID: 11}
	text, sources, err := registry.Call(context.Background(), session, ToolSearchPosts, "c1", `{"keyword":"黑神话"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "[post:11]") || !strings.Contains(text, "[post:13]") {
		t.Fatalf("expected merged posts, got %s", text)
	}
	if !strings.Contains(text, "[comment:21]") || strings.Contains(text, "[comment:22]") {
		t.Fatalf("expected only active comments, got %s", text)
	}
	var sawComment bool
	for _, source := range sources {
		if source.Type == "comment" && source.ID == "21" && source.Revision == 3 {
			sawComment = true
		}
	}
	if !sawComment {
		t.Fatalf("comment evidence missing parent revision: %+v", sources)
	}
}

func TestGetPostCommentsRejectsUnpublishedParent(t *testing.T) {
	content := &fakeContentService{
		postsByIDs: func(context.Context, *contentservice.GetPostsByIdsReq) (*contentservice.GetPostsByIdsResp, error) {
			return &contentservice.GetPostsByIdsResp{Posts: []*contentservice.PostInfo{
				{Id: 9, Title: "draft", Status: 0, Revision: 1},
			}}, nil
		},
	}
	registry, err := NewToolRegistry(Clients{Content: content}, []string{ToolGetPostComments})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = registry.Call(context.Background(), &Session{}, ToolGetPostComments, "c1", `{"post_id":9}`)
	if !errx.Is(err, errx.ContentNotFound) {
		t.Fatalf("got %v", err)
	}
}

func TestSearchUsersAreNotEvidence(t *testing.T) {
	search := &fakeSearchService{
		users: func(context.Context, *searchservice.SearchUsersReq) (*searchservice.SearchUsersResp, error) {
			return &searchservice.SearchUsersResp{Users: []*searchservice.UserSearchResult{
				{Id: 2, Username: "alice", Nickname: "Alice", FollowerCount: 3},
			}}, nil
		},
	}
	registry, err := NewToolRegistry(Clients{Search: search}, []string{ToolSearchUsers})
	if err != nil {
		t.Fatal(err)
	}
	text, sources, err := registry.Call(context.Background(), &Session{}, ToolSearchUsers, "c1", `{"keyword":"alice"}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 0 {
		t.Fatalf("users must not become evidence: %+v", sources)
	}
	if !strings.Contains(text, "alice") {
		t.Fatalf("output=%q", text)
	}
}

func TestRestrictHidesNewSearchToolsOnV1Consent(t *testing.T) {
	registry, err := NewToolRegistry(Clients{}, []string{ToolSearchPosts, ToolSearchUsers, ToolGetPost, ToolCreatePost})
	if err != nil {
		t.Fatal(err)
	}
	filtered := RestrictToolsForConsent(registry, 1)
	if filtered.Has(ToolSearchUsers) || filtered.Has(ToolGetPost) {
		t.Fatal("v1 consent must not expose new search tools")
	}
	if !filtered.Has(ToolSearchPosts) || !filtered.Has(ToolCreatePost) {
		t.Fatal("v1 tools must remain")
	}
}

type fakeSearchService struct {
	searchservice.SearchService
	posts func(context.Context, *searchservice.SearchPostsReq) (*searchservice.SearchPostsResp, error)
	users func(context.Context, *searchservice.SearchUsersReq) (*searchservice.SearchUsersResp, error)
	tags  func(context.Context, *searchservice.SearchTagsReq) (*searchservice.SearchTagsResp, error)
}

func (f *fakeSearchService) SearchPosts(ctx context.Context, req *searchservice.SearchPostsReq, _ ...grpc.CallOption) (*searchservice.SearchPostsResp, error) {
	return f.posts(ctx, req)
}
func (f *fakeSearchService) SearchUsers(ctx context.Context, req *searchservice.SearchUsersReq, _ ...grpc.CallOption) (*searchservice.SearchUsersResp, error) {
	return f.users(ctx, req)
}
func (f *fakeSearchService) SearchTags(ctx context.Context, req *searchservice.SearchTagsReq, _ ...grpc.CallOption) (*searchservice.SearchTagsResp, error) {
	return f.tags(ctx, req)
}

type fakeRecommendService struct {
	recommendservice.RecommendService
	similar func(context.Context, *recommendservice.GetSimilarPostsReq) (*recommendservice.GetSimilarPostsResp, error)
	feed    func(context.Context, *recommendservice.GetRecommendPostsReq) (*recommendservice.GetRecommendPostsResp, error)
}

func (f *fakeRecommendService) GetSimilarPosts(ctx context.Context, req *recommendservice.GetSimilarPostsReq, _ ...grpc.CallOption) (*recommendservice.GetSimilarPostsResp, error) {
	return f.similar(ctx, req)
}
