// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package posts

import (
	"context"

	"esx/app/content/rpc/contentservice"
	"esx/pkg/errx"
	"esx/pkg/jwtx"

	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type DeletePostV2Logic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 删除帖子（v2，强制 expectedRevision，CORE-013）
func NewDeletePostV2Logic(ctx context.Context, svcCtx *svc.ServiceContext) *DeletePostV2Logic {
	return &DeletePostV2Logic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *DeletePostV2Logic) DeletePostV2(req *types.DeletePostV2Req) (resp *types.DeletePostResp, err error) {
	// CORE-013：v2 写接口强制乐观锁，缺失或为 0 一律按参数错误拒绝。
	if req.ExpectedRevision <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}

	userId, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, err
	}

	_, err = l.svcCtx.ContentService.DeletePost(l.ctx, &contentservice.DeletePostReq{
		PostId:           req.PostId,
		AuthorId:         userId,
		ExpectedRevision: req.ExpectedRevision,
	})
	if err != nil {
		l.Errorw("ContentService.DeletePost RPC failed",
			logx.Field("postId", req.PostId),
			logx.Field("err", err.Error()),
		)
		return nil, errx.FromRPCError(err)
	}

	return &types.DeletePostResp{}, nil
}
