package logic

import (
	"context"

	"errx"
	"esx/app/search/rpc/internal/store"
	"esx/app/search/rpc/internal/svc"
	"esx/app/search/rpc/xiaobaihe/search/pb"
	"user/userservice"

	"github.com/zeromicro/go-zero/core/logx"
)

type SearchLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewSearchLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SearchLogic {
	return &SearchLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// 综合搜索
func (l *SearchLogic) Search(in *pb.SearchReq) (*pb.SearchResp, error) {
	if in == nil || !validPage(in.Page, in.PageSize) {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	searchKeyword, err := keyword(in.Keyword)
	if err != nil {
		return nil, err
	}
	posts, err := l.svcCtx.Store.SearchPosts(l.ctx, store.PostQuery{
		Keyword: searchKeyword, Page: in.Page, PageSize: in.PageSize,
	})
	if err != nil {
		l.Errorw("combined search posts failed", logx.Field("err", err.Error()))
		return nil, storeError(err)
	}
	if l.svcCtx.UserService == nil {
		return nil, errx.NewWithCode(errx.ServiceUnavailable)
	}
	users, err := l.svcCtx.UserService.SearchUsers(l.ctx, &userservice.SearchUsersReq{
		Keyword: searchKeyword, Page: in.Page, PageSize: in.PageSize,
	})
	if err != nil {
		l.Errorw("combined search users RPC failed", logx.Field("err", err.Error()))
		return nil, storeError(err)
	}
	if users == nil {
		return nil, errx.NewWithCode(errx.ServiceUnavailable)
	}
	tags, err := l.svcCtx.Store.SearchTags(l.ctx, searchKeyword, in.PageSize)
	if err != nil {
		l.Errorw("combined search tags failed", logx.Field("err", err.Error()))
		return nil, storeError(err)
	}
	profiles, err := loadUserProfiles(l.ctx, l.svcCtx.UserService, posts.Posts)
	if err != nil {
		l.Errorw("hydrate combined search users failed", logx.Field("err", err.Error()))
		return nil, storeError(err)
	}
	return &pb.SearchResp{
		Posts: postResults(posts.Posts, profiles), Users: userResults(users.Users), Tags: tagResults(tags),
	}, nil
}
