package logic

import (
	"context"
	"errx"
	"esx/app/content/internal/model"
	"esx/app/content/pb/xiaobaihe/content/pb"
	"fmt"

	"esx/app/content/internal/svc"

	"github.com/zeromicro/go-zero/core/logx"
)

type DeletePostLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewDeletePostLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeletePostLogic {
	return &DeletePostLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// DeletePost 删除帖子（软删除，status=2）
func (l *DeletePostLogic) DeletePost(in *pb.DeletePostReq) (*pb.DeletePostResp, error) {
	if in.PostId <= 0 || in.AuthorId <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}

	post, err := l.svcCtx.PostModel.FindPostById(l.ctx, in.PostId)
	if err != nil {
		if err == model.ErrNotFound {
			return nil, errx.NewWithCode(errx.ContentNotFound)
		}
		return nil, fmt.Errorf("查询帖子失败: %w", err)
	}

	if post.Status == 2 {
		return nil, errx.NewWithCode(errx.PostAlreadyDeleted)
	}
	if post.AuthorId != in.AuthorId {
		return nil, errx.NewWithCode(errx.ContentForbidden)
	}

	if err = l.svcCtx.PostModel.UpdateStatus(l.ctx, post.Id, 2); err != nil {
		return nil, fmt.Errorf("删除帖子失败: %w", err)
	}

	return &pb.DeletePostResp{}, nil
}
