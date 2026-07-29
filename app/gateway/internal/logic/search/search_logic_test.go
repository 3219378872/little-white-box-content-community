package search

import (
	"context"
	"testing"

	"errx"
	"esx/app/search/rpc/searchservice"
	"gateway/internal/svc"
	"gateway/internal/types"

	"google.golang.org/grpc"
)

type fakeSearchService struct {
	searchservice.SearchService
	searchFn      func(context.Context, *searchservice.SearchReq, ...grpc.CallOption) (*searchservice.SearchResp, error)
	searchUsersFn func(context.Context, *searchservice.SearchUsersReq, ...grpc.CallOption) (*searchservice.SearchUsersResp, error)
	searchTagsFn  func(context.Context, *searchservice.SearchTagsReq, ...grpc.CallOption) (*searchservice.SearchTagsResp, error)
}

func (f *fakeSearchService) Search(ctx context.Context, in *searchservice.SearchReq, opts ...grpc.CallOption) (*searchservice.SearchResp, error) {
	return f.searchFn(ctx, in, opts...)
}

func (f *fakeSearchService) SearchUsers(ctx context.Context, in *searchservice.SearchUsersReq, opts ...grpc.CallOption) (*searchservice.SearchUsersResp, error) {
	return f.searchUsersFn(ctx, in, opts...)
}

func (f *fakeSearchService) SearchTags(ctx context.Context, in *searchservice.SearchTagsReq, opts ...grpc.CallOption) (*searchservice.SearchTagsResp, error) {
	return f.searchTagsFn(ctx, in, opts...)
}

