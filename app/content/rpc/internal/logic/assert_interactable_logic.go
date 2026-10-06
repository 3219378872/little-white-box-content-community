package logic

import (
	"context"
	"errors"

	"esx/app/content/rpc/internal/model"
	"esx/app/content/rpc/internal/svc"
	pb "esx/kitex_gen/content"
	"esx/pkg/errx"
	"esx/pkg/visibilityx"

	"esx/pkg/logging"
)

const (
	interactablePost    int32 = 1
	interactableComment int32 = 2
)

// AssertInteractableLogic 承载 AssertInteractable 接口的业务逻辑；每个请求新建一个实例。
type AssertInteractableLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logging.Logger
}

// NewAssertInteractableLogic 绑定请求上下文与服务依赖，日志自动携带请求追踪信息。
func NewAssertInteractableLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AssertInteractableLogic {
	return &AssertInteractableLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logging.WithContext(ctx),
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

// assertComment 要求评论有效，且其所属帖子已发布。
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
			logging.Field("commentId", commentID),
			logging.Field("err", err.Error()),
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

// requirePublishedPost 要求帖子存在且已发布；不可见与不存在统一返回内容不存在。
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
			logging.Field("postId", postID),
			logging.Field("err", err.Error()),
		)
		return errx.NewWithCode(errx.SystemError)
	}
	if post == nil || !visibilityx.IsPublished(int32(post.Status)) {
		return errx.NewWithCode(errx.ContentNotFound)
	}
	return nil
}
