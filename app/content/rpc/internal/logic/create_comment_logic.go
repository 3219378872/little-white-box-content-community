package logic

import (
	"context"
	"database/sql"
	"errors"
	model2 "esx/app/content/rpc/internal/model"
	"esx/app/content/rpc/internal/svc"
	"esx/app/content/rpc/pb/xiaobaihe/content/pb"
	"esx/pkg/errx"
	"esx/pkg/event"
	"esx/pkg/idempotencyx"
	"esx/pkg/util"
	"esx/pkg/visibilityx"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateCommentLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

// commentIdempotencyRecord 构造评论创建幂等记录（CORE-050/051）。
// 哈希覆盖内容、帖子、回复目标评论与被回复用户：同键异命令返回幂等冲突，
// 而不是静默返回旧评论。
func commentIdempotencyRecord(in *pb.CreateCommentReq) idempotencyx.IdempotencyRecord {
	return idempotencyx.IdempotencyRecord{
		Scope:  "comment:create",
		UserID: in.GetUserId(),
		Key:    strings.TrimSpace(in.GetIdempotencyKey()),
		CommandHash: idempotencyx.CommandHash(
			in.GetContent(),
			strconv.FormatInt(in.GetPostId(), 10),
			strconv.FormatInt(in.GetParentId(), 10),
			strconv.FormatInt(in.GetReplyUserId(), 10),
		),
	}
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
	idem := commentIdempotencyRecord(in)
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

	// ReplyUserId 由客户端传入且用于构造回复通知，必须校验其与父评论的
	// 从属关系，防止对任意用户伪造"评论回复"通知。
	if in.ReplyUserId > 0 {
		if in.ParentId <= 0 {
			return nil, errx.NewWithCode(errx.ParamError)
		}
		parent, err := l.svcCtx.CommentModel.FindCommentById(l.ctx, in.ParentId)
		if err != nil {
			if errors.Is(err, model2.ErrNotFound) {
				return nil, errx.NewWithCode(errx.ParamError)
			}
			l.Errorw("CommentModel.FindCommentById failed",
				logx.Field("parentId", in.ParentId),
				logx.Field("err", err.Error()),
			)
			return nil, errx.NewWithCode(errx.SystemError)
		}
		if parent.PostId != in.PostId || parent.UserId != in.ReplyUserId {
			return nil, errx.NewWithCode(errx.ParamError)
		}
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