func TestSearch_MapsAllResultKinds(t *testing.T) {
	type requestContextKey struct{}
	ctxKey := requestContextKey{}
	ctx := context.WithValue(context.Background(), ctxKey, "preserved")
	svcCtx := &svc.ServiceContext{SearchService: &fakeSearchService{
		searchFn: func(gotCtx context.Context, in *searchservice.SearchReq, _ ...grpc.CallOption) (*searchservice.SearchResp, error) {
			if gotCtx.Value(ctxKey) != "preserved" {
				t.Fatal("request context was not propagated")
			}
			if in.Keyword != "golang" || in.Page != 2 || in.PageSize != 10 {
				t.Fatalf("unexpected rpc request: %+v", in)
			}
			return &searchservice.SearchResp{
				Posts: []*searchservice.PostSearchResult{{
					Id: 1, Title: "Go", ContentHighlight: "<em>Go</em>", AuthorName: "alice",
					LikeCount: 2, CommentCount: 3, CreatedAt: 4,
				}},
				Users: []*searchservice.UserSearchResult{{
					Id: 5, Username: "bob", Nickname: "B", AvatarUrl: "avatar", Bio: "bio", FollowerCount: 6,
				}},
				Tags: []*searchservice.TagSearchResult{{Name: "go", PostCount: 7}},
			}, nil
		},
	}}

	resp, err := NewSearchLogic(ctx, svcCtx).Search(&types.SearchReq{Keyword: "golang", Page: 2, PageSize: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Posts) != 1 || resp.Posts[0].ContentHighlight != "<em>Go</em>" || resp.Posts[0].CommentCount != 3 {
		t.Fatalf("posts were not mapped: %+v", resp.Posts)
	}
	if len(resp.Users) != 1 || resp.Users[0].Id != 5 || resp.Users[0].FollowerCount != 6 {
		t.Fatalf("users were not mapped: %+v", resp.Users)
	}
	if len(resp.Tags) != 1 || resp.Tags[0].Name != "go" || resp.Tags[0].PostCount != 7 {
		t.Fatalf("tags were not mapped: %+v", resp.Tags)
	}
}

func TestSearchUsers_MapsResponse(t *testing.T) {
	svcCtx := &svc.ServiceContext{SearchService: &fakeSearchService{
		searchUsersFn: func(_ context.Context, in *searchservice.SearchUsersReq, _ ...grpc.CallOption) (*searchservice.SearchUsersResp, error) {
			if in.Keyword != "alice" || in.Page != 1 || in.PageSize != 20 {
				t.Fatalf("unexpected rpc request: %+v", in)
			}
			return &searchservice.SearchUsersResp{
				Users: []*searchservice.UserSearchResult{{Id: 9, Username: "alice", Nickname: "Alice", FollowerCount: 10}},
				Total: 1,
			}, nil
		},
	}}

	resp, err := NewSearchUsersLogic(context.Background(), svcCtx).SearchUsers(&types.SearchUsersReq{Keyword: "alice", Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Total != 1 || len(resp.Users) != 1 || resp.Users[0].Username != "alice" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestSearchTags_MapsResponse(t *testing.T) {
	svcCtx := &svc.ServiceContext{SearchService: &fakeSearchService{
		searchTagsFn: func(_ context.Context, in *searchservice.SearchTagsReq, _ ...grpc.CallOption) (*searchservice.SearchTagsResp, error) {
			if in.Keyword != "go" || in.Limit != 5 {
				t.Fatalf("unexpected rpc request: %+v", in)
			}
			return &searchservice.SearchTagsResp{Tags: []*searchservice.TagSearchResult{{Name: "golang", PostCount: 12}}}, nil
		},
	}}

	resp, err := NewSearchTagsLogic(context.Background(), svcCtx).SearchTags(&types.SearchTagsReq{Keyword: "go", Limit: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Tags) != 1 || resp.Tags[0].Name != "golang" || resp.Tags[0].PostCount != 12 {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestSearchLogics_RejectEmptyKeywordWithoutRPC(t *testing.T) {
	called := false
	fake := &fakeSearchService{
		searchFn: func(context.Context, *searchservice.SearchReq, ...grpc.CallOption) (*searchservice.SearchResp, error) {
			called = true
			return &searchservice.SearchResp{}, nil
		},
		searchUsersFn: func(context.Context, *searchservice.SearchUsersReq, ...grpc.CallOption) (*searchservice.SearchUsersResp, error) {
			called = true
			return &searchservice.SearchUsersResp{}, nil
		},
		searchTagsFn: func(context.Context, *searchservice.SearchTagsReq, ...grpc.CallOption) (*searchservice.SearchTagsResp, error) {
			called = true
			return &searchservice.SearchTagsResp{}, nil
		},
	}
	svcCtx := &svc.ServiceContext{SearchService: fake}

	tests := []struct {
		name string
		call func() error
	}{
		{name: "combined", call: func() error {
			_, err := NewSearchLogic(context.Background(), svcCtx).Search(&types.SearchReq{Keyword: "   ", Page: 1, PageSize: 20})
			return err
		}},
		{name: "users", call: func() error {
			_, err := NewSearchUsersLogic(context.Background(), svcCtx).SearchUsers(&types.SearchUsersReq{Keyword: "", Page: 1, PageSize: 20})
			return err
		}},
		{name: "tags", call: func() error {
			_, err := NewSearchTagsLogic(context.Background(), svcCtx).SearchTags(&types.SearchTagsReq{Keyword: "\t", Limit: 20})
			return err
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); !errx.Is(err, errx.SearchEmpty) {
				t.Fatalf("expected SearchEmpty, got %v", err)
			}
		})
	}
	if called {
		t.Fatal("search rpc must not be called for an empty keyword")
	}
}

func TestSearchLogics_RPCError(t *testing.T) {
	fake := &fakeSearchService{
		searchFn: func(context.Context, *searchservice.SearchReq, ...grpc.CallOption) (*searchservice.SearchResp, error) {
			return nil, context.DeadlineExceeded
		},
		searchUsersFn: func(context.Context, *searchservice.SearchUsersReq, ...grpc.CallOption) (*searchservice.SearchUsersResp, error) {
			return nil, context.DeadlineExceeded
		},
		searchTagsFn: func(context.Context, *searchservice.SearchTagsReq, ...grpc.CallOption) (*searchservice.SearchTagsResp, error) {
			return nil, context.DeadlineExceeded
		},
	}
	svcCtx := &svc.ServiceContext{SearchService: fake}

	tests := []struct {
		name string
		call func() error
	}{
		{name: "combined", call: func() error {
			_, err := NewSearchLogic(context.Background(), svcCtx).Search(&types.SearchReq{Keyword: "go", Page: 1, PageSize: 20})
			return err
		}},
		{name: "users", call: func() error {
			_, err := NewSearchUsersLogic(context.Background(), svcCtx).SearchUsers(&types.SearchUsersReq{Keyword: "go", Page: 1, PageSize: 20})
			return err
		}},
		{name: "tags", call: func() error {
			_, err := NewSearchTagsLogic(context.Background(), svcCtx).SearchTags(&types.SearchTagsReq{Keyword: "go", Limit: 20})
			return err
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); err == nil {
				t.Fatal("expected rpc error")
			}
		})
	}
}
