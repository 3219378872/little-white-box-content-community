package logic

import (
	"context"
	"errors"
	"testing"
	"time"

	"errx"
	"esx/app/content/rpc/contentservice"
	"esx/app/feed/rpc/internal/config"
	"esx/app/feed/rpc/internal/svc"
	"esx/app/feed/rpc/xiaobaihe/feed/pb"
	"esx/app/recommend/rpc/recommendservice"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

type mockContentService struct{ mock.Mock }

func (m *mockContentService) GetPostList(ctx context.Context, in *contentservice.GetPostListReq, opts ...grpc.CallOption) (*contentservice.GetPostListResp, error) {
	args := m.Called(ctx, in)
	if v := args.Get(0); v != nil {
		return v.(*contentservice.GetPostListResp), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockContentService) GetPostsByIds(ctx context.Context, in *contentservice.GetPostsByIdsReq, opts ...grpc.CallOption) (*contentservice.GetPostsByIdsResp, error) {
	args := m.Called(ctx, in)
	if v := args.Get(0); v != nil {
		return v.(*contentservice.GetPostsByIdsResp), args.Error(1)
	}
	return nil, args.Error(1)
}

type mockRecommendService struct{ mock.Mock }

func (m *mockRecommendService) GetRecommendPosts(ctx context.Context, in *recommendservice.GetRecommendPostsReq, opts ...grpc.CallOption) (*recommendservice.GetRecommendPostsResp, error) {
	args := m.Called(ctx, in)
	if v := args.Get(0); v != nil {
		return v.(*recommendservice.GetRecommendPostsResp), args.Error(1)
	}
	return nil, args.Error(1)
}

func TestGetRecommendFeedLogic_EnrichesRecommendationAndFiltersMissingPosts(t *testing.T) {
	type requestContextKey struct{}
	ctx := context.WithValue(context.Background(), requestContextKey{}, "request-context")
	recommendSvc := new(mockRecommendService)
	contentSvc := new(mockContentService)
	recommendSvc.On("GetRecommendPosts", ctx, &recommendservice.GetRecommendPostsReq{
		UserId: 7, Scene: "home", RequestId: "req-1", SessionId: "session-1", PageSize: 3,
	}).Return(&recommendservice.GetRecommendPostsResp{
		Posts: []*recommendservice.RecommendPost{
			{PostId: 12, Score: 0.8, RecallSource: "popular"},
			{PostId: 11, Score: 0.9, Reason: "interest", RecallSource: "itemcf", ModelVersion: "rank-v2", ExperimentId: "exp-a"},
			{PostId: 13, Score: 0.7, Reason: "explore", RecallSource: "explore", ModelVersion: "rank-v2", Position: 7},
			{PostId: 14, Score: 0.6, Reason: "missing", RecallSource: "explore"},
		},
		NextCursor: "recommend-next", HasMore: true, RequestId: "server-req-1",
	}, nil).Once()
	contentSvc.On("GetPostsByIds", ctx, &contentservice.GetPostsByIdsReq{PostIds: []int64{12, 11, 13, 14}}).Return(&contentservice.GetPostsByIdsResp{
		Posts: []*contentservice.PostInfo{
			{Id: 12, AuthorId: 102, Title: "unpublished", Status: 2, CreatedAt: 1002},
			{
				Id: 11, AuthorId: 101, Title: "recommend title", Content: "recommend content", Status: 1,
				Images: []string{"recommend.png"}, Tags: []string{"recommend", "go"}, ViewCount: 20,
				LikeCount: 19, CommentCount: 18, FavoriteCount: 17, CreatedAt: 1001,
			},
			{Id: 13, AuthorId: 103, Title: "explore", Status: 1, CreatedAt: 1000},
		},
	}, nil).Once()

	logic := NewGetRecommendFeedLogic(ctx, &svc.ServiceContext{
		ContentService: contentSvc, RecommendService: recommendSvc,
	})
	resp, err := logic.GetRecommendFeed(&pb.GetRecommendFeedReq{
		UserId: 7, RequestId: "req-1", SessionId: "session-1", PageSize: 3,
	})

	require.NoError(t, err)
	require.Len(t, resp.Items, 2)
	item := resp.Items[0]
	assert.Equal(t, int64(11), item.PostId)
	assert.Equal(t, int64(101), item.AuthorId)
	assert.Equal(t, int64(1001), item.CreatedAt)
	assert.Equal(t, int32(feedTypeRecommend), item.FeedType)
	assert.Equal(t, "recommend title", item.Title)
	assert.Equal(t, "recommend content", item.Content)
	assert.Equal(t, []string{"recommend.png"}, item.Images)
	assert.Equal(t, []string{"recommend", "go"}, item.Tags)
	assert.Equal(t, int64(20), item.ViewCount)
	assert.Equal(t, int64(19), item.LikeCount)
	assert.Equal(t, int64(18), item.CommentCount)
	assert.Equal(t, int64(17), item.FavoriteCount)
	assert.Equal(t, 0.9, item.Score)
	assert.Equal(t, "interest", item.Reason)
	assert.Equal(t, "itemcf", item.RecallSource)
	assert.Equal(t, "rank-v2", item.ModelVersion)
	assert.Equal(t, "exp-a", item.ExperimentId)
	assert.Equal(t, int32(2), item.Position)
	assert.Equal(t, int32(7), resp.Items[1].Position)
	assert.True(t, resp.HasMore)
	assert.Equal(t, "recommend-next", resp.NextCursor)
	assert.Equal(t, "server-req-1", resp.RequestId)
	recommendSvc.AssertExpectations(t)
	contentSvc.AssertExpectations(t)
}

func TestGetRecommendFeedLogic_RecommendFailureUsesInterleavedFallback(t *testing.T) {
	recommendSvc := new(mockRecommendService)
	contentSvc := new(mockContentService)
	recommendSvc.On("GetRecommendPosts", mock.Anything, &recommendservice.GetRecommendPostsReq{
		AnonymousId: "device-1", Scene: "home", RequestId: "req-fallback", PageSize: 4, ExperimentId: "exp-fallback",
	}).Return(nil, errors.New("recommend unavailable")).Once()
	contentSvc.On("GetPostList", mock.Anything, &contentservice.GetPostListReq{Page: 1, PageSize: 4, SortBy: 3}).Return(&contentservice.GetPostListResp{
		Posts: []*contentservice.PostInfo{
			{Id: 21, AuthorId: 201, Title: "popular", Content: "popular content", Images: []string{"popular.png"}, Tags: []string{"hot"}, Status: 1, ViewCount: 6, LikeCount: 5, CommentCount: 4, FavoriteCount: 3, CreatedAt: 2001},
			{Id: 22, AuthorId: 202, Status: 1},
		}, Total: 8,
	}, nil).Once()
	contentSvc.On("GetPostList", mock.Anything, &contentservice.GetPostListReq{Page: 1, PageSize: 4, SortBy: 1}).Return(&contentservice.GetPostListResp{
		Posts: []*contentservice.PostInfo{{Id: 22, AuthorId: 202, Status: 1}, {Id: 23, AuthorId: 203, Status: 1}}, Total: 4,
	}, nil).Once()

	logic := NewGetRecommendFeedLogic(context.Background(), &svc.ServiceContext{
		Config: config.Config{CursorSecret: "cursor-secret"}, ContentService: contentSvc, RecommendService: recommendSvc,
	})
	resp, err := logic.GetRecommendFeed(&pb.GetRecommendFeedReq{
		AnonymousId: "device-1", RequestId: "req-fallback", PageSize: 4, ExperimentId: "exp-fallback",
	})

	require.NoError(t, err)
	require.Len(t, resp.Items, 3)
	assert.Equal(t, []int64{21, 22, 23}, []int64{resp.Items[0].PostId, resp.Items[1].PostId, resp.Items[2].PostId})
	assert.Equal(t, []string{"popular", "latest", "latest"}, []string{resp.Items[0].RecallSource, resp.Items[1].RecallSource, resp.Items[2].RecallSource})
	assert.Equal(t, "popular", resp.Items[0].Title)
	assert.Equal(t, "popular content", resp.Items[0].Content)
	assert.Equal(t, []string{"popular.png"}, resp.Items[0].Images)
	assert.Equal(t, []string{"hot"}, resp.Items[0].Tags)
	assert.Equal(t, int64(6), resp.Items[0].ViewCount)
	assert.Equal(t, int64(5), resp.Items[0].LikeCount)
	assert.Equal(t, int64(4), resp.Items[0].CommentCount)
	assert.Equal(t, int64(3), resp.Items[0].FavoriteCount)
	for i, item := range resp.Items {
		assert.Equal(t, int32(i+1), item.Position)
		assert.Equal(t, "rule-fallback-v2", item.ModelVersion)
		assert.Equal(t, "exp-fallback", item.ExperimentId)
	}
	assert.True(t, resp.HasMore)
	assert.True(t, len(resp.NextCursor) > len(fallbackCursorPrefix))
	recommendSvc.AssertExpectations(t)
	contentSvc.AssertExpectations(t)
}

func TestGetRecommendFeedLogic_FallbackCursorContinuesWithoutRecommend(t *testing.T) {
	contentSvc := new(mockContentService)
	cursor, err := encodeFallbackCursor("cursor-secret", "req-2", 2, timeNowForTest())
	require.NoError(t, err)
	contentSvc.On("GetPostList", mock.Anything, &contentservice.GetPostListReq{Page: 2, PageSize: 2, SortBy: 3}).Return(&contentservice.GetPostListResp{
		Posts: []*contentservice.PostInfo{{Id: 31, Status: 1}}, Total: 3,
	}, nil).Once()
	contentSvc.On("GetPostList", mock.Anything, &contentservice.GetPostListReq{Page: 2, PageSize: 2, SortBy: 1}).Return(&contentservice.GetPostListResp{
		Posts: []*contentservice.PostInfo{{Id: 32, Status: 1}}, Total: 3,
	}, nil).Once()

	logic := NewGetRecommendFeedLogic(context.Background(), &svc.ServiceContext{
		Config: config.Config{CursorSecret: "cursor-secret"}, ContentService: contentSvc,
	})
	resp, err := logic.GetRecommendFeed(&pb.GetRecommendFeedReq{
		AnonymousId: "device-2", RequestId: "req-2", Cursor: cursor, PageSize: 2,
	})

	require.NoError(t, err)
	require.Len(t, resp.Items, 2)
	assert.False(t, resp.HasMore)
	assert.Empty(t, resp.NextCursor)
	contentSvc.AssertExpectations(t)
}

func TestGetRecommendFeedLogic_EnrichmentFailureFallsBack(t *testing.T) {
	recommendSvc := new(mockRecommendService)
	contentSvc := new(mockContentService)
	recommendSvc.On("GetRecommendPosts", mock.Anything, mock.Anything).Return(&recommendservice.GetRecommendPostsResp{
		Posts: []*recommendservice.RecommendPost{{PostId: 41}},
	}, nil).Once()
	contentSvc.On("GetPostsByIds", mock.Anything, &contentservice.GetPostsByIdsReq{PostIds: []int64{41}}).Return(nil, errors.New("content unavailable")).Once()
	contentSvc.On("GetPostList", mock.Anything, &contentservice.GetPostListReq{Page: 1, PageSize: 2, SortBy: 3}).Return(&contentservice.GetPostListResp{
		Posts: []*contentservice.PostInfo{{Id: 42, Title: "fallback", Status: 1}}, Total: 1,
	}, nil).Once()
	contentSvc.On("GetPostList", mock.Anything, &contentservice.GetPostListReq{Page: 1, PageSize: 2, SortBy: 1}).Return(nil, errors.New("latest unavailable")).Once()

	logic := NewGetRecommendFeedLogic(context.Background(), &svc.ServiceContext{
		Config: config.Config{CursorSecret: "cursor-secret"}, ContentService: contentSvc, RecommendService: recommendSvc,
	})
	resp, err := logic.GetRecommendFeed(&pb.GetRecommendFeedReq{UserId: 1, RequestId: "req-3", PageSize: 2})

	require.NoError(t, err)
	require.Len(t, resp.Items, 1)
	assert.Equal(t, int64(42), resp.Items[0].PostId)
	assert.Equal(t, "fallback", resp.Items[0].Title)
	contentSvc.AssertExpectations(t)
}

func TestGetRecommendFeedLogic_AllFallbackSourcesFail(t *testing.T) {
	contentSvc := new(mockContentService)
	contentSvc.On("GetPostList", mock.Anything, mock.Anything).Return(nil, errors.New("content unavailable")).Twice()
	logic := NewGetRecommendFeedLogic(context.Background(), &svc.ServiceContext{ContentService: contentSvc})

	resp, err := logic.GetRecommendFeed(&pb.GetRecommendFeedReq{AnonymousId: "device", RequestId: "req-4", PageSize: 2})

	require.Nil(t, resp)
	require.Error(t, err)
	assert.Equal(t, errx.SystemError, errx.GetCode(err))
}

func TestGetRecommendFeedLogic_InvalidInput(t *testing.T) {
	tests := []struct {
		name string
		req  *pb.GetRecommendFeedReq
	}{
		{name: "nil request"},
		{name: "missing identity", req: &pb.GetRecommendFeedReq{RequestId: "req", PageSize: 2}},
		{name: "missing request id", req: &pb.GetRecommendFeedReq{UserId: 1, PageSize: 2}},
		{name: "zero page size", req: &pb.GetRecommendFeedReq{UserId: 1, RequestId: "req"}},
		{name: "oversized page", req: &pb.GetRecommendFeedReq{UserId: 1, RequestId: "req", PageSize: 101}},
		{name: "tampered fallback cursor", req: &pb.GetRecommendFeedReq{UserId: 1, RequestId: "req", PageSize: 2, Cursor: fallbackCursorPrefix + "bad"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logic := NewGetRecommendFeedLogic(context.Background(), &svc.ServiceContext{Config: config.Config{CursorSecret: "cursor-secret"}})
			resp, err := logic.GetRecommendFeed(tt.req)

			require.Nil(t, resp)
			require.Error(t, err)
			assert.Equal(t, errx.ParamError, errx.GetCode(err))
		})
	}
}

func timeNowForTest() time.Time {
	return time.Now()
}
