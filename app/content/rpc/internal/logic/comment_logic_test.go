package logic

import (
	"context"
	model2 "esx/app/content/rpc/internal/model"
	"esx/app/content/rpc/pb/xiaobaihe/content/pb"
	"esx/pkg/outboxx"
	"fmt"
	"strings"
	"testing"

	"errx"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type conflictCommentCommandModel struct {
	model2.CommentCommandModel
}

func (conflictCommentCommandModel) CreateComment(
	context.Context,
	*model2.Comment,
	outboxx.Event,
	model2.IdempotencyRecord,
) (int64, bool, error) {
	return 0, false, model2.ErrIdempotencyConflict
}

// ─── CreateComment ────────────────────────────────────────────────────────────

func TestCreateCommentLogic(t *testing.T) {
	publishedPost := &model2.Post{Id: 1000, AuthorId: 100, Status: 1}
	deletedPost := &model2.Post{Id: 1001, AuthorId: 100, Status: 2}

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
				pm.On("FindPostById", mock.Anything, int64(9999)).Return(nil, model2.ErrNotFound)
			},
			wantErr: true,
			errCode: errx.ContentNotFound,
		},
		{
			name: "已删除帖子不能评论（统一不存在）",
			req:  &pb.CreateCommentReq{PostId: 1001, UserId: 200, Content: "评论"},
			setupMock: func(pm *MockPostModel, cm *MockCommentModel) {
				pm.On("FindPostById", mock.Anything, int64(1001)).Return(deletedPost, nil)
			},
			wantErr: true,
			errCode: errx.ContentNotFound,
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
			name: "评论计数失败时事务整体失败",
			req:  &pb.CreateCommentReq{PostId: 1000, UserId: 200, Content: "评论"},
			setupMock: func(pm *MockPostModel, cm *MockCommentModel) {
				pm.On("FindPostById", mock.Anything, int64(1000)).Return(publishedPost, nil)
				cm.On("InsertComment", mock.Anything, mock.AnythingOfType("*model.Comment")).Return(nil)
				pm.On("IncrCommentCount", mock.Anything, int64(1000)).Return(fmt.Errorf("redis error"))
			},
			wantErr: true,
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

func TestCreateCommentLogicRejectsOversizedContent(t *testing.T) {
	pm := new(MockPostModel)
	pm.On("FindPostById", mock.Anything, int64(1000)).Return(&model2.Post{Id: 1000, AuthorId: 100, Status: 1}, nil)
	svcCtx := newUnitSvcCtx(pm, new(MockCommentModel), nil, nil)

	// CORE-022：评论正文上限 2,000 Unicode 字符；越界在写库前被拒绝。
	resp, err := NewCreateCommentLogic(context.Background(), svcCtx).CreateComment(&pb.CreateCommentReq{
		PostId: 1000, UserId: 200, Content: strings.Repeat("长", 2001),
	})
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.True(t, errx.Is(err, errx.ContentTooLong), "期望内容过长码，实际: %v", err)

	// 边界值 2,000 应通过（进入命令模型路径前已完成校验）。
	cm := new(MockCommentModel)
	cm.On("InsertComment", mock.Anything, mock.AnythingOfType("*model.Comment")).Return(nil)
	pm.On("IncrCommentCount", mock.Anything, int64(1000)).Return(nil)
	okSvcCtx := newUnitSvcCtx(pm, cm, nil, nil)
	okResp, err := NewCreateCommentLogic(context.Background(), okSvcCtx).CreateComment(&pb.CreateCommentReq{
		PostId: 1000, UserId: 200, Content: strings.Repeat("长", 2000),
	})
	require.NoError(t, err)
	require.NotNil(t, okResp)
}

func TestCreateCommentLogicMapsIdempotencyConflict(t *testing.T) {
	pm := new(MockPostModel)
	pm.On("FindPostById", mock.Anything, int64(1000)).Return(&model2.Post{Id: 1000, AuthorId: 100, Status: 1}, nil)
	svcCtx := newUnitSvcCtx(pm, new(MockCommentModel), nil, nil)
	svcCtx.CommentCommandModel = conflictCommentCommandModel{}

	resp, err := NewCreateCommentLogic(context.Background(), svcCtx).CreateComment(&pb.CreateCommentReq{
		PostId: 1000, UserId: 200, Content: "评论", IdempotencyKey: "key-1",
	})

	assert.Nil(t, resp)
	require.Error(t, err)
	assert.True(t, errx.Is(err, errx.IdempotencyConflict), "期望幂等冲突码，实际: %v", err)
}

// ─── DeleteComment ────────────────────────────────────────────────────────────

func TestDeleteCommentLogic(t *testing.T) {
	activeComment := &model2.Comment{Id: 2000, PostId: 1000, UserId: 300, Status: 1}
	deletedComment := &model2.Comment{Id: 2001, PostId: 1000, UserId: 300, Status: 0}

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
				cm.On("FindCommentById", mock.Anything, int64(9999)).Return(nil, model2.ErrNotFound)
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
			name: "删除评论计数失败时事务整体失败",
			req:  &pb.DeleteCommentReq{CommentId: 2000, UserId: 300},
			setupMock: func(pm *MockPostModel, cm *MockCommentModel) {
				cm.On("FindCommentById", mock.Anything, int64(2000)).Return(activeComment, nil)
				cm.On("UpdateStatus", mock.Anything, int64(2000), int64(0)).Return(nil)
				pm.On("DecrCommentCount", mock.Anything, int64(1000)).Return(fmt.Errorf("redis error"))
			},
			wantErr: true,
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
	comments := []*model2.Comment{
		{Id: 3000, PostId: 1000, UserId: 400, Content: "评论1", Status: 1},
		{Id: 3001, PostId: 1000, UserId: 401, Content: "评论2", Status: 1},
	}

	tests := []struct {
		name      string
		req       *pb.GetCommentListReq
		setupMock func(*MockPostModel, *MockCommentModel)
		wantErr   bool
		errCode   int
		check     func(t *testing.T, resp *pb.GetCommentListResp)
	}{
		{
			name: "成功获取评论列表",
			req:  &pb.GetCommentListReq{PostId: 1000, Page: 1, PageSize: 10},
			setupMock: func(pm *MockPostModel, cm *MockCommentModel) {
				pm.On("FindPostById", mock.Anything, int64(1000)).Return(&model2.Post{Id: 1000, Status: 1}, nil)
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
			setupMock: func(pm *MockPostModel, cm *MockCommentModel) {
				pm.On("FindPostById", mock.Anything, int64(1000)).Return(&model2.Post{Id: 1000, Status: 1}, nil)
				cm.On("FindByPostId", mock.Anything, int64(1000), 1, 20, 0).Return([]*model2.Comment{}, int64(0), nil)
			},
			check: func(t *testing.T, resp *pb.GetCommentListResp) {
				assert.Len(t, resp.Comments, 0)
			},
		},
		{
			name: "页大小超限修正为20",
			req:  &pb.GetCommentListReq{PostId: 1000, Page: 1, PageSize: 200},
			setupMock: func(pm *MockPostModel, cm *MockCommentModel) {
				pm.On("FindPostById", mock.Anything, int64(1000)).Return(&model2.Post{Id: 1000, Status: 1}, nil)
				cm.On("FindByPostId", mock.Anything, int64(1000), 1, 20, 0).Return([]*model2.Comment{}, int64(0), nil)
			},
			check: func(t *testing.T, resp *pb.GetCommentListResp) {
				assert.Len(t, resp.Comments, 0)
			},
		},
		{
			name: "数据库错误",
			req:  &pb.GetCommentListReq{PostId: 1000, Page: 1, PageSize: 10},
			setupMock: func(pm *MockPostModel, cm *MockCommentModel) {
				pm.On("FindPostById", mock.Anything, int64(1000)).Return(&model2.Post{Id: 1000, Status: 1}, nil)
				cm.On("FindByPostId", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return([]*model2.Comment{}, int64(0), fmt.Errorf("db error"))
			},
			wantErr: true,
		},
		{
			name: "已删除帖子的评论统一不存在",
			req:  &pb.GetCommentListReq{PostId: 1001, Page: 1, PageSize: 10},
			setupMock: func(pm *MockPostModel, cm *MockCommentModel) {
				pm.On("FindPostById", mock.Anything, int64(1001)).Return(&model2.Post{Id: 1001, Status: 2}, nil)
			},
			wantErr: true,
			errCode: errx.ContentNotFound,
		},
		{
			name: "草稿帖子的评论统一不存在",
			req:  &pb.GetCommentListReq{PostId: 1002, Page: 1, PageSize: 10},
			setupMock: func(pm *MockPostModel, cm *MockCommentModel) {
				pm.On("FindPostById", mock.Anything, int64(1002)).Return(&model2.Post{Id: 1002, Status: 0}, nil)
			},
			wantErr: true,
			errCode: errx.ContentNotFound,
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
			l := NewGetCommentListLogic(context.Background(), svcCtx)

			resp, err := l.GetCommentList(tt.req)

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
			cm.AssertExpectations(t)
		})
	}
}
