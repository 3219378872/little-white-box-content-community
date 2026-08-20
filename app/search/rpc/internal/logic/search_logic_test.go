package logic

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"errx"
	"esx/app/content/rpc/contentservice"
	"esx/app/search/rpc/internal/config"
	"esx/app/search/rpc/internal/store"
	"esx/app/search/rpc/internal/svc"
	"esx/app/search/rpc/xiaobaihe/search/pb"
	"user/userservice"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

type fakeStore struct {
	postsFn func(context.Context, store.PostQuery) (store.PostResult, error)
	tagsFn  func(context.Context, string, int32) ([]store.Tag, error)
	hotFn   func(context.Context, int32) ([]string, error)
}

func (f *fakeStore) Health(context.Context) error { return nil }
func (f *fakeStore) SearchPosts(ctx context.Context, query store.PostQuery) (store.PostResult, error) {
	return f.postsFn(ctx, query)
}
func (f *fakeStore) SearchTags(ctx context.Context, keyword string, limit int32) ([]store.Tag, error) {
	return f.tagsFn(ctx, keyword, limit)
}
func (f *fakeStore) HotSearches(ctx context.Context, limit int32) ([]string, error) {
	return f.hotFn(ctx, limit)
}

type fakeUserService struct {
	batchGetUsersFn func(context.Context, *userservice.BatchGetUsersReq) (*userservice.BatchGetUsersResp, error)
	searchUsersFn   func(context.Context, *userservice.SearchUsersReq) (*userservice.SearchUsersResp, error)
}

func (f *fakeUserService) SearchUsers(
	ctx context.Context,
	in *userservice.SearchUsersReq,
	_ ...grpc.CallOption,
) (*userservice.SearchUsersResp, error) {
	if f.searchUsersFn != nil {
		return f.searchUsersFn(ctx, in)
	}
	return &userservice.SearchUsersResp{}, nil
}

func (f *fakeUserService) BatchGetUsers(
	ctx context.Context,
	in *userservice.BatchGetUsersReq,
	_ ...grpc.CallOption,
) (*userservice.BatchGetUsersResp, error) {
	if f.batchGetUsersFn != nil {
		return f.batchGetUsersFn(ctx, in)
	}
	users := make([]*userservice.UserInfo, 0, len(in.UserIds))
	for _, id := range in.UserIds {
		users = append(users, &userservice.UserInfo{
			Id: id, Username: fmt.Sprintf("user-%d", id), Nickname: fmt.Sprintf("User %d", id),
		})
	}
	return &userservice.BatchGetUsersResp{Users: users}, nil
}

type fakeContentService struct {
	getPostsByIDs func(context.Context, *contentservice.GetPostsByIdsReq) (*contentservice.GetPostsByIdsResp, error)
}

func (f *fakeContentService) GetPostsByIds(
	ctx context.Context,
	in *contentservice.GetPostsByIdsReq,
	_ ...grpc.CallOption,
) (*contentservice.GetPostsByIdsResp, error) {
	if f != nil && f.getPostsByIDs != nil {
		return f.getPostsByIDs(ctx, in)
	}
	posts := make([]*contentservice.PostInfo, 0, len(in.PostIds))
	for _, id := range in.PostIds {
		posts = append(posts, &contentservice.PostInfo{Id: id, Status: 1, Title: "published"})
	}
	return &contentservice.GetPostsByIdsResp{Posts: posts}, nil
}

func serviceContext(searchStore store.Store) *svc.ServiceContext {
	return serviceContextWithUser(searchStore, &fakeUserService{})
}

func serviceContextWithUser(searchStore store.Store, userService svc.UserService) *svc.ServiceContext {
	return serviceContextWithDeps(searchStore, userService, &fakeContentService{})
}

func serviceContextWithDeps(searchStore store.Store, userService svc.UserService, content svc.ContentService) *svc.ServiceContext {
	return &svc.ServiceContext{Config: config.Config{}, Store: searchStore, UserService: userService, ContentService: content}
}

