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
	"esx/pkg/idempotencyx"
	"esx/pkg/visibilityx"
	"strconv"
	"strings"
	"unicode/utf8"
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
	contentRunes := utf8.RuneCountInString(in.GetContent())
	if contentRunes < 1 {
		return nil, errx.NewWithCode(errx.ContentEmpty)
	}
	if contentRunes > 2000 {
		return nil, errx.NewWithCode(errx.ContentTooLong)
	}
	idempotencyKey := strings.TrimSpace(in.GetIdempotencyKey())
	idem := idempotencyx.IdempotencyRecord{
		Scope:       "comment:create",
		UserID:      in.UserId,
		Key:         idempotencyKey,
		CommandHash: idempotencyx.CommandHash(in.GetContent(), strconv.FormatInt(in.GetPostId(), 10)),
	}
	if !idem.Valid() {
		return nil, errx.NewWithCode(errx.ParamError)
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
	// CORE-022：评论只能附着在当前可互动的已发布内容上。
	if !visibilityx.IsPublished(int32(post.Status)) {
		return nil, errx.NewWithCode(errx.ContentNotFound)
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
	commentID, created, err := l.svcCtx.CommentCommandModel.CreateComment(l.ctx, comment, outboxEvent, idem)
	if err != nil {
		if errors.Is(err, idempotencyx.ErrIdempotencyConflict) {
			return nil, errx.NewWithCode(errx.IdempotencyConflict)
		}
		l.Errorw("create comment transaction failed",
			logx.Field("postId", in.PostId),
			logx.Field("err", err.Error()),
		)
		return nil, errx.NewWithCode(errx.SystemError)
	}
	if created {
		if err = l.svcCtx.CommentModel.InvalidateCommentCache(l.ctx, id); err != nil {
			l.Errorw("invalidate comment cache after create failed", logx.Field("err", err.Error()))
		}
		if err = l.svcCtx.PostModel.InvalidatePostCache(l.ctx, in.PostId); err != nil {
			l.Errorw("invalidate post cache after comment create failed", logx.Field("err", err.Error()))
		}
	}

	return &pb.CreateCommentResp{
		CommentId: commentID,
	}, nil
}
