package logic

import (
	"context"
	"errors"

	"errx"
	"esx/app/content/rpc/internal/model"
	"esx/app/content/rpc/internal/svc"
	"esx/app/content/rpc/pb/xiaobaihe/content/pb"
	"esx/pkg/visibilityx"

	"github.com/zeromicro/go-zero/core/logx"
)

const (
	interactablePost    int32 = 1
	interactableComment int32 = 2
)

type AssertInteractableLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewAssertInteractableLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AssertInteractableLogic {
	return &AssertInteractableLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// AssertInteractable 断言目标当前可互动（CORE-034）。
// 帖子必须 published；评论必须有效且父帖 published。权威不可用失败关闭。
func (l *AssertInteractableLogic) AssertInteractable(in *pb.AssertInteractableReq) (*pb.AssertInteractableResp, error) {
	if in == nil || in.TargetId <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	switch in.TargetType {
	case interactablePost:
		if err := l.requirePublishedPost(in.TargetId); err != nil {
			return nil, err
		}
		return &pb.AssertInteractableResp{}, nil
	case interactableComment:
		return l.assertComment(in.TargetId)
	default:
		return nil, errx.NewWithCode(errx.ParamError)
	}
}

func (l *AssertInteractableLogic) assertComment(commentID int64) (*pb.AssertInteractableResp, error) {
	if l.svcCtx == nil || l.svcCtx.CommentModel == nil {
		return nil, errx.NewWithCode(errx.ServiceUnavailable)
	}
	comment, err := l.svcCtx.CommentModel.FindCommentById(l.ctx, commentID)
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			return nil, errx.NewWithCode(errx.ContentNotFound)
		}
		l.Errorw("CommentModel.FindCommentById failed",
			logx.Field("commentId", commentID),
			logx.Field("err", err.Error()),
		)
		return nil, errx.NewWithCode(errx.SystemError)
	}
	if comment == nil || comment.Status != commentActiveStatus || comment.PostId <= 0 {
		return nil, errx.NewWithCode(errx.ContentNotFound)
	}
	if err := l.requirePublishedPost(comment.PostId); err != nil {
		return nil, err
	}
	return &pb.AssertInteractableResp{}, nil
}

func (l *AssertInteractableLogic) requirePublishedPost(postID int64) error {
	if l.svcCtx == nil || l.svcCtx.PostModel == nil {
		return errx.NewWithCode(errx.ServiceUnavailable)
	}
	post, err := l.svcCtx.PostModel.FindPostById(l.ctx, postID)
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			return errx.NewWithCode(errx.ContentNotFound)
		}
		l.Errorw("PostModel.FindPostById failed",
			logx.Field("postId", postID),
			logx.Field("err", err.Error()),
		)
		return errx.NewWithCode(errx.SystemError)
	}
	if post == nil || !visibilityx.IsPublished(int32(post.Status)) {
		return errx.NewWithCode(errx.ContentNotFound)
	}
	return nil
}
