package logic

import (
	"context"
	"errors"
	"testing"

	"errx"
	"esx/app/content/rpc/contentservice"
	"esx/app/feed/rpc/internal/svc"
	"esx/app/feed/rpc/xiaobaihe/feed/pb"

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

func TestGetRecommendFeedLogic_Success(t *testing.T) {
	contentSvc := new(mockContentService)
	contentSvc.On("GetPostList", mock.Anything, &contentservice.GetPostListReq{Page: 1, PageSize: 2, SortBy: 3}).Return(&contentservice.GetPostListResp{
		Posts: []*contentservice.PostInfo{{Id: 11, AuthorId: 101, CreatedAt: 1001}, {Id: 12, AuthorId: 102, CreatedAt: 1000}},
		Total: 3,
	}, nil).Once()
	logic := NewGetRecommendFeedLogic(context.Background(), &svc.ServiceContext{ContentService: contentSvc})
	resp, err := logic.GetRecommendFeed(&pb.GetRecommendFeedReq{UserId: 1, Page: 1, PageSize: 2})

	require.NoError(t, err)
	require.Len(t, resp.Items, 2)
	assert.Equal(t, int32(2), resp.Items[0].FeedType)
	assert.True(t, resp.HasMore)
	contentSvc.AssertExpectations(t)
}

func TestGetRecommendFeedLogic_FallbackSort(t *testing.T) {
	contentSvc := new(mockContentService)
	contentSvc.On("GetPostList", mock.Anything, &contentservice.GetPostListReq{Page: 1, PageSize: 2, SortBy: 3}).Return(nil, errors.New("sort unsupported")).Once()
	contentSvc.On("GetPostList", mock.Anything, &contentservice.GetPostListReq{Page: 1, PageSize: 2, SortBy: 1}).Return(&contentservice.GetPostListResp{
		Posts: []*contentservice.PostInfo{{Id: 21, AuthorId: 201, CreatedAt: 2001}},
		Total: 1,
	}, nil).Once()
	logic := NewGetRecommendFeedLogic(context.Background(), &svc.ServiceContext{ContentService: contentSvc})
	resp, err := logic.GetRecommendFeed(&pb.GetRecommendFeedReq{UserId: 1, Page: 1, PageSize: 2})

	require.NoError(t, err)
	require.Len(t, resp.Items, 1)
	assert.False(t, resp.HasMore)
	contentSvc.AssertExpectations(t)
}

func TestGetRecommendFeedLogic_InvalidInput(t *testing.T) {
	logic := NewGetRecommendFeedLogic(context.Background(), &svc.ServiceContext{})
	resp, err := logic.GetRecommendFeed(&pb.GetRecommendFeedReq{UserId: 1, Page: 0, PageSize: 2})

	require.Nil(t, resp)
	require.Error(t, err)
	assert.Equal(t, errx.ParamError, errx.GetCode(err))
}
