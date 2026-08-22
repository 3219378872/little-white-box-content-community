package logic

import (
	"context"
	"errors"
	model2 "esx/app/content/rpc/internal/model"
	"esx/app/content/rpc/internal/svc"
	"esx/app/content/rpc/pb/xiaobaihe/content/pb"
	"esx/pkg/errx"
	"esx/pkg/visibilityx"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetCommentRepliesLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetCommentRepliesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetCommentRepliesLogic {
	return &GetCommentRepliesLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// GetCommentReplies 获取评论的回复列表（楼中楼全量分页，时间正序）
func (l *GetCommentRepliesLogic) GetCommentReplies(in *pb.GetCommentRepliesReq) (*pb.GetCommentRepliesResp, error) {
	if in.CommentId <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	page, pageSize := normalizePage(int(in.Page), int(in.PageSize))

	// CORE-015/016：父评论不存在或已删除统一不存在；其所属帖子必须已发布。
	parent, err := l.svcCtx.CommentModel.FindCommentById(l.ctx, in.CommentId)
	if err != nil {
		if errors.Is(err, model2.ErrNotFound) {
			return nil, errx.NewWithCode(errx.ContentNotFound)
		}
		l.Errorw("CommentModel.FindCommentById failed",
			logx.Field("commentId", in.CommentId),
			logx.Field("err", err.Error()),
		)
		return nil, errx.NewWithCode(errx.SystemError)
	}
	if parent.Status != 1 || parent.ParentId.Valid {
		return nil, errx.NewWithCode(errx.ContentNotFound)
	}
	post, err := l.svcCtx.PostModel.FindPostById(l.ctx, parent.PostId)
	if err != nil {
		if errors.Is(err, model2.ErrNotFound) {
			return nil, errx.NewWithCode(errx.ContentNotFound)
		}
		l.Errorw("PostModel.FindPostById failed",
			logx.Field("postId", parent.PostId),
			logx.Field("err", err.Error()),
		)
		return nil, errx.NewWithCode(errx.SystemError)
	}
	if !visibilityx.IsPublished(int32(post.Status)) {
		return nil, errx.NewWithCode(errx.ContentNotFound)
	}

	replies, total, err := l.svcCtx.CommentModel.FindByParentId(l.ctx, in.CommentId, page, pageSize)
	if err != nil {
		l.Errorw("CommentModel.FindByParentId failed",
			logx.Field("commentId", in.CommentId),
			logx.Field("err", err.Error()),
		)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	// CORE-016 纵深防御：SQL 已过滤 status=1，这里再按内存状态二次过滤并回减 Total，
	// 与评论列表保持一致。
	fetched := len(replies)
	replies = keepActiveComments(replies)
	total = visibilityx.AdjustPageTotal(total, fetched, len(replies))
	commentInfos := make([]*pb.CommentInfo, 0, len(replies))
	for _, reply := range replies {
		commentInfos = append(commentInfos, CommentToCommentInfo(reply))
	}

	return &pb.GetCommentRepliesResp{
		Comments: commentInfos,
		Total:    total,
	}, nil
}
