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

type GetCommentListLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetCommentListLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetCommentListLogic {
	return &GetCommentListLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// GetCommentList 获取评论列表（一级评论分页）
func (l *GetCommentListLogic) GetCommentList(in *pb.GetCommentListReq) (*pb.GetCommentListResp, error) {
	page, pageSize := normalizePage(int(in.Page), int(in.PageSize))

	// CORE-015/016：只有已发布内容的评论可公开读取；草稿/删除/不可用内容
	// 的评论线程统一返回不存在，不泄露历史状态。
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
	if !visibilityx.IsPublished(int32(post.Status)) {
		return nil, errx.NewWithCode(errx.ContentNotFound)
	}

	comments, total, err := l.svcCtx.CommentModel.FindByPostId(l.ctx, in.PostId, page, pageSize, int(in.SortBy))
	if err != nil {
		l.Errorw("CommentModel.FindByPostId failed",
			logx.Field("postId", in.PostId),
			logx.Field("err", err.Error()),
		)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	// CORE-016 纵深防御：SQL 已过滤 status=1，这里再按内存状态二次过滤，
	// 与帖子列表“SQL 过滤后再次丢弃非 published 行”的模式一致；Total 随可见行回减。
	fetched := len(comments)
	comments = keepActiveComments(comments)
	total = visibilityx.AdjustPageTotal(total, fetched, len(comments))

	// 批量拉取本页一级评论的可见回复，内嵌前 previewReplyLimit 条预览（时间正序）。
	parentIds := make([]int64, 0, len(comments))
	for _, comment := range comments {
		parentIds = append(parentIds, comment.Id)
	}
	var replies []*model2.Comment
	if len(parentIds) > 0 {
		var err error
		replies, err = l.svcCtx.CommentModel.FindByParentIds(l.ctx, in.PostId, parentIds)
		if err != nil {
			l.Errorw("CommentModel.FindByParentIds failed",
				logx.Field("postId", in.PostId),
				logx.Field("err", err.Error()),
			)
			return nil, errx.NewWithCode(errx.SystemError)
		}
	}
	replies = keepActiveComments(replies)
	previews := groupReplyPreviews(replies)

	commentInfos := make([]*pb.CommentInfo, 0, len(comments))
	for _, comment := range comments {
		info := CommentToCommentInfo(comment)
		info.Replies = previews[comment.Id]
		commentInfos = append(commentInfos, info)
	}

	return &pb.GetCommentListResp{
		Comments: commentInfos,
		Total:    total,
	}, nil
}
