package logic

import (
	"context"
	"fmt"
	"testing"

	"errx"
	"esx/app/content/internal/model"
	"esx/app/content/pb/xiaobaihe/content/pb"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// ─── CreateComment ────────────────────────────────────────────────────────────

func TestCreateCommentLogic(t *testing.T) {
	publishedPost := &model.Post{Id: 1000, AuthorId: 100, Status: 1}
	deletedPost := &model.Post{Id: 1001, AuthorId: 100, Status: 2}

	tests := []struct {
		name      string
		req       *pb.CreateCommentReq
		setupMock func(*MockPostModel, *MockCommentModel)
		wantErr   bool
		errCode   int
		check     func(t *testing.T, resp *pb.CreateCommentResp)
	}{
		{
			name: "成功创建顶级评论",
			req: &pb.CreateCommentReq{
				PostId:  1000,
				UserId:  200,
				Content: "这是评论内容",
			},
			setupMock: func(pm *MockPostModel, cm *MockCommentModel) {
				pm.On("FindPostById", mock.Anything, int64(1000)).Return(publishedPost, nil)
				cm.On("InsertComment", mock.Anything, mock.AnythingOfType("*model.Comment")).Return(nil)
				pm.On("IncrCommentCount", mock.Anything, int64(1000)).Return(nil)
			},
			check: func(t *testing.T, resp *pb.CreateCommentResp) {
				assert.Greater(t, resp.CommentId, int64(0))
			},
		},
		{
			name: "成功创建回复评论",
			req: &pb.CreateCommentReq{
				PostId:      1000,
				UserId:      201,
				ParentId:    5001,
				ReplyUserId: 200,
				Content:     "这是回复",
			},
			setupMock: func(pm *MockPostModel, cm *MockCommentModel) {
				pm.On("FindPostById", mock.Anything, int64(1000)).Return(publishedPost, nil)
				cm.On("InsertComment", mock.Anything, mock.AnythingOfType("*model.Comment")).Return(nil)
				pm.On("IncrCommentCount", mock.Anything, int64(1000)).Return(nil)
			},
			check: func(t *testing.T, resp *pb.CreateCommentResp) {
				assert.Greater(t, resp.CommentId, int64(0))
			},
		},
		{
			name:    "空内容报错",
			req:     &pb.CreateCommentReq{PostId: 1000, UserId: 200, Content: ""},
			wantErr: true,
			errCode: errx.ContentEmpty,
		},
		{
			name: "帖子不存在报错",
			req:  &pb.CreateCommentReq{PostId: 9999, UserId: 200, Content: "评论"},
			setupMock: func(pm *MockPostModel, cm *MockCommentModel) {
				pm.On("FindPostById", mock.Anything, int64(9999)).Return(nil, model.ErrNotFound)
			},
			wantErr: true,
			errCode: errx.ContentNotFound,
		},
		{
			name: "已删除帖子不能评论",
			req:  &pb.CreateCommentReq{PostId: 1001, UserId: 200, Content: "评论"},
			setupMock: func(pm *MockPostModel, cm *MockCommentModel) {
				pm.On("FindPostById", mock.Anything, int64(1001)).Return(deletedPost, nil)
			},
			wantErr: true,
			errCode: errx.PostAlreadyDeleted,
		},
		{
			name: "查询帖子数据库错误",
			req:  &pb.CreateCommentReq{PostId: 1000, UserId: 200, Content: "评论"},
			setupMock: func(pm *MockPostModel, cm *MockCommentModel) {
				pm.On("FindPostById", mock.Anything, int64(1000)).Return(nil, fmt.Errorf("db error"))
			},
			wantErr: true,
		},
		{
			name: "插入评论数据库错误",
			req:  &pb.CreateCommentReq{PostId: 1000, UserId: 200, Content: "评论"},
			setupMock: func(pm *MockPostModel, cm *MockCommentModel) {
				pm.On("FindPostById", mock.Anything, int64(1000)).Return(publishedPost, nil)
				cm.On("InsertComment", mock.Anything, mock.AnythingOfType("*model.Comment")).Return(fmt.Errorf("insert error"))
			},
			wantErr: true,
		},
		{
			name: "IncrCommentCount失败时仍返回成功（降级）",
			req:  &pb.CreateCommentReq{PostId: 1000, UserId: 200, Content: "评论"},
			setupMock: func(pm *MockPostModel, cm *MockCommentModel) {
				pm.On("FindPostById", mock.Anything, int64(1000)).Return(publishedPost, nil)
				cm.On("InsertComment", mock.Anything, mock.AnythingOfType("*model.Comment")).Return(nil)
				pm.On("IncrCommentCount", mock.Anything, int64(1000)).Return(fmt.Errorf("redis error"))
			},
			check: func(t *testing.T, resp *pb.CreateCommentResp) {
				assert.Greater(t, resp.CommentId, int64(0))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pm := new(MockPostModel)
			cm := new(MockCommentModel)
			if tt.setupMock != nil {
				tt.setupMock(pm, cm)
			}
			svcCtx := newUnitSvcCtx(pm, cm, nil, nil)
			l := NewCreateCommentLogic(context.Background(), svcCtx)

			resp, err := l.CreateComment(tt.req)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errCode != 0 {
					assert.True(t, errx.Is(err, tt.errCode), "期望错误码 %d，实际: %v", tt.errCode, err)
				}
				return
			}
			require.NoError(t, err)
			if tt.check != nil {
				tt.check(t, resp)
			}
			pm.AssertExpectations(t)
			cm.AssertExpectations(t)
		})
	}
}

