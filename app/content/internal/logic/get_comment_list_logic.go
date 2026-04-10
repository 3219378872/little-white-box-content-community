package logic

import (
	"context"
	"esx/app/content/pb/xiaobaihe/content/pb"
	"fmt"

	"esx/app/content/internal/svc"

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
	page := int(in.Page)
	pageSize := int(in.PageSize)
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 50 {
		pageSize = 20
	}

	comments, total, err := l.svcCtx.CommentModel.FindByPostId(l.ctx, in.PostId, page, pageSize, int(in.SortBy))
	if err != nil {
		return nil, fmt.Errorf("查询评论列表失败: %w", err)
	}

	commentInfos := make([]*pb.CommentInfo, 0, len(comments))
	for _, c := range comments {
		commentInfos = append(commentInfos, CommentToCommentInfo(c))
	}

	return &pb.GetCommentListResp{
		Comments: commentInfos,
		Total:    total,
	}, nil
}
