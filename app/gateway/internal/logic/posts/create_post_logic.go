// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package posts

import (
	"context"
	"esx/app/content/contentservice"
	"jwtx"

	"gateway/internal/svc"
	"gateway/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreatePostLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 创建帖子
func NewCreatePostLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreatePostLogic {
	return &CreatePostLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreatePostLogic) CreatePost(req *types.CreatePostReq) (resp *types.CreatePostResp, err error) {
	userId, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, err
	}

	result, err := l.svcCtx.ContentService.CreatePost(l.ctx, &contentservice.CreatePostReq{
		AuthorId: userId,
		Title:    req.Title,
		Content:  req.Content,
		Images:   req.Images,
		Tags:     req.Tags,
		Status:   req.Status,
	})
	if err != nil {
		return nil, err
	}

	return &types.CreatePostResp{
		PostId: result.PostId,
	}, nil
}
