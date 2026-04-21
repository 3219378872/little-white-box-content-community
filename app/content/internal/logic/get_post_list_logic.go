package logic

import (
	"context"
	"errx"
	"esx/app/content/pb/xiaobaihe/content/pb"

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
		l.Errorw("PostModel.FindList failed", logx.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}

	if len(posts) == 0 {
		return &pb.GetPostListResp{Posts: []*pb.PostInfo{}, Total: total}, nil
	}

	postIds := make([]int64, 0, len(posts))
	for _, post := range posts {
		postIds = append(postIds, post.Id)
	}
	tagsMap, err := l.svcCtx.PostTagModel.FindTagNamesByPostIds(l.ctx, postIds)
	if err != nil {
		l.Errorw("PostTagModel.FindTagNamesByPostIds failed", logx.Field("err", err.Error()))
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
