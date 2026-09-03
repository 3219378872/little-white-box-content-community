// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package posts

import (
	"context"

	"esx/app/content/rpc/contentservice"
	"esx/app/gateway/internal/logic/rpcx"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreatePostV2Logic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 创建帖子（v2）：创建无前置 revision，契约与 v1 一致（CORE-013 只约束变更类操作）。
func NewCreatePostV2Logic(ctx context.Context, svcCtx *svc.ServiceContext) *CreatePostV2Logic {
	return &CreatePostV2Logic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreatePostV2Logic) CreatePostV2(req *types.CreatePostReq) (resp *types.CreatePostResp, err error) {
	userId, err := rpcx.RequireUser(l.ctx)
	if err != nil {
		return nil, err
	}

	result, err := l.svcCtx.ContentService.CreatePost(l.ctx, &contentservice.CreatePostReq{
		AuthorId:       userId,
		Title:          req.Title,
		Content:        req.Content,
		Images:         req.Images,
		Tags:           req.Tags,
		Status:         req.Status,
		IdempotencyKey: req.IdempotencyKey,
		MediaIds:       req.MediaIds,
	})
	if err != nil {
		return nil, rpcx.Error(l.Logger, "ContentService.CreatePost", err)
	}

	return &types.CreatePostResp{
		PostId:   result.PostId,
		Status:   result.Status,
		Revision: result.Revision,
	}, nil
}
