package logic

import (
	"context"
	"database/sql"
	"errors"
	"errx"
	model2 "esx/app/content/rpc/internal/model"
	"esx/app/content/rpc/internal/svc"
	"esx/app/content/rpc/pb/xiaobaihe/content/pb"
	"esx/pkg/event"
	"util"

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
		if errors.Is(err, model2.ErrNotFound) {
			return nil, errx.NewWithCode(errx.ContentNotFound)
		}
		l.Errorw("PostModel.FindPostById failed",
			logx.Field("postId", in.PostId),
			logx.Field("err", err.Error()),
		)
		return nil, errx.NewWithCode(errx.SystemError)
	}
	if post.Status == 2 {
		return nil, errx.NewWithCode(errx.PostAlreadyDeleted)
	}

	id, err := util.NextID()
	if err != nil {
		return nil, errx.NewWithCode(errx.SystemError)
	}

	comment := &model2.Comment{
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

	outboxEvent, err := buildBusinessBehaviorOutbox(event.InteractionEvent{
		UserID: in.UserId, Action: event.BehaviorActionComment,
		TargetID: in.PostId, TargetType: "post", Scene: "content",
	})
	if err != nil {
		l.Errorw("build comment behavior event failed", logx.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}
	if l.svcCtx.CommentCommandModel == nil {
		l.Errorw("CommentCommandModel is nil")
		return nil, errx.NewWithCode(errx.SystemError)
	}
	if err = l.svcCtx.CommentCommandModel.CreateComment(l.ctx, comment, outboxEvent); err != nil {
		l.Errorw("create comment transaction failed",
			logx.Field("postId", in.PostId),
			logx.Field("err", err.Error()),
		)
		return nil, errx.NewWithCode(errx.SystemError)
	}
	if err = l.svcCtx.CommentModel.InvalidateCommentCache(l.ctx, id); err != nil {
		l.Errorw("invalidate comment cache after create failed", logx.Field("err", err.Error()))
	}
	if err = l.svcCtx.PostModel.InvalidatePostCache(l.ctx, in.PostId); err != nil {
		l.Errorw("invalidate post cache after comment create failed", logx.Field("err", err.Error()))
	}

	return &pb.CreateCommentResp{
		CommentId: id,
	}, nil
}
