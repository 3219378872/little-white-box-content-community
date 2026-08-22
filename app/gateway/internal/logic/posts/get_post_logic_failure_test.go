package posts

import (
	"context"
	"errors"
	"testing"

	"esx/app/content/rpc/contentservice"
	contentpb "esx/app/content/rpc/pb/xiaobaihe/content/pb"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/app/interaction/rpc/interactionservice"
	"esx/app/user/rpc/userservice"
	"esx/pkg/errx"
	"esx/pkg/jwtx"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

type failingGetPostInteractionService struct {
	interactionservice.InteractionService
	likedErr error
}

func (f *failingGetPostInteractionService) BatchCheckLiked(_ context.Context, _ *interactionservice.BatchCheckLikedReq, _ ...grpc.CallOption) (*interactionservice.BatchCheckLikedResp, error) {
	return nil, f.likedErr
}

func getPostContentStub(fn func(_ context.Context, in *contentservice.GetPostReq, opts ...grpc.CallOption) (*contentservice.GetPostResp, error)) *fakeGetPostContentService {
	return &fakeGetPostContentService{getPostFn: fn}
}

func publishedPost() *contentservice.GetPostResp {
	return &contentservice.GetPostResp{Post: &contentpb.PostInfo{
		Id: 11, AuthorId: 7, Title: "pub", Content: "body", Status: 1, Revision: 2,
	}}
}

func TestGetPost_RPCFailed(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		ContentService: getPostContentStub(func(_ context.Context, _ *contentservice.GetPostReq, _ ...grpc.CallOption) (*contentservice.GetPostResp, error) {
			return nil, errors.New("rpc unavailable")
		}),
	}
	resp, err := NewGetPostLogic(context.Background(), svcCtx).GetPost(&types.GetPostReq{PostId: 11})
	require.Error(t, err)
	assert.Nil(t, resp)
}

func TestGetPost_PostMissing(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		ContentService: getPostContentStub(func(_ context.Context, _ *contentservice.GetPostReq, _ ...grpc.CallOption) (*contentservice.GetPostResp, error) {
			return &contentservice.GetPostResp{Post: nil}, nil
		}),
	}
	_, err := NewGetPostLogic(context.Background(), svcCtx).GetPost(&types.GetPostReq{PostId: 11})
	require.Error(t, err)
	assert.True(t, errx.Is(err, errx.ContentNotFound), "got %v", err)
}

func TestGetPost_ViewerStateFailed(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		ContentService: getPostContentStub(func(_ context.Context, _ *contentservice.GetPostReq, _ ...grpc.CallOption) (*contentservice.GetPostResp, error) {
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
		ContentService: getPostContentStub(func(_ context.Context, _ *contentservice.GetPostReq, _ ...grpc.CallOption) (*contentservice.GetPostResp, error) {
			return publishedPost(), nil
		}),
		UserService: &fakeGetPostUserService{
			batchGetUsersFn: func(_ context.Context, _ *userservice.BatchGetUsersReq, _ ...grpc.CallOption) (*userservice.BatchGetUsersResp, error) {
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
		ContentService: getPostContentStub(func(_ context.Context, _ *contentservice.GetPostReq, _ ...grpc.CallOption) (*contentservice.GetPostResp, error) {
			return publishedPost(), nil
		}),
		UserService: &fakeGetPostUserService{
			batchGetUsersFn: func(_ context.Context, _ *userservice.BatchGetUsersReq, _ ...grpc.CallOption) (*userservice.BatchGetUsersResp, error) {
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
		ContentService: getPostContentStub(func(_ context.Context, _ *contentservice.GetPostReq, _ ...grpc.CallOption) (*contentservice.GetPostResp, error) {
			return publishedPost(), nil
		}),
		UserService: &fakeGetPostUserService{
			batchGetUsersFn: func(_ context.Context, _ *userservice.BatchGetUsersReq, _ ...grpc.CallOption) (*userservice.BatchGetUsersResp, error) {
				// 含 nil 用户与不匹配 ID：应跳过；目标作者昵称为空则回退用户名。
				return &userservice.BatchGetUsersResp{Users: []*userservice.UserInfo{
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
		ContentService: getPostContentStub(func(_ context.Context, _ *contentservice.GetPostReq, _ ...grpc.CallOption) (*contentservice.GetPostResp, error) {
			return publishedPost(), nil
		}),
		UserService: &fakeGetPostUserService{
			batchGetUsersFn: func(_ context.Context, _ *userservice.BatchGetUsersReq, _ ...grpc.CallOption) (*userservice.BatchGetUsersResp, error) {
				return &userservice.BatchGetUsersResp{Users: []*userservice.UserInfo{
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
