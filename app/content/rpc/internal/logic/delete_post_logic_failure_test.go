package logic

import (
	"context"
	"errors"
	"testing"

	model2 "esx/app/content/rpc/internal/model"
	"esx/app/content/rpc/internal/svc"
	"esx/app/content/rpc/pb/xiaobaihe/content/pb"
	"esx/pkg/errx"
	"esx/pkg/outboxx"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// deleteCommandStub 注入 PostCommandModel.DeletePost 的返回值。
type deleteCommandStub struct {
	model2.PostCommandModel
	err error
}

func (s *deleteCommandStub) DeletePost(context.Context, int64, outboxx.Event, int64) error {
	return s.err
}

// failingCachePostModel 在 Mock 基础上注入缓存失效失败。
type failingCachePostModel struct {
	*MockPostModel
}

func (f *failingCachePostModel) InvalidatePostCache(context.Context, int64) error {
	return errors.New("redis down")
}

func TestDeletePostRejectsInvalidParams(t *testing.T) {
	svcCtx := newUnitSvcCtx(new(MockPostModel), nil, nil, nil)
	for _, req := range []*pb.DeletePostReq{
		{PostId: 0, AuthorId: 4001},
		{PostId: 400, AuthorId: 0},
	} {
		_, err := NewDeletePostLogic(context.Background(), svcCtx).DeletePost(req)
		require.Error(t, err)
		assert.True(t, errx.Is(err, errx.ParamError), "req %+v: want ParamError, got %v", req, err)
	}
}

func TestDeletePostWithoutCommandModelFails(t *testing.T) {
	pm := new(MockPostModel)
	pm.On("FindPostById", mock.Anything, int64(400)).
		Return(&model2.Post{Id: 400, AuthorId: 4001, Status: 1, Revision: 1}, nil).Once()
	svcCtx := &svc.ServiceContext{PostModel: pm}
	_, err := NewDeletePostLogic(context.Background(), svcCtx).
		DeletePost(&pb.DeletePostReq{PostId: 400, AuthorId: 4001})
	require.Error(t, err)
	assert.True(t, errx.Is(err, errx.SystemError), "want SystemError, got %v", err)
	pm.AssertExpectations(t)
}

func TestDeletePostTransactionVersionConflict(t *testing.T) {
	pm := new(MockPostModel)
	pm.On("FindPostById", mock.Anything, int64(400)).
		Return(&model2.Post{Id: 400, AuthorId: 4001, Status: 1, Revision: 1}, nil).Once()
	svcCtx := &svc.ServiceContext{
		PostModel:        pm,
		PostCommandModel: &deleteCommandStub{err: model2.ErrVersionConflict},
	}
	_, err := NewDeletePostLogic(context.Background(), svcCtx).
		DeletePost(&pb.DeletePostReq{PostId: 400, AuthorId: 4001})
	require.Error(t, err)
	assert.True(t, errx.Is(err, errx.ContentVersionConflict), "got %v", err)
	pm.AssertExpectations(t)
}

func TestDeletePostTransactionUnexpectedFailure(t *testing.T) {
	pm := new(MockPostModel)
	pm.On("FindPostById", mock.Anything, int64(400)).
		Return(&model2.Post{Id: 400, AuthorId: 4001, Status: 1, Revision: 1}, nil).Once()
	svcCtx := &svc.ServiceContext{
		PostModel:        pm,
		PostCommandModel: &deleteCommandStub{err: errors.New("tx aborted")},
	}
	_, err := NewDeletePostLogic(context.Background(), svcCtx).
		DeletePost(&pb.DeletePostReq{PostId: 400, AuthorId: 4001})
	require.Error(t, err)
	assert.True(t, errx.Is(err, errx.SystemError), "want SystemError, got %v", err)
	pm.AssertExpectations(t)
}

func TestDeletePostToleratesCacheInvalidationFailure(t *testing.T) {
	mockModel := new(MockPostModel)
	mockModel.On("FindPostById", mock.Anything, int64(400)).
		Return(&model2.Post{Id: 400, AuthorId: 4001, Status: 1, Revision: 1}, nil).Once()
	mockModel.On("UpdateStatus", mock.Anything, int64(400), int64(2)).Return(nil).Once()

	pm := &failingCachePostModel{MockPostModel: mockModel}
	svcCtx := &svc.ServiceContext{
		PostModel:        pm,
		PostCommandModel: legacyPostCommandModel{post: mockModel},
	}
	resp, err := NewDeletePostLogic(context.Background(), svcCtx).
		DeletePost(&pb.DeletePostReq{PostId: 400, AuthorId: 4001})
	// 缓存失效失败只告警，不回滚删除结果。
	require.NoError(t, err)
	require.NotNil(t, resp)
	mockModel.AssertExpectations(t)
}
