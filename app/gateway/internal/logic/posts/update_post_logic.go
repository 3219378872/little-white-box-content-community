// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package posts

import (
	"context"
	"esx/app/content/contentservice"
	"fmt"
	"jwtx"

	"gateway/internal/svc"
	"gateway/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type UpdatePostLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 更新帖子
func NewUpdatePostLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdatePostLogic {
	return &UpdatePostLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *UpdatePostLogic) UpdatePost(req *types.UpdatePostReq) (resp *types.UpdatePostResp, err error) {
	userId, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, err
	}

	_, err = l.svcCtx.ContentService.UpdatePost(l.ctx, &contentservice.UpdatePostReq{
		PostId:   req.PostId,
		AuthorId: userId,
		Title:    req.Title,
		Content:  req.Content,
		Images:   req.Images,
		Tags:     req.Tags,
	})
	if err != nil {
		return nil, fmt.Errorf("更新帖子失败: %w", err)
	}

	return &types.UpdatePostResp{}, nil
}
