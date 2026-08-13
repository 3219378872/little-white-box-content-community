// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package posts

import (
	"context"

	"errx"
	"esx/app/content/rpc/contentservice"
	"jwtx"

	"gateway/internal/svc"
	"gateway/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetPostLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 获取帖子详情
func NewGetPostLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetPostLogic {
	return &GetPostLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetPostLogic) GetPost(req *types.GetPostReq) (resp *types.GetPostResp, err error) {
	// CORE：已发布帖子允许匿名读取；登录用户才回填互动状态。
	userId, _ := jwtx.GetOptionalUserIdFromContext(l.ctx)

	result, err := l.svcCtx.ContentService.GetPost(l.ctx, &contentservice.GetPostReq{
		PostId: req.PostId,
		UserId: userId,
	})
	if err != nil {
		l.Errorw("ContentService.GetPost RPC failed",
			logx.Field("postId", req.PostId),
			logx.Field("err", err.Error()),
		)
		return nil, errx.FromRPCError(err)
	}

	post := result.Post
	if post == nil {
		return nil, errx.NewWithCode(errx.ContentNotFound)
	}

	return &types.GetPostResp{
		Id:            post.Id,
		AuthorId:      post.AuthorId,
		Title:         post.Title,
		Content:       post.Content,
		Images:        post.Images,
		Tags:          post.Tags,
		Status:        post.Status,
		ViewCount:     post.ViewCount,
		LikeCount:     post.LikeCount,
		CommentCount:  post.CommentCount,
		FavoriteCount: post.FavoriteCount,
		IsLiked:       result.IsLiked,
		IsFavorited:   result.IsFavorited,
		Revision:      post.Revision,
		CreatedAt:     post.CreatedAt,
	}, nil
}
