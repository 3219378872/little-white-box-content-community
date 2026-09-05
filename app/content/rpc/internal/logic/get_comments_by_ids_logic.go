package logic

import (
	"context"
	"errors"
	"esx/app/content/rpc/internal/model"
	"esx/pkg/errx"
	"esx/pkg/visibilityx"

	"esx/app/content/rpc/internal/svc"
	"esx/app/content/rpc/pb/xiaobaihe/content/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetCommentsByIdsLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetCommentsByIdsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetCommentsByIdsLogic {
	return &GetCommentsByIdsLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *GetCommentsByIdsLogic) GetCommentsByIds(in *pb.GetCommentsByIdsReq) (*pb.GetCommentsByIdsResp, error) {
	if in == nil || in.PostId <= 0 || len(in.Ids) == 0 || len(in.Ids) > 80 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	for _, id := range in.Ids {
		if id <= 0 {
			return nil, errx.NewWithCode(errx.ParamError)
		}
	}
	post, err := l.svcCtx.PostModel.FindPostById(l.ctx, in.PostId)
	if errors.Is(err, model.ErrNotFound) {
		return nil, errx.NewWithCode(errx.ContentNotFound)
	}
	if err != nil {
		return nil, errx.NewWithCode(errx.SystemError)
	}
	if !visibilityx.IsPublished(int32(post.Status)) {
		return nil, errx.NewWithCode(errx.ContentNotFound)
	}
	comments, err := l.svcCtx.CommentModel.FindActiveByIds(l.ctx, in.PostId, in.Ids)
	if err != nil {
		return nil, errx.NewWithCode(errx.SystemError)
	}
	out := &pb.GetCommentsByIdsResp{Comments: []*pb.CommentInfo{}}
	for _, comment := range comments {
		if comment.PostId == in.PostId && comment.Status == 1 {
			out.Comments = append(out.Comments, CommentToCommentInfo(comment))
		}
	}
	return out, nil
}
