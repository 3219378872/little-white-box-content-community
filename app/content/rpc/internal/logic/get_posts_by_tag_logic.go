package logic

import (
	"context"
	"errx"
	"esx/app/content/rpc/internal/svc"
	"esx/app/content/rpc/pb/xiaobaihe/content/pb"
	"esx/pkg/visibilityx"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetPostsByTagLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetPostsByTagLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetPostsByTagLogic {
	return &GetPostsByTagLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// GetPostsByTag 获取标签下的帖子
func (l *GetPostsByTagLogic) GetPostsByTag(in *pb.GetPostsByTagReq) (*pb.GetPostsByTagResp, error) {
	if in.TagName == "" {
		return nil, errx.NewWithCode(errx.ParamError)
	}

	page, pageSize := normalizePage(int(in.Page), int(in.PageSize))

	// FindPostIdsByTagName JOIN published，回源后再丢弃状态已变的帖。
	postIds, total, err := l.svcCtx.PostTagModel.FindPostIdsByTagName(l.ctx, in.TagName, page, pageSize)
	if err != nil {
		l.Errorw("PostTagModel.FindPostIdsByTagName failed",
			logx.Field("tagName", in.TagName),
			logx.Field("err", err.Error()),
		)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	if len(postIds) == 0 {
		return &pb.GetPostsByTagResp{Posts: []*pb.PostInfo{}, Total: total}, nil
	}

	posts, err := l.svcCtx.PostModel.FindByIds(l.ctx, postIds)
	if err != nil {
		l.Errorw("PostModel.FindByIds failed", logx.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}
	postsByID := indexPublishedPosts(posts)
	publishedIDs := make([]int64, 0, len(postsByID))
	for _, postID := range postIds {
		if _, ok := postsByID[postID]; ok {
			publishedIDs = append(publishedIDs, postID)
		}
	}

	tagsMap, err := l.svcCtx.PostTagModel.FindTagNamesByPostIds(l.ctx, publishedIDs)
	if err != nil {
		l.Errorw("PostTagModel.FindTagNamesByPostIds failed", logx.Field("err", err.Error()))
		tagsMap = map[int64][]string{}
	}
	postInfos := make([]*pb.PostInfo, 0, len(publishedIDs))
	for _, postID := range postIds {
		post := postsByID[postID]
		if post == nil {
			continue
		}
		postInfos = append(postInfos, PostToPostInfo(post, tagsMap[post.Id]))
	}
	total = visibilityx.AdjustPageTotal(total, len(postIds), len(postInfos))

	return &pb.GetPostsByTagResp{
		Posts: postInfos,
		Total: total,
	}, nil
}
