package logic

import (
	"context"

	"errx"
	"esx/app/content/contentservice"
	"esx/app/feed/internal/svc"
	"esx/app/feed/xiaobaihe/feed/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetRecommendFeedLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetRecommendFeedLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetRecommendFeedLogic {
	return &GetRecommendFeedLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *GetRecommendFeedLogic) GetRecommendFeed(in *pb.GetRecommendFeedReq) (*pb.GetRecommendFeedResp, error) {
	if in.Page <= 0 || in.PageSize <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	postResp, err := l.svcCtx.ContentService.GetPostList(l.ctx, &contentservice.GetPostListReq{Page: in.Page, PageSize: in.PageSize, SortBy: 3})
	if err != nil {
		postResp, err = l.svcCtx.ContentService.GetPostList(l.ctx, &contentservice.GetPostListReq{Page: in.Page, PageSize: in.PageSize, SortBy: 1})
		if err != nil {
			l.Errorw("ContentService.GetPostList failed", logx.Field("err", err.Error()))
			return nil, errx.NewWithCode(errx.SystemError)
		}
	}
	items := make([]*pb.FeedItem, 0, len(postResp.Posts))
	for _, post := range postResp.Posts {
		items = append(items, &pb.FeedItem{PostId: post.Id, AuthorId: post.AuthorId, CreatedAt: post.CreatedAt, FeedType: 2})
	}
	return &pb.GetRecommendFeedResp{Items: items, HasMore: int64(in.Page*in.PageSize) < postResp.Total}, nil
}
