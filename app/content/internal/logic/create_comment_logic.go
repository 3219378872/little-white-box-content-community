package logic

import (
	"context"
	"database/sql"
	"errors"
	"errx"
	"esx/app/content/internal/model"
	"esx/app/content/pb/xiaobaihe/content/pb"
	"fmt"
	"util"

	"esx/app/content/internal/svc"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateCommentLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewCreateCommentLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateCommentLogic {
	return &CreateCommentLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// CreateComment 创建评论
func (l *CreateCommentLogic) CreateComment(in *pb.CreateCommentReq) (*pb.CreateCommentResp, error) {
	if in.PostId <= 0 || in.UserId <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	if in.Content == "" {
		return nil, errx.NewWithCode(errx.ContentEmpty)
	}

	// 验证帖子是否存在
	post, err := l.svcCtx.PostModel.FindPostById(l.ctx, in.PostId)
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			return nil, errx.NewWithCode(errx.ContentNotFound)
		}
		return nil, fmt.Errorf("查询帖子失败: %w", err)
	}
	if post.Status == 2 {
		return nil, errx.NewWithCode(errx.PostAlreadyDeleted)
	}

	id, err := util.NextID()
	if err != nil {
		return nil, errx.NewWithCode(errx.SystemError)
	}

	comment := &model.Comment{
		Id:      id,
		PostId:  in.PostId,
		UserId:  in.UserId,
		Content: in.Content,
		Status:  1,
	}
	if in.ParentId > 0 {
		comment.ParentId = sql.NullInt64{Int64: in.ParentId, Valid: true}
	}
	if in.ReplyUserId > 0 {
		comment.ReplyUserId = sql.NullInt64{Int64: in.ReplyUserId, Valid: true}
	}

	if err = l.svcCtx.CommentModel.InsertComment(l.ctx, comment); err != nil {
		return nil, fmt.Errorf("创建评论失败: %w", err)
	}

	// 原子递增评论数；计数服务不可用时降级——评论已落库，不因统计失败回滚
	if err = l.svcCtx.PostModel.IncrCommentCount(l.ctx, in.PostId); err != nil {
		l.Logger.Errorf("更新评论数失败 postId=%d err=%v", in.PostId, err)
	}

	return &pb.CreateCommentResp{
		CommentId: id,
	}, nil
}
