package logic

import (
	"context"
	"esx/app/content/pb/xiaobaihe/content/pb"
	"fmt"

	"esx/app/content/internal/svc"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetPostListLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetPostListLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetPostListLogic {
	return &GetPostListLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// GetPostList 获取帖子列表
func (l *GetPostListLogic) GetPostList(in *pb.GetPostListReq) (*pb.GetPostListResp, error) {
	page := int(in.Page)
	pageSize := int(in.PageSize)
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 50 {
		pageSize = 20
	}

	posts, total, err := l.svcCtx.PostModel.FindList(l.ctx, page, pageSize, int(in.SortBy))
	if err != nil {
		return nil, fmt.Errorf("查询帖子列表失败: %w", err)
	}

	postIds := make([]int64, 0, len(posts))
	for _, post := range posts {
		postIds = append(postIds, post.Id)
	}
	tagsMap, err := l.svcCtx.PostTagModel.FindTagNamesByPostIds(l.ctx, postIds)
	if err != nil {
		l.Logger.Errorf("批量查询标签失败 err=%v", err)
		tagsMap = map[int64][]string{}
	}
	postInfos := make([]*pb.PostInfo, 0, len(posts))
	for _, post := range posts {
		postInfos = append(postInfos, PostToPostInfo(post, tagsMap[post.Id]))
	}

	return &pb.GetPostListResp{
		Posts: postInfos,
		Total: total,
	}, nil
}
