package logic

import (
	"context"
	"fmt"
	"testing"

	model2 "esx/app/content/rpc/internal/model"
	"esx/app/content/rpc/internal/svc"
	"esx/app/content/rpc/pb/xiaobaihe/content/pb"

	"esx/pkg/errx"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestAssertInteractable(t *testing.T) {
	published := &model2.Post{Id: 11, Status: 1}
	draft := &model2.Post{Id: 12, Status: 0}
	activeComment := &model2.Comment{Id: 21, PostId: 11, Status: 1}
	deletedComment := &model2.Comment{Id: 22, PostId: 11, Status: 0}
	orphanComment := &model2.Comment{Id: 23, PostId: 12, Status: 1}

	tests := []struct {
		name    string
		req     *pb.AssertInteractableReq
		setup   func(pm *MockPostModel, cm *MockCommentModel)
		errCode int
	}{
		{
			name: "published post",
			req:  &pb.AssertInteractableReq{TargetId: 11, TargetType: 1},
			setup: func(pm *MockPostModel, _ *MockCommentModel) {
				pm.On("FindPostById", mock.Anything, int64(11)).Return(published, nil)
			},
		},
		{
			name: "draft post",
			req:  &pb.AssertInteractableReq{TargetId: 12, TargetType: 1},
			setup: func(pm *MockPostModel, _ *MockCommentModel) {
				pm.On("FindPostById", mock.Anything, int64(12)).Return(draft, nil)
			},
			errCode: errx.ContentNotFound,
		},
		{
			name: "missing post",
			req:  &pb.AssertInteractableReq{TargetId: 99, TargetType: 1},
			setup: func(pm *MockPostModel, _ *MockCommentModel) {
				pm.On("FindPostById", mock.Anything, int64(99)).Return(nil, model2.ErrNotFound)
			},
			errCode: errx.ContentNotFound,
		},
		{
			name: "post authority down",
			req:  &pb.AssertInteractableReq{TargetId: 11, TargetType: 1},
			setup: func(pm *MockPostModel, _ *MockCommentModel) {
				pm.On("FindPostById", mock.Anything, int64(11)).Return(nil, fmt.Errorf("db down"))
			},
			errCode: errx.SystemError,
		},
		{
			name: "comment on published post",
			req:  &pb.AssertInteractableReq{TargetId: 21, TargetType: 2},
			setup: func(pm *MockPostModel, cm *MockCommentModel) {
				cm.On("FindCommentById", mock.Anything, int64(21)).Return(activeComment, nil)
				pm.On("FindPostById", mock.Anything, int64(11)).Return(published, nil)
			},
		},
		{
			name: "comment on draft post",
			req:  &pb.AssertInteractableReq{TargetId: 23, TargetType: 2},
			setup: func(pm *MockPostModel, cm *MockCommentModel) {
				cm.On("FindCommentById", mock.Anything, int64(23)).Return(orphanComment, nil)
				pm.On("FindPostById", mock.Anything, int64(12)).Return(draft, nil)
			},
			errCode: errx.ContentNotFound,
		},
		{
			name: "deleted comment",
			req:  &pb.AssertInteractableReq{TargetId: 22, TargetType: 2},
			setup: func(_ *MockPostModel, cm *MockCommentModel) {
				cm.On("FindCommentById", mock.Anything, int64(22)).Return(deletedComment, nil)
			},
			errCode: errx.ContentNotFound,
		},
		{
			name: "missing comment",
			req:  &pb.AssertInteractableReq{TargetId: 99, TargetType: 2},
			setup: func(_ *MockPostModel, cm *MockCommentModel) {
				cm.On("FindCommentById", mock.Anything, int64(99)).Return(nil, model2.ErrNotFound)
			},
			errCode: errx.ContentNotFound,
		},
		{
			name:    "invalid type",
			req:     &pb.AssertInteractableReq{TargetId: 11, TargetType: 9},
			setup:   func(*MockPostModel, *MockCommentModel) {},
			errCode: errx.ParamError,
		},
		{
			name:    "missing id",
			req:     &pb.AssertInteractableReq{TargetId: 0, TargetType: 1},
			setup:   func(*MockPostModel, *MockCommentModel) {},
			errCode: errx.ParamError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pm := new(MockPostModel)
			cm := new(MockCommentModel)
			tt.setup(pm, cm)
			logic := NewAssertInteractableLogic(context.Background(), &svc.ServiceContext{
				PostModel:    pm,
				CommentModel: cm,
			})
			resp, err := logic.AssertInteractable(tt.req)
			if tt.errCode == 0 {
				require.NoError(t, err)
				require.NotNil(t, resp)
			} else {
				require.Error(t, err)
				require.True(t, errx.Is(err, tt.errCode), "got %v", err)
			}
			pm.AssertExpectations(t)
			cm.AssertExpectations(t)
		})
	}
}
