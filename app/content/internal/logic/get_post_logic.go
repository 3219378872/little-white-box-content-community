package logic

import (
	"context"
	"errors"
	"errx"
	"esx/app/content/internal/model"
	"esx/app/content/pb/xiaobaihe/content/pb"
	"fmt"

	"esx/app/content/internal/svc"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetPostLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetPostLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetPostLogic {
	return &GetPostLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// GetPost 获取帖子详情
func (l *GetPostLogic) GetPost(in *pb.GetPostReq) (*pb.GetPostResp, error) {
	if in.PostId <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}

	post, err := l.svcCtx.PostModel.FindPostById(l.ctx, in.PostId)
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			return nil, errx.NewWithCode(errx.ContentNotFound)
		}
		return nil, fmt.Errorf("查询帖子失败: %w", err)
	}

	switch post.Status {
	case 2:
		return nil, errx.NewWithCode(errx.PostAlreadyDeleted)
	case 1:
		// 已发布，继续
	default:
		// 草稿（0）、审核中（3）等非公开状态
		return nil, errx.NewWithCode(errx.ContentNotFound)
	}

	tags, err := l.svcCtx.PostTagModel.FindTagNamesByPostId(l.ctx, post.Id)
	if err != nil {
		l.Logger.Errorf("查询标签失败 postId=%d err=%v", post.Id, err)
		tags = []string{}
	}

	return &pb.GetPostResp{
		Post: PostToPostInfo(post, tags),
	}, nil
}
