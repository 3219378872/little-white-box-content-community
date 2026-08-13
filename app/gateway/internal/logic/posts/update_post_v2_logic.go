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

type UpdatePostV2Logic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 更新帖子（v2，强制 expectedRevision，CORE-013）
func NewUpdatePostV2Logic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdatePostV2Logic {
	return &UpdatePostV2Logic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *UpdatePostV2Logic) UpdatePostV2(req *types.UpdatePostV2Req) (resp *types.UpdatePostResp, err error) {
	// CORE-013：v2 写接口强制乐观锁，缺失或为 0 一律按参数错误拒绝，
	// 不允许通过 0 绕过版本冲突检测。
	if req.ExpectedRevision <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}

	userId, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, err
	}

	result, err := l.svcCtx.ContentService.UpdatePost(l.ctx, &contentservice.UpdatePostReq{
		PostId:           req.PostId,
		AuthorId:         userId,
		Title:            req.Title,
		Content:          req.Content,
		Images:           req.Images,
		Tags:             req.Tags,
		Status:           req.Status,
		ExpectedRevision: req.ExpectedRevision,
		MediaIds:         req.MediaIds,
	})
	if err != nil {
		l.Errorw("ContentService.UpdatePost RPC failed",
			logx.Field("postId", req.PostId),
			logx.Field("err", err.Error()),
		)
		return nil, errx.FromRPCError(err)
	}

	return &types.UpdatePostResp{
		Status:   result.Status,
		Revision: result.Revision,
	}, nil
}