func TestSearchPostsSuccessPropagatesContextAndMapsResult(t *testing.T) {
	type contextKey string
	ctx := context.WithValue(context.Background(), contextKey("request"), "r-1")
	fake := &fakeStore{postsFn: func(got context.Context, query store.PostQuery) (store.PostResult, error) {
		assert.Equal(t, ctx, got)
		assert.Equal(t, "go zero", query.Keyword)
		assert.Equal(t, []string{"go"}, query.Tags)
		return store.PostResult{Posts: []store.Post{{
			ID: 7, AuthorID: 9, Title: "stale title", ContentHighlight: "<em>go</em>", CreatedAt: 123,
		}}, Total: 1}, nil
	}}
	userService := &fakeUserService{batchGetUsersFn: func(got context.Context, in *userservice.BatchGetUsersReq) (*userservice.BatchGetUsersResp, error) {
		assert.Equal(t, ctx, got)
		assert.Equal(t, []int64{9}, in.UserIds)
		return &userservice.BatchGetUsersResp{Users: []*userservice.UserInfo{{
			Id: 9, Nickname: "Go Author", AvatarUrl: "https://avatar/9.png",
		}}}, nil
	}}
	content := &fakeContentService{getPostsByIDs: func(got context.Context, in *contentservice.GetPostsByIdsReq) (*contentservice.GetPostsByIdsResp, error) {
		assert.Equal(t, ctx, got)
		assert.Equal(t, []int64{7}, in.PostIds)
		return &contentservice.GetPostsByIdsResp{Posts: []*contentservice.PostInfo{{
			Id: 7, AuthorId: 9, Status: 1, Title: "Go Zero", Content: "learn go zero", CreatedAt: 123,
		}}}, nil
	}}

	resp, err := NewSearchPostsLogic(ctx, serviceContextWithDeps(fake, userService, content)).SearchPosts(&pb.SearchPostsReq{
		Keyword: " go zero ", Page: 1, PageSize: 20, Tags: []string{" go ", ""},
	})
	require.NoError(t, err)
	require.Len(t, resp.Posts, 1)
	assert.Equal(t, int64(7), resp.Posts[0].Id)
	assert.Equal(t, "Go Zero", resp.Posts[0].Title)
	assert.Equal(t, "<em>go</em>", resp.Posts[0].ContentHighlight)
	assert.Equal(t, int64(9), resp.Posts[0].AuthorId)
	assert.Equal(t, "Go Author", resp.Posts[0].AuthorName)
	assert.Equal(t, "https://avatar/9.png", resp.Posts[0].AuthorAvatar)
	assert.Equal(t, int64(1), resp.Total)
}

func TestSearchPostsKeepsAuthorIDWhenProfileHydrationFails(t *testing.T) {
	fake := &fakeStore{postsFn: func(context.Context, store.PostQuery) (store.PostResult, error) {
		return store.PostResult{Posts: []store.Post{{
			ID: 7, AuthorID: 0, Title: "stale title",
		}}, Total: 1}, nil
	}}
	userService := &fakeUserService{batchGetUsersFn: func(context.Context, *userservice.BatchGetUsersReq) (*userservice.BatchGetUsersResp, error) {
		return nil, errors.New("user rpc unavailable")
	}}
	content := &fakeContentService{getPostsByIDs: func(_ context.Context, in *contentservice.GetPostsByIdsReq) (*contentservice.GetPostsByIdsResp, error) {
		return &contentservice.GetPostsByIdsResp{Posts: []*contentservice.PostInfo{{
			Id: 7, AuthorId: 9, Status: 1, Title: "Go Zero", Content: "learn go zero",
		}}}, nil
	}}

	resp, err := NewSearchPostsLogic(context.Background(), serviceContextWithDeps(fake, userService, content)).SearchPosts(&pb.SearchPostsReq{
		Keyword: "go", Page: 1, PageSize: 20,
	})
	require.NoError(t, err)
	require.Len(t, resp.Posts, 1)
	assert.Equal(t, int64(9), resp.Posts[0].AuthorId)
	assert.Empty(t, resp.Posts[0].AuthorName)
	assert.Empty(t, resp.Posts[0].AuthorAvatar)
}

func TestSearchPostsSupportsHotSort(t *testing.T) {
	fake := &fakeStore{postsFn: func(_ context.Context, query store.PostQuery) (store.PostResult, error) {
		assert.Equal(t, int32(3), query.SortBy)
		return store.PostResult{}, nil
	}}
	resp, err := NewSearchPostsLogic(context.Background(), serviceContext(fake)).SearchPosts(&pb.SearchPostsReq{
		Keyword: "go", Page: 1, PageSize: 20, SortBy: 3,
	})
	require.NoError(t, err)
	assert.Empty(t, resp.Posts)
}

