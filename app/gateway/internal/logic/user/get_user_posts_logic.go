// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package user

import (
	"context"

	"esx/app/content/rpc/contentservice"
	"esx/app/gateway/internal/logic/authorx"
	"esx/app/gateway/internal/logic/postmap"
	"esx/app/gateway/internal/logic/rpcx"
	"esx/app/gateway/internal/logic/viewerstate"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/pkg/jwtx"
	"esx/pkg/pageutil"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetUserPostsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 获取用户发布的帖子列表（公开接口）
func NewGetUserPostsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetUserPostsLogic {
	return &GetUserPostsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetUserPostsLogic) GetUserPosts(req *types.GetUserPostsReq) (*types.GetPostListResp, error) {
	pageSize := pageutil.ClampPageSize(req.PageSize)
	result, err := l.svcCtx.ContentService.GetUserPosts(l.ctx, &contentservice.GetUserPostsReq{
		UserId:   req.UserId,
		PageSize: pageSize,
		SortBy:   req.SortBy,
		Cursor:   req.Cursor,
	})
	if err != nil {
		return nil, rpcx.Error(l.Logger, "ContentService.GetUserPosts", err, logx.Field("userId", req.UserId))
	}

	postIDs := make([]int64, 0, len(result.Posts))
	for _, post := range result.Posts {
		if post != nil && post.Id > 0 {
			postIDs = append(postIDs, post.Id)
		}
	}
	viewerID, _ := jwtx.GetOptionalUserIdFromContext(l.ctx)
	liked, favorited, err := viewerstate.Enrich(l.ctx, l.svcCtx, viewerID, postIDs)
	if err != nil {
		l.Errorw("viewerstate.Enrich failed", logx.Field("err", err.Error()))
		return nil, err
	}
	authors := authorx.LoadSoft(l.ctx, l.svcCtx, authorx.PostAuthorIDs(result.Posts))

	return &types.GetPostListResp{
		List:       postmap.Items(result.Posts, liked, favorited, authors),
		NextCursor: result.NextCursor,
	}, nil
}
