package logic

import (
	"context"
	model2 "esx/app/content/rpc/internal/model"
	"esx/app/content/rpc/pb/xiaobaihe/content/pb"
	"fmt"
	"testing"

	"errx"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// ─── GetTags ──────────────────────────────────────────────────────────────────

func TestGetTagsLogic(t *testing.T) {
	tags := []*model2.Tag{
		{Id: 1, Name: "golang", PostCount: 100},
		{Id: 2, Name: "python", PostCount: 80},
		{Id: 3, Name: "rust", PostCount: 50},
	}

	tests := []struct {
		name      string
		req       *pb.GetTagsReq
		setupMock func(*MockTagModel)
		wantErr   bool
		check     func(t *testing.T, resp *pb.GetTagsResp)
	}{
		{
			name: "默认Limit获取标签列表",
			req:  &pb.GetTagsReq{Limit: 0},
			setupMock: func(tm *MockTagModel) {
				// Limit=0 被修正为 20
				tm.On("FindList", mock.Anything, 20).Return(tags, nil)
			},
			check: func(t *testing.T, resp *pb.GetTagsResp) {
				assert.Len(t, resp.Tags, 3)
				assert.Equal(t, "golang", resp.Tags[0].Name)
				assert.Equal(t, int64(100), resp.Tags[0].PostCount)
			},
		},
		{
			name: "自定义Limit截断结果",
			req:  &pb.GetTagsReq{Limit: 2},
			setupMock: func(tm *MockTagModel) {
				tm.On("FindList", mock.Anything, 2).Return(tags[:2], nil)
			},
			check: func(t *testing.T, resp *pb.GetTagsResp) {
				assert.Len(t, resp.Tags, 2)
			},
		},
		{
			name: "数据库错误",
			req:  &pb.GetTagsReq{Limit: 10},
			setupMock: func(tm *MockTagModel) {
				tm.On("FindList", mock.Anything, 10).Return([]*model2.Tag{}, fmt.Errorf("db error"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tm := new(MockTagModel)
			if tt.setupMock != nil {
				tt.setupMock(tm)
			}
			svcCtx := newUnitSvcCtx(nil, nil, tm, nil)
			l := NewGetTagsLogic(context.Background(), svcCtx)

			resp, err := l.GetTags(tt.req)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			if tt.check != nil {
				tt.check(t, resp)
			}
			tm.AssertExpectations(t)
		})
	}
}

// ─── GetPostsByTag ────────────────────────────────────────────────────────────

func TestGetPostsByTagLogic(t *testing.T) {
	postIds := []int64{100, 101}
	posts := []*model2.Post{
		{Id: 100, AuthorId: 1, Title: "标签帖子1", Content: "内容1", Status: 1},
		{Id: 101, AuthorId: 2, Title: "标签帖子2", Content: "内容2", Status: 1},
	}

	tests := []struct {
		name      string
		req       *pb.GetPostsByTagReq
		setupMock func(*MockPostModel, *MockPostTagModel)
		wantErr   bool
		errCode   int
		check     func(t *testing.T, resp *pb.GetPostsByTagResp)
	}{
		{
			name: "成功按标签获取帖子",
			req:  &pb.GetPostsByTagReq{TagName: "golang", Page: 1, PageSize: 10},
			setupMock: func(pm *MockPostModel, ptm *MockPostTagModel) {
				ptm.On("FindPostIdsByTagName", mock.Anything, "golang", 1, 10).Return(postIds, int64(2), nil)
				pm.On("FindByIds", mock.Anything, postIds).Return(posts, nil)
				ptm.On("FindTagNamesByPostIds", mock.Anything, postIds).Return(
					map[int64][]string{100: {"golang"}, 101: {"golang"}}, nil,
				)
			},
			check: func(t *testing.T, resp *pb.GetPostsByTagResp) {
				assert.Len(t, resp.Posts, 2)
				assert.Equal(t, int64(2), resp.Total)
				for _, p := range resp.Posts {
					assert.Contains(t, p.Tags, "golang")
				}
			},
		},
		{
			name:    "空标签名报错",
			req:     &pb.GetPostsByTagReq{TagName: ""},
			wantErr: true,
			errCode: errx.ParamError,
		},
		{
			name: "无匹配帖子返回空列表",
			req:  &pb.GetPostsByTagReq{TagName: "nonexistent", Page: 1, PageSize: 10},
			setupMock: func(pm *MockPostModel, ptm *MockPostTagModel) {
				ptm.On("FindPostIdsByTagName", mock.Anything, "nonexistent", 1, 10).Return([]int64{}, int64(0), nil)
				pm.On("FindByIds", mock.Anything, []int64{}).Return([]*model2.Post{}, nil)
			},
			check: func(t *testing.T, resp *pb.GetPostsByTagResp) {
				assert.Len(t, resp.Posts, 0)
				assert.Equal(t, int64(0), resp.Total)
			},
		},
		{
			name: "查询帖子ID数据库错误",
			req:  &pb.GetPostsByTagReq{TagName: "golang", Page: 1, PageSize: 10},
			setupMock: func(pm *MockPostModel, ptm *MockPostTagModel) {
				ptm.On("FindPostIdsByTagName", mock.Anything, "golang", 1, 10).Return([]int64{}, int64(0), fmt.Errorf("db error"))
			},
			wantErr: true,
		},
		{
			name: "批量查询帖子数据库错误",
			req:  &pb.GetPostsByTagReq{TagName: "golang", Page: 1, PageSize: 10},
			setupMock: func(pm *MockPostModel, ptm *MockPostTagModel) {
				ptm.On("FindPostIdsByTagName", mock.Anything, "golang", 1, 10).Return(postIds, int64(2), nil)
				pm.On("FindByIds", mock.Anything, postIds).Return([]*model2.Post{}, fmt.Errorf("db error"))
			},
			wantErr: true,
		},
		{
			name: "页码/页大小默认值修正",
			req:  &pb.GetPostsByTagReq{TagName: "golang", Page: 0, PageSize: 0},
			setupMock: func(pm *MockPostModel, ptm *MockPostTagModel) {
				ptm.On("FindPostIdsByTagName", mock.Anything, "golang", 1, 20).Return([]int64{}, int64(0), nil)
				pm.On("FindByIds", mock.Anything, []int64{}).Return([]*model2.Post{}, nil)
			},
			check: func(t *testing.T, resp *pb.GetPostsByTagResp) {
				assert.Len(t, resp.Posts, 0)
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
			l := NewGetPostsByTagLogic(context.Background(), svcCtx)

			resp, err := l.GetPostsByTag(tt.req)

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