func TestSearchPostsStoreFailureIsUnavailable(t *testing.T) {
	fake := &fakeStore{postsFn: func(context.Context, store.PostQuery) (store.PostResult, error) {
		return store.PostResult{}, errors.New("es unavailable")
	}}
	_, err := NewSearchPostsLogic(context.Background(), serviceContext(fake)).SearchPosts(&pb.SearchPostsReq{
		Keyword: "go", Page: 1, PageSize: 20,
	})
	require.Error(t, err)
	assert.True(t, errx.Is(err, errx.ServiceUnavailable))
}

func TestSearchPostsCanceledIsSearchTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	fake := &fakeStore{postsFn: func(got context.Context, _ store.PostQuery) (store.PostResult, error) {
		assert.Equal(t, ctx, got)
		return store.PostResult{}, got.Err()
	}}
	_, err := NewSearchPostsLogic(ctx, serviceContext(fake)).SearchPosts(&pb.SearchPostsReq{
		Keyword: "go", Page: 1, PageSize: 20,
	})
	require.Error(t, err)
	assert.True(t, errx.Is(err, errx.SearchTimeout))
}

func TestDerivedUserTagAndHotSearchMappings(t *testing.T) {
	fake := &fakeStore{
		tagsFn: func(_ context.Context, keyword string, limit int32) ([]store.Tag, error) {
			assert.Equal(t, "go", keyword)
			assert.Equal(t, int32(5), limit)
			return []store.Tag{{Name: "golang", PostCount: 9}}, nil
		},
		hotFn: func(_ context.Context, limit int32) ([]string, error) {
			assert.Equal(t, int32(3), limit)
			return []string{"golang", "database"}, nil
		},
	}
	userService := &fakeUserService{searchUsersFn: func(_ context.Context, in *userservice.SearchUsersReq) (*userservice.SearchUsersResp, error) {
		assert.Equal(t, "distributed", in.Keyword)
		assert.Equal(t, int32(2), in.Page)
		assert.Equal(t, int32(10), in.PageSize)
		return &userservice.SearchUsersResp{
			Users: []*userservice.UserInfo{{Id: 42, Username: "user-42"}}, Total: 11,
		}, nil
	}}

	users, err := NewSearchUsersLogic(context.Background(), serviceContextWithUser(fake, userService)).SearchUsers(&pb.SearchUsersReq{
		Keyword: "distributed", Page: 2, PageSize: 10,
	})
	require.NoError(t, err)
	require.Len(t, users.Users, 1)
	assert.Equal(t, int64(42), users.Users[0].Id)
	assert.Equal(t, "user-42", users.Users[0].Username)
	assert.Equal(t, int64(11), users.Total)

	tags, err := NewSearchTagsLogic(context.Background(), serviceContext(fake)).SearchTags(&pb.SearchTagsReq{Keyword: "go", Limit: 5})
	require.NoError(t, err)
	assert.Equal(t, "golang", tags.Tags[0].Name)
	assert.Equal(t, int64(9), tags.Tags[0].PostCount)

	hot, err := NewGetHotSearchesLogic(context.Background(), serviceContext(fake)).GetHotSearches(&pb.GetHotSearchesReq{Limit: 3})
	require.NoError(t, err)
	assert.Equal(t, []string{"golang", "database"}, hot.Keywords)
}

func TestCombinedSearchSuccessAndTagFailure(t *testing.T) {
	ctx := context.Background()
	fake := &fakeStore{
		postsFn: func(got context.Context, _ store.PostQuery) (store.PostResult, error) {
			assert.Equal(t, ctx, got)
			return store.PostResult{Posts: []store.Post{{ID: 1}}}, nil
		},
		tagsFn: func(got context.Context, _ string, _ int32) ([]store.Tag, error) {
			assert.Equal(t, ctx, got)
			return []store.Tag{{Name: "go", PostCount: 3}}, nil
		},
	}
	userService := &fakeUserService{searchUsersFn: func(got context.Context, _ *userservice.SearchUsersReq) (*userservice.SearchUsersResp, error) {
		assert.Equal(t, ctx, got)
		return &userservice.SearchUsersResp{Users: []*userservice.UserInfo{{Id: 2, Username: "gopher"}}, Total: 1}, nil
	}}
	resp, err := NewSearchLogic(ctx, serviceContextWithUser(fake, userService)).Search(&pb.SearchReq{Keyword: "go", Page: 1, PageSize: 10})
	require.NoError(t, err)
	assert.Len(t, resp.Posts, 1)
	assert.Len(t, resp.Users, 1)
	assert.Len(t, resp.Tags, 1)

	fake.tagsFn = func(context.Context, string, int32) ([]store.Tag, error) {
		return nil, errors.New("aggregation failed")
	}
	resp, err = NewSearchLogic(ctx, serviceContextWithUser(fake, userService)).Search(&pb.SearchReq{Keyword: "go", Page: 1, PageSize: 10})
	require.NoError(t, err)
	assert.True(t, resp.Degraded)
	assert.Equal(t, []string{"tag"}, resp.UnavailableTypes)
	assert.Len(t, resp.Posts, 1)
	assert.Len(t, resp.Users, 1)
	assert.Empty(t, resp.Tags)
}

