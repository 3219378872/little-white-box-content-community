package logic

import (
	"context"
	"strings"

	"esx/app/search/rpc/internal/store"
	"esx/app/search/rpc/internal/svc"
	"esx/app/search/rpc/xiaobaihe/search/pb"
	"esx/pkg/errx"

	"esx/app/user/rpc/userservice"

	"github.com/zeromicro/go-zero/core/logx"
)

type SearchPostsLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewSearchPostsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SearchPostsLogic {
	return &SearchPostsLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// 搜索帖子
func (l *SearchPostsLogic) SearchPosts(in *pb.SearchPostsReq) (*pb.SearchPostsResp, error) {
	if in == nil || !validPage(in.Page, in.PageSize) || in.SortBy < 0 || in.SortBy > 3 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	searchKeyword, err := keyword(in.Keyword)
	if err != nil {
		return nil, err
	}
	tags := make([]string, 0, len(in.Tags))
	for _, tag := range in.Tags {
		if tag = strings.TrimSpace(tag); tag != "" {
			tags = append(tags, tag)
		}
	}
	result, err := l.svcCtx.Store.SearchPosts(l.ctx, store.PostQuery{
		Keyword: searchKeyword, Page: in.Page, PageSize: in.PageSize, SortBy: in.SortBy, Tags: tags,
	})
	if err != nil {
		l.Errorw("search posts failed", logx.Field("err", err.Error()))
		return nil, storeError(err)
	}
	fetched := len(result.Posts)
	visiblePosts, err := publishedSearchPosts(l.ctx, l.svcCtx.ContentService, result.Posts)
	if err != nil {
		l.Errorw("search posts visibility check failed", logx.Field("err", err.Error()))
		return nil, storeError(err)
	}
	result.Posts = visiblePosts
	result.Total = searchTotalAfterVisibility(result.Total, fetched, len(visiblePosts))
	profiles, err := loadUserProfiles(l.ctx, l.svcCtx.UserService, result.Posts)
	if err != nil {
		l.Errorw("hydrate search post authors failed", logx.Field("err", err.Error()))
		profiles = map[int64]*userservice.UserInfo{}
	}
	return &pb.SearchPostsResp{Posts: postResults(result.Posts, profiles), Total: result.Total}, nil
}