// ─── DeleteComment ────────────────────────────────────────────────────────────

func TestDeleteCommentLogic(t *testing.T) {
	activeComment := &model.Comment{Id: 2000, PostId: 1000, UserId: 300, Status: 1}
	deletedComment := &model.Comment{Id: 2001, PostId: 1000, UserId: 300, Status: 0}

	tests := []struct {
		name      string
		req       *pb.DeleteCommentReq
		setupMock func(*MockPostModel, *MockCommentModel)
		wantErr   bool
		errCode   int
	}{
		{
			name: "成功删除评论",
			req:  &pb.DeleteCommentReq{CommentId: 2000, UserId: 300},
			setupMock: func(pm *MockPostModel, cm *MockCommentModel) {
				cm.On("FindCommentById", mock.Anything, int64(2000)).Return(activeComment, nil)
				cm.On("UpdateStatus", mock.Anything, int64(2000), int64(0)).Return(nil)
				pm.On("DecrCommentCount", mock.Anything, int64(1000)).Return(nil)
			},
		},
		{
			name: "重复删除幂等",
			req:  &pb.DeleteCommentReq{CommentId: 2001, UserId: 300},
			setupMock: func(pm *MockPostModel, cm *MockCommentModel) {
				cm.On("FindCommentById", mock.Anything, int64(2001)).Return(deletedComment, nil)
				// 已删除，直接返回成功，不调用 UpdateStatus
			},
		},
		{
			name: "评论不存在报错",
			req:  &pb.DeleteCommentReq{CommentId: 9999, UserId: 300},
			setupMock: func(pm *MockPostModel, cm *MockCommentModel) {
				cm.On("FindCommentById", mock.Anything, int64(9999)).Return(nil, model.ErrNotFound)
			},
			wantErr: true,
			errCode: errx.ContentNotFound,
		},
		{
			name: "非作者删除报错",
			req:  &pb.DeleteCommentReq{CommentId: 2000, UserId: 9999},
			setupMock: func(pm *MockPostModel, cm *MockCommentModel) {
				cm.On("FindCommentById", mock.Anything, int64(2000)).Return(activeComment, nil)
			},
			wantErr: true,
			errCode: errx.ContentForbidden,
		},
		{
			name: "查询评论数据库错误",
			req:  &pb.DeleteCommentReq{CommentId: 2000, UserId: 300},
			setupMock: func(pm *MockPostModel, cm *MockCommentModel) {
				cm.On("FindCommentById", mock.Anything, int64(2000)).Return(nil, fmt.Errorf("timeout"))
			},
			wantErr: true,
		},
		{
			name: "UpdateStatus失败报错",
			req:  &pb.DeleteCommentReq{CommentId: 2000, UserId: 300},
			setupMock: func(pm *MockPostModel, cm *MockCommentModel) {
				cm.On("FindCommentById", mock.Anything, int64(2000)).Return(activeComment, nil)
				cm.On("UpdateStatus", mock.Anything, int64(2000), int64(0)).Return(fmt.Errorf("db error"))
			},
			wantErr: true,
		},
		{
			name: "DecrCommentCount失败时仍返回成功（降级）",
			req:  &pb.DeleteCommentReq{CommentId: 2000, UserId: 300},
			setupMock: func(pm *MockPostModel, cm *MockCommentModel) {
				cm.On("FindCommentById", mock.Anything, int64(2000)).Return(activeComment, nil)
				cm.On("UpdateStatus", mock.Anything, int64(2000), int64(0)).Return(nil)
				pm.On("DecrCommentCount", mock.Anything, int64(1000)).Return(fmt.Errorf("redis error"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pm := new(MockPostModel)
			cm := new(MockCommentModel)
			if tt.setupMock != nil {
				tt.setupMock(pm, cm)
			}
			svcCtx := newUnitSvcCtx(pm, cm, nil, nil)
			l := NewDeleteCommentLogic(context.Background(), svcCtx)

			_, err := l.DeleteComment(tt.req)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errCode != 0 {
					assert.True(t, errx.Is(err, tt.errCode), "期望错误码 %d，实际: %v", tt.errCode, err)
				}
				return
			}
			require.NoError(t, err)
			pm.AssertExpectations(t)
			cm.AssertExpectations(t)
		})
	}
}

// ─── GetCommentList ───────────────────────────────────────────────────────────

func TestGetCommentListLogic(t *testing.T) {
	comments := []*model.Comment{
		{Id: 3000, PostId: 1000, UserId: 400, Content: "评论1", Status: 1},
		{Id: 3001, PostId: 1000, UserId: 401, Content: "评论2", Status: 1},
	}

	tests := []struct {
		name      string
		req       *pb.GetCommentListReq
		setupMock func(*MockCommentModel)
		wantErr   bool
		check     func(t *testing.T, resp *pb.GetCommentListResp)
	}{
		{
			name: "成功获取评论列表",
			req:  &pb.GetCommentListReq{PostId: 1000, Page: 1, PageSize: 10},
			setupMock: func(cm *MockCommentModel) {
				cm.On("FindByPostId", mock.Anything, int64(1000), 1, 10, 0).Return(comments, int64(2), nil)
			},
			check: func(t *testing.T, resp *pb.GetCommentListResp) {
				assert.Len(t, resp.Comments, 2)
				assert.Equal(t, int64(2), resp.Total)
			},
		},
		{
			name: "页码/页大小默认值修正",
			req:  &pb.GetCommentListReq{PostId: 1000, Page: 0, PageSize: 0},
			setupMock: func(cm *MockCommentModel) {
				cm.On("FindByPostId", mock.Anything, int64(1000), 1, 20, 0).Return([]*model.Comment{}, int64(0), nil)
			},
			check: func(t *testing.T, resp *pb.GetCommentListResp) {
				assert.Len(t, resp.Comments, 0)
			},
		},
		{
			name: "页大小超限修正为20",
			req:  &pb.GetCommentListReq{PostId: 1000, Page: 1, PageSize: 200},
			setupMock: func(cm *MockCommentModel) {
				cm.On("FindByPostId", mock.Anything, int64(1000), 1, 20, 0).Return([]*model.Comment{}, int64(0), nil)
			},
			check: func(t *testing.T, resp *pb.GetCommentListResp) {
				assert.Len(t, resp.Comments, 0)
			},
		},
		{
			name: "数据库错误",
			req:  &pb.GetCommentListReq{PostId: 1000, Page: 1, PageSize: 10},
			setupMock: func(cm *MockCommentModel) {
				cm.On("FindByPostId", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return([]*model.Comment{}, int64(0), fmt.Errorf("db error"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cm := new(MockCommentModel)
			if tt.setupMock != nil {
				tt.setupMock(cm)
			}
			svcCtx := newUnitSvcCtx(nil, cm, nil, nil)
			l := NewGetCommentListLogic(context.Background(), svcCtx)

			resp, err := l.GetCommentList(tt.req)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			if tt.check != nil {
				tt.check(t, resp)
			}
			cm.AssertExpectations(t)
		})
	}
}
