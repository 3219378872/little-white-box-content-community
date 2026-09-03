// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package posts

import (
	"context"

	"esx/app/content/rpc/contentservice"
	"esx/app/gateway/internal/logic/authorx"
	"esx/app/gateway/internal/logic/postmap"
	"esx/app/gateway/internal/logic/rpcx"
	"esx/app/gateway/internal/logic/viewerstate"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/pkg/errx"
	"esx/pkg/jwtx"

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
	userId, _ := jwtx.GetOptionalUserIdFromContext(l.ctx)

	result, err := l.svcCtx.ContentService.GetPost(l.ctx, &contentservice.GetPostReq{
		PostId: req.PostId,
		UserId: userId,
	})
	if err != nil {
		return nil, rpcx.Error(l.Logger, "ContentService.GetPost", err, logx.Field("postId", req.PostId))
	}

	post := result.Post
	if post == nil {
		return nil, errx.NewWithCode(errx.ContentNotFound)
	}

	liked, favorited, err := viewerstate.Enrich(l.ctx, l.svcCtx, userId, []int64{post.Id})
	if err != nil {
		l.Errorw("viewerstate.Enrich failed", logx.Field("postId", post.Id), logx.Field("err", err.Error()))
		return nil, err
	}

	authors := authorx.LoadSoft(l.ctx, l.svcCtx, []int64{post.AuthorId})
	detail := postmap.Detail(post, liked, favorited, authors[post.AuthorId])
	return &detail, nil
}
