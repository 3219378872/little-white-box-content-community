package logic

import (
	"context"
	"fmt"
	"testing"

	"errx"
	"esx/app/content/internal/model"
	"esx/app/content/internal/svc"
	"esx/app/content/pb/xiaobaihe/content/pb"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// ─── CreatePost ───────────────────────────────────────────────────────────────

func TestCreatePostLogic(t *testing.T) {
	tests := []struct {
		name      string
		req       *pb.CreatePostReq
		setupMock func(*MockPostModel, *MockPostTagModel)
		wantErr   bool
		errCode   int
		check     func(t *testing.T, resp *pb.CreatePostResp)
	}{
		{
			name: "成功创建帖子（无标签）",
			req: &pb.CreatePostReq{
				AuthorId: 1001,
				Title:    "标题",
				Content:  "内容",
				Status:   1,
			},
			setupMock: func(pm *MockPostModel, ptm *MockPostTagModel) {
				pm.On("InsertPost", mock.Anything, mock.AnythingOfType("*model.Post")).Return(nil)
				// 无标签时仍调用 BatchInsertTagsByPostId（传入空切片，实现内部短路返回 nil）
				ptm.On("BatchInsertTagsByPostId", mock.Anything, mock.Anything, mock.AnythingOfType("int64"), mock.Anything, mock.Anything).Return(nil)
			},
			check: func(t *testing.T, resp *pb.CreatePostResp) {
				assert.Greater(t, resp.PostId, int64(0))
			},
		},
		{
			name: "成功创建帖子（含标签）",
			req: &pb.CreatePostReq{
				AuthorId: 1001,
				Title:    "标题",
				Content:  "内容",
				Tags:     []string{"golang", "go-zero"},
				Status:   1,
			},
			setupMock: func(pm *MockPostModel, ptm *MockPostTagModel) {
				pm.On("InsertPost", mock.Anything, mock.AnythingOfType("*model.Post")).Return(nil)
				ptm.On("BatchInsertTagsByPostId", mock.Anything, mock.Anything, mock.AnythingOfType("int64"), mock.Anything, mock.Anything).Return(nil)
			},
			check: func(t *testing.T, resp *pb.CreatePostResp) {
				assert.Greater(t, resp.PostId, int64(0))
			},
		},
		{
			name:    "空标题报错",
			req:     &pb.CreatePostReq{AuthorId: 1001, Title: "", Content: "内容"},
			wantErr: true,
			errCode: errx.TitleEmpty,
		},
		{
			name:    "空内容报错",
			req:     &pb.CreatePostReq{AuthorId: 1001, Title: "标题", Content: ""},
			wantErr: true,
			errCode: errx.ContentEmpty,
		},
		{
			name: "图片URL含逗号报错",
			req: &pb.CreatePostReq{
				AuthorId: 1001,
				Title:    "标题",
				Content:  "内容",
				Images:   []string{"http://example.com/a,b.jpg"},
			},
			wantErr: true,
			errCode: errx.ParamError,
		},
		{
			name: "数据库Insert失败",
			req:  &pb.CreatePostReq{AuthorId: 1001, Title: "标题", Content: "内容"},
			setupMock: func(pm *MockPostModel, ptm *MockPostTagModel) {
				pm.On("InsertPost", mock.Anything, mock.AnythingOfType("*model.Post")).Return(fmt.Errorf("connection refused"))
			},
			wantErr: true,
		},
		{
			name: "BatchInsertTagsByPostId失败",
			req: &pb.CreatePostReq{
				AuthorId: 1001,
				Title:    "标题",
				Content:  "内容",
				Tags:     []string{"golang"},
			},
			setupMock: func(pm *MockPostModel, ptm *MockPostTagModel) {
				pm.On("InsertPost", mock.Anything, mock.AnythingOfType("*model.Post")).Return(nil)
				ptm.On("BatchInsertTagsByPostId", mock.Anything, mock.Anything, mock.AnythingOfType("int64"), mock.Anything, mock.Anything).Return(fmt.Errorf("db error"))
			},
			wantErr: true,
		},
		{
			name: "空标签被过滤",
			req: &pb.CreatePostReq{
				AuthorId: 1001,
				Title:    "标题",
				Content:  "内容",
				Tags:     []string{"", "golang", ""},
			},
			setupMock: func(pm *MockPostModel, ptm *MockPostTagModel) {
				pm.On("InsertPost", mock.Anything, mock.AnythingOfType("*model.Post")).Return(nil)
				ptm.On("BatchInsertTagsByPostId", mock.Anything, mock.Anything, mock.AnythingOfType("int64"), mock.Anything, mock.Anything).Return(nil)
			},
			check: func(t *testing.T, resp *pb.CreatePostResp) {
				assert.Greater(t, resp.PostId, int64(0))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pm := new(MockPostModel)
			ptm := new(MockPostTagModel)
			if tt.setupMock != nil {
				tt.setupMock(pm, ptm)
			}
			svcCtx := newUnitSvcCtx(pm, nil, nil, ptm)
			l := NewCreatePostLogic(context.Background(), svcCtx)

			resp, err := l.CreatePost(tt.req)

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
			ptm.AssertExpectations(t)
		})
	}
}

// ─── GetPost ──────────────────────────────────────────────────────────────────

func TestGetPostLogic(t *testing.T) {
	publishedPost := &model.Post{Id: 100, AuthorId: 200, Title: "标题", Content: "内容", Status: 1}
	deletedPost := &model.Post{Id: 101, AuthorId: 200, Status: 2}
	draftPost := &model.Post{Id: 102, AuthorId: 200, Status: 0}

	tests := []struct {
		name      string
		req       *pb.GetPostReq
		setupMock func(*MockPostModel, *MockPostTagModel)
		wantErr   bool
		errCode   int
		check     func(t *testing.T, resp *pb.GetPostResp)
	}{
		{
			name: "成功获取已发布帖子",
			req:  &pb.GetPostReq{PostId: 100},
			setupMock: func(pm *MockPostModel, ptm *MockPostTagModel) {
				pm.On("FindPostById", mock.Anything, int64(100)).Return(publishedPost, nil)
				ptm.On("FindTagNamesByPostId", mock.Anything, int64(100)).Return([]string{"go"}, nil)
			},
			check: func(t *testing.T, resp *pb.GetPostResp) {
				assert.Equal(t, int64(100), resp.Post.Id)
				assert.Equal(t, "标题", resp.Post.Title)
				assert.Equal(t, []string{"go"}, resp.Post.Tags)
			},
		},
		{
			name: "帖子不存在报错",
			req:  &pb.GetPostReq{PostId: 999},
			setupMock: func(pm *MockPostModel, ptm *MockPostTagModel) {
				pm.On("FindPostById", mock.Anything, int64(999)).Return(nil, model.ErrNotFound)
			},
			wantErr: true,
			errCode: errx.ContentNotFound,
		},
		{
			name: "已删除帖子报错",
			req:  &pb.GetPostReq{PostId: 101},
			setupMock: func(pm *MockPostModel, ptm *MockPostTagModel) {
				pm.On("FindPostById", mock.Anything, int64(101)).Return(deletedPost, nil)
			},
			wantErr: true,
			errCode: errx.PostAlreadyDeleted,
		},
		{
			name: "草稿帖子不公开",
			req:  &pb.GetPostReq{PostId: 102},
			setupMock: func(pm *MockPostModel, ptm *MockPostTagModel) {
				pm.On("FindPostById", mock.Anything, int64(102)).Return(draftPost, nil)
			},
			wantErr: true,
			errCode: errx.ContentNotFound,
		},
		{
			name: "数据库错误",
			req:  &pb.GetPostReq{PostId: 200},
			setupMock: func(pm *MockPostModel, ptm *MockPostTagModel) {
				pm.On("FindPostById", mock.Anything, int64(200)).Return(nil, fmt.Errorf("timeout"))
			},
			wantErr: true,
		},
		{
			name: "查询标签失败时返回空标签（降级）",
			req:  &pb.GetPostReq{PostId: 100},
			setupMock: func(pm *MockPostModel, ptm *MockPostTagModel) {
				pm.On("FindPostById", mock.Anything, int64(100)).Return(publishedPost, nil)
				ptm.On("FindTagNamesByPostId", mock.Anything, int64(100)).Return([]string{}, fmt.Errorf("redis down"))
			},
			check: func(t *testing.T, resp *pb.GetPostResp) {
				assert.Equal(t, int64(100), resp.Post.Id)
				assert.Equal(t, []string{}, resp.Post.Tags)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pm := new(MockPostModel)
			ptm := new(MockPostTagModel)
			if tt.setupMock != nil {
				tt.setupMock(pm, ptm)
			}
			svcCtx := newUnitSvcCtx(pm, nil, nil, ptm)
			l := NewGetPostLogic(context.Background(), svcCtx)

			resp, err := l.GetPost(tt.req)

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
			ptm.AssertExpectations(t)
		})
	}
}

// ─── UpdatePost ───────────────────────────────────────────────────────────────

func TestUpdatePostLogic(t *testing.T) {
	authorPost := &model.Post{Id: 300, AuthorId: 3001, Title: "原标题", Content: "原内容", Status: 1}
	deletedPost := &model.Post{Id: 301, AuthorId: 3001, Status: 2}

	tests := []struct {
		name      string
		req       *pb.UpdatePostReq
		setupMock func(*MockPostModel, *MockPostTagModel)
		wantErr   bool
		errCode   int
	}{
		{
			name: "成功更新帖子",
			req: &pb.UpdatePostReq{
				PostId:   300,
				AuthorId: 3001,
				Title:    "新标题",
				Content:  "新内容",
				Tags:     []string{"golang"},
				Status:   1,
			},
			setupMock: func(pm *MockPostModel, ptm *MockPostTagModel) {
				pm.On("FindPostById", mock.Anything, int64(300)).Return(authorPost, nil)
				pm.On("UpdateFields", mock.Anything, int64(300), mock.Anything).Return(nil)
				ptm.On("TransactReplaceTagsByPostId", mock.Anything, mock.Anything, int64(300), mock.Anything, mock.Anything).Return(nil)
			},
		},
		{
			name: "帖子不存在报错",
			req:  &pb.UpdatePostReq{PostId: 999, AuthorId: 3001, Title: "t", Content: "c"},
			setupMock: func(pm *MockPostModel, ptm *MockPostTagModel) {
				pm.On("FindPostById", mock.Anything, int64(999)).Return(nil, model.ErrNotFound)
			},
			wantErr: true,
			errCode: errx.ContentNotFound,
		},
		{
			name: "非作者操作报错",
			req:  &pb.UpdatePostReq{PostId: 300, AuthorId: 9999, Title: "t", Content: "c"},
			setupMock: func(pm *MockPostModel, ptm *MockPostTagModel) {
				pm.On("FindPostById", mock.Anything, int64(300)).Return(authorPost, nil)
			},
			wantErr: true,
			errCode: errx.ContentForbidden,
		},
		{
			name: "已删除帖子报错",
			req:  &pb.UpdatePostReq{PostId: 301, AuthorId: 3001, Title: "t", Content: "c"},
			setupMock: func(pm *MockPostModel, ptm *MockPostTagModel) {
				pm.On("FindPostById", mock.Anything, int64(301)).Return(deletedPost, nil)
			},
			wantErr: true,
			errCode: errx.PostAlreadyDeleted,
		},
		{
			name: "图片URL含逗号报错",
			req: &pb.UpdatePostReq{
				PostId: 300, AuthorId: 3001, Title: "t", Content: "c",
				Images: []string{"http://example.com/a,b.jpg"},
			},
			setupMock: func(pm *MockPostModel, ptm *MockPostTagModel) {
				pm.On("FindPostById", mock.Anything, int64(300)).Return(authorPost, nil)
			},
			wantErr: true,
			errCode: errx.ParamError,
		},
		{
			name: "UpdateFields数据库错误",
			req:  &pb.UpdatePostReq{PostId: 300, AuthorId: 3001, Title: "t", Content: "c"},
			setupMock: func(pm *MockPostModel, ptm *MockPostTagModel) {
				pm.On("FindPostById", mock.Anything, int64(300)).Return(authorPost, nil)
				pm.On("UpdateFields", mock.Anything, mock.Anything, mock.Anything).Return(fmt.Errorf("db error"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pm := new(MockPostModel)
			ptm := new(MockPostTagModel)
			if tt.setupMock != nil {
				tt.setupMock(pm, ptm)
			}
			svcCtx := newUnitSvcCtx(pm, nil, nil, ptm)
			l := NewUpdatePostLogic(context.Background(), svcCtx)

			_, err := l.UpdatePost(tt.req)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errCode != 0 {
					assert.True(t, errx.Is(err, tt.errCode), "期望错误码 %d，实际: %v", tt.errCode, err)
				}
				return
			}
			require.NoError(t, err)
			pm.AssertExpectations(t)
			ptm.AssertExpectations(t)
		})
	}
}

// ─── DeletePost ───────────────────────────────────────────────────────────────

func TestDeletePostLogic(t *testing.T) {
	activePost := &model.Post{Id: 400, AuthorId: 4001, Status: 1}
	deletedPost := &model.Post{Id: 401, AuthorId: 4001, Status: 2}

	tests := []struct {
		name      string
		req       *pb.DeletePostReq
		setupMock func(*MockPostModel)
		wantErr   bool
		errCode   int
	}{
		{
			name: "成功软删除帖子",
			req:  &pb.DeletePostReq{PostId: 400, AuthorId: 4001},
			setupMock: func(pm *MockPostModel) {
				pm.On("FindPostById", mock.Anything, int64(400)).Return(activePost, nil)
				pm.On("UpdateStatus", mock.Anything, int64(400), int64(2)).Return(nil)
			},
		},
		{
			name: "帖子不存在报错",
			req:  &pb.DeletePostReq{PostId: 999, AuthorId: 4001},
			setupMock: func(pm *MockPostModel) {
				pm.On("FindPostById", mock.Anything, int64(999)).Return(nil, model.ErrNotFound)
			},
			wantErr: true,
			errCode: errx.ContentNotFound,
		},
		{
			name: "已删除帖子报错",
			req:  &pb.DeletePostReq{PostId: 401, AuthorId: 4001},
			setupMock: func(pm *MockPostModel) {
				pm.On("FindPostById", mock.Anything, int64(401)).Return(deletedPost, nil)
			},
			wantErr: true,
			errCode: errx.PostAlreadyDeleted,
		},
		{
			name: "非作者删除报错",
			req:  &pb.DeletePostReq{PostId: 400, AuthorId: 9999},
			setupMock: func(pm *MockPostModel) {
				pm.On("FindPostById", mock.Anything, int64(400)).Return(activePost, nil)
			},
			wantErr: true,
			errCode: errx.ContentForbidden,
		},
		{
			name: "数据库查询错误",
			req:  &pb.DeletePostReq{PostId: 400, AuthorId: 4001},
			setupMock: func(pm *MockPostModel) {
				pm.On("FindPostById", mock.Anything, int64(400)).Return(nil, fmt.Errorf("timeout"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pm := new(MockPostModel)
			if tt.setupMock != nil {
				tt.setupMock(pm)
			}
			svcCtx := newUnitSvcCtx(pm, nil, nil, nil)
			l := NewDeletePostLogic(context.Background(), svcCtx)

			_, err := l.DeletePost(tt.req)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errCode != 0 {
					assert.True(t, errx.Is(err, tt.errCode), "期望错误码 %d，实际: %v", tt.errCode, err)
				}
				return
			}
			require.NoError(t, err)
			pm.AssertExpectations(t)
		})
	}
}

// ─── GetPostList ──────────────────────────────────────────────────────────────

func TestGetPostListLogic(t *testing.T) {
	posts := []*model.Post{
		{Id: 1, AuthorId: 100, Title: "帖子1", Content: "内容1", Status: 1},
		{Id: 2, AuthorId: 101, Title: "帖子2", Content: "内容2", Status: 1},
	}

	tests := []struct {
		name      string
		req       *pb.GetPostListReq
		setupMock func(*MockPostModel, *MockPostTagModel)
		wantErr   bool
		check     func(t *testing.T, resp *pb.GetPostListResp)
	}{
		{
			name: "正常获取帖子列表",
			req:  &pb.GetPostListReq{Page: 1, PageSize: 10, SortBy: 1},
			setupMock: func(pm *MockPostModel, ptm *MockPostTagModel) {
				pm.On("FindList", mock.Anything, 1, 10, 1).Return(posts, int64(2), nil)
				ptm.On("FindTagNamesByPostIds", mock.Anything, mock.Anything).Return(map[int64][]string{1: {"go"}, 2: {}}, nil)
			},
			check: func(t *testing.T, resp *pb.GetPostListResp) {
				assert.Len(t, resp.Posts, 2)
				assert.Equal(t, int64(2), resp.Total)
				assert.Equal(t, []string{"go"}, resp.Posts[0].Tags)
			},
		},
		{
			name: "页码/页大小默认值修正",
			req:  &pb.GetPostListReq{Page: 0, PageSize: 0},
			setupMock: func(pm *MockPostModel, ptm *MockPostTagModel) {
				pm.On("FindList", mock.Anything, 1, 20, 0).Return([]*model.Post{}, int64(0), nil)
			},
			check: func(t *testing.T, resp *pb.GetPostListResp) {
				assert.Len(t, resp.Posts, 0)
			},
		},
		{
			name: "页大小超限修正为20",
			req:  &pb.GetPostListReq{Page: 1, PageSize: 100},
			setupMock: func(pm *MockPostModel, ptm *MockPostTagModel) {
				pm.On("FindList", mock.Anything, 1, 20, 0).Return([]*model.Post{}, int64(0), nil)
			},
			check: func(t *testing.T, resp *pb.GetPostListResp) {
				assert.Len(t, resp.Posts, 0)
			},
		},
		{
			name: "数据库错误",
			req:  &pb.GetPostListReq{Page: 1, PageSize: 10},
			setupMock: func(pm *MockPostModel, ptm *MockPostTagModel) {
				pm.On("FindList", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return([]*model.Post{}, int64(0), fmt.Errorf("db error"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pm := new(MockPostModel)
			ptm := new(MockPostTagModel)
			if tt.setupMock != nil {
				tt.setupMock(pm, ptm)
			}
			svcCtx := newUnitSvcCtx(pm, nil, nil, ptm)
			l := NewGetPostListLogic(context.Background(), svcCtx)

			resp, err := l.GetPostList(tt.req)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			if tt.check != nil {
				tt.check(t, resp)
			}
			pm.AssertExpectations(t)
			ptm.AssertExpectations(t)
		})
	}
}

// ─── GetUserPosts ─────────────────────────────────────────────────────────────

func TestGetUserPostsLogic(t *testing.T) {
	userPosts := []*model.Post{
		{Id: 10, AuthorId: 5001, Title: "我的帖子", Content: "内容", Status: 1},
	}

	tests := []struct {
		name      string
		req       *pb.GetUserPostsReq
		setupMock func(*MockPostModel, *MockPostTagModel)
		wantErr   bool
		check     func(t *testing.T, resp *pb.GetUserPostsResp)
	}{
		{
			name: "成功获取用户帖子",
			req:  &pb.GetUserPostsReq{UserId: 5001, Page: 1, PageSize: 10},
			setupMock: func(pm *MockPostModel, ptm *MockPostTagModel) {
				pm.On("FindByAuthorId", mock.Anything, int64(5001), 1, 10, model.SortByLatest).Return(userPosts, int64(1), nil)
				ptm.On("FindTagNamesByPostIds", mock.Anything, mock.Anything).Return(map[int64][]string{10: {"go"}}, nil)
			},
			check: func(t *testing.T, resp *pb.GetUserPostsResp) {
				assert.Len(t, resp.Posts, 1)
				assert.Equal(t, int64(1), resp.Total)
				assert.Equal(t, int64(5001), resp.Posts[0].AuthorId)
			},
		},
		{
			name: "用户无帖子返回空列表",
			req:  &pb.GetUserPostsReq{UserId: 9999, Page: 1, PageSize: 10},
			setupMock: func(pm *MockPostModel, ptm *MockPostTagModel) {
				pm.On("FindByAuthorId", mock.Anything, int64(9999), 1, 10, model.SortByLatest).Return([]*model.Post{}, int64(0), nil)
			},
			check: func(t *testing.T, resp *pb.GetUserPostsResp) {
				assert.Len(t, resp.Posts, 0)
				assert.Equal(t, int64(0), resp.Total)
			},
		},
		{
			name: "数据库错误",
			req:  &pb.GetUserPostsReq{UserId: 5001, Page: 1, PageSize: 10},
			setupMock: func(pm *MockPostModel, ptm *MockPostTagModel) {
				pm.On("FindByAuthorId", mock.Anything, int64(5001), 1, 10, model.SortByLatest).Return([]*model.Post{}, int64(0), fmt.Errorf("db error"))
			},
			wantErr: true,
		},
		{
			name: "页大小超限修正为20",
			req:  &pb.GetUserPostsReq{UserId: 5001, Page: 1, PageSize: 100},
			setupMock: func(pm *MockPostModel, ptm *MockPostTagModel) {
				pm.On("FindByAuthorId", mock.Anything, int64(5001), 1, 20, model.SortByLatest).Return([]*model.Post{}, int64(0), nil)
			},
			check: func(t *testing.T, resp *pb.GetUserPostsResp) {
				assert.Len(t, resp.Posts, 0)
			},
		},
		{
			name: "查询标签失败时降级为空标签",
			req:  &pb.GetUserPostsReq{UserId: 5001, Page: 1, PageSize: 10},
			setupMock: func(pm *MockPostModel, ptm *MockPostTagModel) {
				pm.On("FindByAuthorId", mock.Anything, int64(5001), 1, 10, model.SortByLatest).Return(userPosts, int64(1), nil)
				ptm.On("FindTagNamesByPostIds", mock.Anything, mock.Anything).Return(map[int64][]string{}, fmt.Errorf("redis down"))
			},
			check: func(t *testing.T, resp *pb.GetUserPostsResp) {
				assert.Len(t, resp.Posts, 1)
				assert.Equal(t, []string{}, resp.Posts[0].Tags)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pm := new(MockPostModel)
			ptm := new(MockPostTagModel)
			if tt.setupMock != nil {
				tt.setupMock(pm, ptm)
			}
			svcCtx := newUnitSvcCtx(pm, nil, nil, ptm)
			l := NewGetUserPostsLogic(context.Background(), svcCtx)

			resp, err := l.GetUserPosts(tt.req)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			if tt.check != nil {
				tt.check(t, resp)
			}
			pm.AssertExpectations(t)
			ptm.AssertExpectations(t)
		})
	}
}

// ─── GetUserPosts ─────────────────────────────────────────────────────────────

func TestGetUserPostsLogic_SortBy(t *testing.T) {
	tests := []struct {
		name   string
		req    *pb.GetUserPostsReq
		sortBy int
	}{
		{"默认最新排序", &pb.GetUserPostsReq{UserId: 100, Page: 1, PageSize: 10, SortBy: 0}, 1},
		{"sortBy=1 最新", &pb.GetUserPostsReq{UserId: 100, Page: 1, PageSize: 10, SortBy: 1}, 1},
		{"sortBy=2 热门", &pb.GetUserPostsReq{UserId: 100, Page: 1, PageSize: 10, SortBy: 2}, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pm := new(MockPostModel)
			ptm := new(MockPostTagModel)
			pm.On("FindByAuthorId", mock.Anything, int64(100), 1, 10, tt.sortBy).
				Return([]*model.Post{}, int64(0), nil)
			svcCtx := &svc.ServiceContext{PostModel: pm, PostTagModel: ptm}
			l := NewGetUserPostsLogic(context.Background(), svcCtx)
			_, err := l.GetUserPosts(tt.req)
			require.NoError(t, err)
			pm.AssertExpectations(t)
		})
	}
}

// ─── GetPostsByIds ────────────────────────────────────────────────────────────

func TestGetPostsByIdsLogic(t *testing.T) {
	tests := []struct {
		name      string
		req       *pb.GetPostsByIdsReq
		setupMock func(*MockPostModel, *MockPostTagModel)
		wantLen   int
	}{
		{
			name: "空 ID 列表返回空结果",
			req:  &pb.GetPostsByIdsReq{PostIds: []int64{}},
			setupMock: func(pm *MockPostModel, _ *MockPostTagModel) {
				pm.On("FindByIds", mock.Anything, mock.Anything).Return([]*model.Post{}, nil).Maybe()
			},
			wantLen: 0,
		},
		{
			name: "过滤已删除帖子（status != 1）",
			req:  &pb.GetPostsByIdsReq{PostIds: []int64{1, 2, 3}},
			setupMock: func(pm *MockPostModel, ptm *MockPostTagModel) {
				pm.On("FindByIds", mock.Anything, []int64{1, 2, 3}).Return([]*model.Post{
					{Id: 1, AuthorId: 100, Title: "t1", Status: 1},
					{Id: 2, AuthorId: 100, Title: "t2", Status: 2},
					{Id: 3, AuthorId: 100, Title: "t3", Status: 1},
				}, nil)
				ptm.On("FindTagNamesByPostIds", mock.Anything, mock.Anything).Return(map[int64][]string{}, nil)
			},
			wantLen: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pm := new(MockPostModel)
			ptm := new(MockPostTagModel)
			if tt.setupMock != nil {
				tt.setupMock(pm, ptm)
			}
			svcCtx := &svc.ServiceContext{PostModel: pm, PostTagModel: ptm}
			l := NewGetPostsByIdsLogic(context.Background(), svcCtx)
			resp, err := l.GetPostsByIds(tt.req)
			require.NoError(t, err)
			assert.Len(t, resp.Posts, tt.wantLen)
		})
	}
}
