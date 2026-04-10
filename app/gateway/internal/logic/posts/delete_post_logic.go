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

type DeletePostLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 删除帖子
func NewDeletePostLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeletePostLogic {
	return &DeletePostLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *DeletePostLogic) DeletePost(req *types.DeletePostReq) (resp *types.DeletePostResp, err error) {
	userId, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, err
	}

	_, err = l.svcCtx.ContentService.DeletePost(l.ctx, &contentservice.DeletePostReq{
		PostId:   req.PostId,
		AuthorId: userId,
	})
	if err != nil {
		return nil, fmt.Errorf("删除帖子失败: %w", err)
	}

	return &types.DeletePostResp{}, nil
}
