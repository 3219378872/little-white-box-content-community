// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package posts

import (
	"context"
	"gateway/internal/logic/pageutil"

	"errx"
	"esx/app/content/rpc/contentservice"
	"gateway/internal/logic/viewerstate"
	"jwtx"

	"gateway/internal/svc"
	"gateway/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetPostListLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 获取帖子列表
func NewGetPostListLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetPostListLogic {
	return &GetPostListLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetPostListLogic) GetPostList(req *types.GetPostListReq) (resp *types.GetPostListResp, err error) {
	// 与内容 RPC 的 clamp 语义保持一致：回传实际使用的 pageSize。
	pageSize := pageutil.ClampPageSize(req.PageSize)
	rpcReq := &contentservice.GetPostListReq{
		Page:     req.Page,
		PageSize: pageSize,
		SortBy:   req.SortBy,
	}
	if userId, ok := jwtx.GetOptionalUserIdFromContext(l.ctx); ok {
		rpcReq.UserId = userId
	}

	result, err := l.svcCtx.ContentService.GetPostList(l.ctx, rpcReq)
	if err != nil {
		l.Errorw("ContentService.GetPostList RPC failed", logx.Field("err", err.Error()))
		return nil, errx.FromRPCError(err)
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

	list := make([]types.PostItem, 0, len(result.Posts))
	for _, post := range result.Posts {
		list = append(list, types.PostItem{
			Id:           post.Id,
			AuthorId:     post.AuthorId,
			Title:        post.Title,
			Content:      post.Content,
			Images:       post.Images,
			Tags:         post.Tags,
			Status:       post.Status,
			ViewCount:    post.ViewCount,
			LikeCount:    post.LikeCount,
			CommentCount: post.CommentCount,
			IsLiked:      liked[post.Id],
			IsFavorited:  favorited[post.Id],
			Revision:     post.Revision,
			CreatedAt:    post.CreatedAt,
		})
	}

	return &types.GetPostListResp{
		List:     list,
		Total:    result.Total,
		Page:     req.Page,
		PageSize: pageSize,
	}, nil
}