func TestCombinedSearchUserFailureDegrades(t *testing.T) {
	ctx := context.Background()
	fake := &fakeStore{
		postsFn: func(context.Context, store.PostQuery) (store.PostResult, error) {
			return store.PostResult{Posts: []store.Post{{ID: 1}}}, nil
		},
		tagsFn: func(context.Context, string, int32) ([]store.Tag, error) {
			return []store.Tag{{Name: "go", PostCount: 3}}, nil
		},
	}
	userService := &fakeUserService{searchUsersFn: func(context.Context, *userservice.SearchUsersReq) (*userservice.SearchUsersResp, error) {
		return nil, errors.New("user rpc unavailable")
	}}

	resp, err := NewSearchLogic(ctx, serviceContextWithUser(fake, userService)).Search(&pb.SearchReq{Keyword: "go", Page: 1, PageSize: 10})
	require.NoError(t, err)
	assert.True(t, resp.Degraded)
	assert.Equal(t, []string{"user"}, resp.UnavailableTypes)
	assert.Len(t, resp.Posts, 1)
	assert.Len(t, resp.Tags, 1)
	assert.Empty(t, resp.Users)
}

func TestCombinedSearchUserAndTagFailureDegrades(t *testing.T) {
	ctx := context.Background()
	fake := &fakeStore{
		postsFn: func(context.Context, store.PostQuery) (store.PostResult, error) {
			return store.PostResult{Posts: []store.Post{{ID: 1}}}, nil
		},
		tagsFn: func(context.Context, string, int32) ([]store.Tag, error) {
			return nil, errors.New("tags unavailable")
		},
	}
	userService := &fakeUserService{searchUsersFn: func(context.Context, *userservice.SearchUsersReq) (*userservice.SearchUsersResp, error) {
		return nil, errors.New("users unavailable")
	}}

	resp, err := NewSearchLogic(ctx, serviceContextWithUser(fake, userService)).Search(&pb.SearchReq{Keyword: "go", Page: 1, PageSize: 10})
	require.NoError(t, err)
	assert.True(t, resp.Degraded)
	assert.ElementsMatch(t, []string{"user", "tag"}, resp.UnavailableTypes)
	assert.Len(t, resp.Posts, 1)
}

func TestSearchUsersUserRPCFailureIsUnavailable(t *testing.T) {
	fake := &fakeStore{}
	userService := &fakeUserService{searchUsersFn: func(context.Context, *userservice.SearchUsersReq) (*userservice.SearchUsersResp, error) {
		return nil, errors.New("user rpc unavailable")
	}}

	_, err := NewSearchUsersLogic(context.Background(), serviceContextWithUser(fake, userService)).SearchUsers(
		&pb.SearchUsersReq{Keyword: "go", Page: 1, PageSize: 20},
	)
	require.Error(t, err)
	assert.True(t, errx.Is(err, errx.ServiceUnavailable))
}

func TestSearchValidation(t *testing.T) {
	fake := &fakeStore{}
	_, err := NewSearchLogic(context.Background(), serviceContext(fake)).Search(&pb.SearchReq{Keyword: " ", Page: 1, PageSize: 20})
	assert.True(t, errx.Is(err, errx.SearchEmpty))
	_, err = NewSearchUsersLogic(context.Background(), serviceContext(fake)).SearchUsers(&pb.SearchUsersReq{Keyword: "go", Page: 0, PageSize: 20})
	assert.True(t, errx.Is(err, errx.ParamError))
	_, err = NewSearchTagsLogic(context.Background(), serviceContext(fake)).SearchTags(&pb.SearchTagsReq{Keyword: "go", Limit: 101})
	assert.True(t, errx.Is(err, errx.ParamError))
}

