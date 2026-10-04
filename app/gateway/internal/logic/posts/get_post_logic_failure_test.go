package posts

import (
	"context"
	"errors"
	"testing"

	"github.com/cloudwego/kitex/client/callopt"

	"esx/app/content/rpc/contentservice"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/app/interaction/rpc/interactionservice"
	"esx/app/user/rpc/userservice"
	contentpb "esx/kitex_gen/content"
	"esx/pkg/errx"
	"esx/pkg/jwtx"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type failingGetPostInteractionService struct {
	interactionservice.InteractionService
	likedErr error
}

func (f *failingGetPostInteractionService) BatchCheckLiked(_ context.Context, _ *interactionservice.BatchCheckLikedReq, _ ...callopt.Option) (*interactionservice.BatchCheckLikedResp, error) {
	return nil, f.likedErr
}

func getPostContentStub(fn func(_ context.Context, in *contentservice.GetPostReq, opts ...callopt.Option) (*contentservice.GetPostResp, error)) *fakeGetPostContentService {
	return &fakeGetPostContentService{getPostFn: fn}
}

func publishedPost() *contentservice.GetPostResp {
	return &contentservice.GetPostResp{Post: &contentpb.PostInfo{
		Id: 11, AuthorId: 7, Title: "pub", Content: "body", Status: 1, Revision: 2,
	}}
}

func TestGetPost_RPCFailed(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		ContentService: getPostContentStub(func(_ context.Context, _ *contentservice.GetPostReq, _ ...callopt.Option) (*contentservice.GetPostResp, error) {
			return nil, errors.New("rpc unavailable")
		}),
	}
	resp, err := NewGetPostLogic(context.Background(), svcCtx).GetPost(&types.GetPostReq{PostId: 11})
	require.Error(t, err)
	assert.Nil(t, resp)
}

func TestGetPost_PostMissing(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		ContentService: getPostContentStub(func(_ context.Context, _ *contentservice.GetPostReq, _ ...callopt.Option) (*contentservice.GetPostResp, error) {
			return &contentservice.GetPostResp{Post: nil}, nil
		}),
	}
	_, err := NewGetPostLogic(context.Background(), svcCtx).GetPost(&types.GetPostReq{PostId: 11})
	require.Error(t, err)
	assert.True(t, errx.Is(err, errx.ContentNotFound), "got %v", err)
}

func TestGetPost_ViewerStateFailed(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		ContentService: getPostContentStub(func(_ context.Context, _ *contentservice.GetPostReq, _ ...callopt.Option) (*contentservice.GetPostResp, error) {
			return publishedPost(), nil
		}),
		InteractionService: &failingGetPostInteractionService{likedErr: errors.New("interaction rpc down")},
	}
	ctx := jwtx.WithUserIdContext(context.Background(), 42)
	resp, err := NewGetPostLogic(ctx, svcCtx).GetPost(&types.GetPostReq{PostId: 11})
	require.Error(t, err)
	assert.Nil(t, resp)
}

func TestGetPost_AuthorLookupFailedStillReturnsPost(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		ContentService: getPostContentStub(func(_ context.Context, _ *contentservice.GetPostReq, _ ...callopt.Option) (*contentservice.GetPostResp, error) {
			return publishedPost(), nil
		}),
		UserService: &fakeGetPostUserService{
			batchGetUserCardsFn: func(_ context.Context, _ *userservice.BatchGetUserCardsReq, _ ...callopt.Option) (*userservice.BatchGetUserCardsResp, error) {
				return nil, errors.New("user rpc down")
			},
		},
	}
	resp, err := NewGetPostLogic(context.Background(), svcCtx).GetPost(&types.GetPostReq{PostId: 11})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "pub", resp.Title)
	assert.Empty(t, resp.AuthorName)
	assert.Empty(t, resp.AuthorAvatar)
}

func TestGetPost_AuthorNilResponseStillReturnsPost(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		ContentService: getPostContentStub(func(_ context.Context, _ *contentservice.GetPostReq, _ ...callopt.Option) (*contentservice.GetPostResp, error) {
			return publishedPost(), nil
		}),
		UserService: &fakeGetPostUserService{
			batchGetUserCardsFn: func(_ context.Context, _ *userservice.BatchGetUserCardsReq, _ ...callopt.Option) (*userservice.BatchGetUserCardsResp, error) {
				return nil, nil
			},
		},
	}
	resp, err := NewGetPostLogic(context.Background(), svcCtx).GetPost(&types.GetPostReq{PostId: 11})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Empty(t, resp.AuthorName)
}

func TestGetPost_AuthorNicknameFallbackAndMismatchSkip(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		ContentService: getPostContentStub(func(_ context.Context, _ *contentservice.GetPostReq, _ ...callopt.Option) (*contentservice.GetPostResp, error) {
			return publishedPost(), nil
		}),
		UserService: &fakeGetPostUserService{
			batchGetUserCardsFn: func(_ context.Context, _ *userservice.BatchGetUserCardsReq, _ ...callopt.Option) (*userservice.BatchGetUserCardsResp, error) {
				// 含 nil 用户与不匹配 ID：应跳过；目标作者昵称为空则回退用户名。
				return &userservice.BatchGetUserCardsResp{Users: []*userservice.UserCard{
					nil,
					{Id: 8, Nickname: "someone-else", Username: "other"},
					{Id: 7, Nickname: "", Username: "alice", AvatarUrl: " https://avatar/7.png "},
				}}, nil
			},
		},
	}
	resp, err := NewGetPostLogic(context.Background(), svcCtx).GetPost(&types.GetPostReq{PostId: 11})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "alice", resp.AuthorName)
	assert.Equal(t, "https://avatar/7.png", resp.AuthorAvatar)
}

func TestGetPost_AuthorNotInBatchResult(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		ContentService: getPostContentStub(func(_ context.Context, _ *contentservice.GetPostReq, _ ...callopt.Option) (*contentservice.GetPostResp, error) {
			return publishedPost(), nil
		}),
		UserService: &fakeGetPostUserService{
			batchGetUserCardsFn: func(_ context.Context, _ *userservice.BatchGetUserCardsReq, _ ...callopt.Option) (*userservice.BatchGetUserCardsResp, error) {
				return &userservice.BatchGetUserCardsResp{Users: []*userservice.UserCard{
					{Id: 9, Nickname: "unrelated"},
				}}, nil
			},
		},
	}
	resp, err := NewGetPostLogic(context.Background(), svcCtx).GetPost(&types.GetPostReq{PostId: 11})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Empty(t, resp.AuthorName)
}
