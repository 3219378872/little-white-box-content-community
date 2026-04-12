package logic

import (
	"context"
	"esx/app/content/pb/xiaobaihe/content/pb"
	"fmt"

	"esx/app/content/internal/svc"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetUserPostsLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetUserPostsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetUserPostsLogic {
	return &GetUserPostsLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// GetUserPosts 获取用户帖子列表
func (l *GetUserPostsLogic) GetUserPosts(in *pb.GetUserPostsReq) (*pb.GetUserPostsResp, error) {
	page := int(in.Page)
	pageSize := int(in.PageSize)
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 50 {
		pageSize = 20
	}

	posts, total, err := l.svcCtx.PostModel.FindByAuthorId(l.ctx, in.UserId, page, pageSize)
	if err != nil {
		return nil, fmt.Errorf("查询用户帖子失败: %w", err)
	}

	if len(posts) == 0 {
		return &pb.GetUserPostsResp{Posts: []*pb.PostInfo{}, Total: total}, nil
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

	return &pb.GetUserPostsResp{
		Posts: postInfos,
		Total: total,
	}, nil
}