func TestSearchDropsUnpublishedPosts(t *testing.T) {
	ctx := context.Background()
	fake := &fakeStore{
		postsFn: func(context.Context, store.PostQuery) (store.PostResult, error) {
			return store.PostResult{Posts: []store.Post{{ID: 1, Title: "live"}, {ID: 2, Title: "gone"}}}, nil
		},
		tagsFn: func(context.Context, string, int32) ([]store.Tag, error) { return nil, nil },
	}
	content := &fakeContentService{getPostsByIDs: func(_ context.Context, in *contentservice.GetPostsByIdsReq) (*contentservice.GetPostsByIdsResp, error) {
		return &contentservice.GetPostsByIdsResp{Posts: []*contentservice.PostInfo{{Id: 1, Status: 1, Title: "live"}}}, nil
	}}
	resp, err := NewSearchLogic(ctx, serviceContextWithDeps(fake, &fakeUserService{}, content)).Search(&pb.SearchReq{Keyword: "go", Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.Len(t, resp.Posts, 1)
	assert.Equal(t, int64(1), resp.Posts[0].Id)
}

func TestSearchVisibilityUnavailableFailsClosed(t *testing.T) {
	ctx := context.Background()
	fake := &fakeStore{
		postsFn: func(context.Context, store.PostQuery) (store.PostResult, error) {
			return store.PostResult{Posts: []store.Post{{ID: 1}}}, nil
		},
		tagsFn: func(context.Context, string, int32) ([]store.Tag, error) { return nil, nil },
	}
	content := &fakeContentService{getPostsByIDs: func(context.Context, *contentservice.GetPostsByIdsReq) (*contentservice.GetPostsByIdsResp, error) {
		return nil, errors.New("content down")
	}}
	_, err := NewSearchLogic(ctx, serviceContextWithDeps(fake, &fakeUserService{}, content)).Search(&pb.SearchReq{Keyword: "go", Page: 1, PageSize: 10})
	require.Error(t, err)
	assert.True(t, errx.Is(err, errx.ServiceUnavailable))
}

func TestSearchPostsUsesAuthoritativeTitleAndReducesTotal(t *testing.T) {
	fake := &fakeStore{postsFn: func(context.Context, store.PostQuery) (store.PostResult, error) {
		return store.PostResult{Posts: []store.Post{
			{ID: 1, Title: "stale live", ContentHighlight: "old snippet"},
			{ID: 2, Title: "gone"},
		}, Total: 5}, nil
	}}
	content := &fakeContentService{getPostsByIDs: func(_ context.Context, in *contentservice.GetPostsByIdsReq) (*contentservice.GetPostsByIdsResp, error) {
		return &contentservice.GetPostsByIdsResp{Posts: []*contentservice.PostInfo{
			{Id: 1, Status: 1, Title: "live title", Content: "current published body"},
			{Id: 2, Status: 2, Title: "unpublished"},
		}}, nil
	}}

	resp, err := NewSearchPostsLogic(context.Background(), serviceContextWithDeps(fake, &fakeUserService{}, content)).SearchPosts(
		&pb.SearchPostsReq{Keyword: "go", Page: 1, PageSize: 10},
	)
	require.NoError(t, err)
	require.Len(t, resp.Posts, 1)
	assert.Equal(t, int64(1), resp.Posts[0].Id)
	assert.Equal(t, "live title", resp.Posts[0].Title)
	assert.Equal(t, "current published body", resp.Posts[0].ContentHighlight)
	assert.Equal(t, int64(4), resp.Total)
}

func TestSearchPostsVisibilityUnavailableFailsClosed(t *testing.T) {
	ctx := context.Background()
	fake := &fakeStore{postsFn: func(context.Context, store.PostQuery) (store.PostResult, error) {
		return store.PostResult{Posts: []store.Post{{ID: 1}}}, nil
	}}
	_, err := NewSearchPostsLogic(ctx, serviceContextWithDeps(fake, &fakeUserService{}, nil)).SearchPosts(&pb.SearchPostsReq{Keyword: "go", Page: 1, PageSize: 10})
	require.Error(t, err)
	assert.True(t, errx.Is(err, errx.ServiceUnavailable))
}
