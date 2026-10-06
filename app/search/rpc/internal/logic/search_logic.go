package logic

import (
	"context"

	"golang.org/x/sync/errgroup"

	"esx/app/search/rpc/internal/store"
	"esx/app/search/rpc/internal/svc"
	"esx/app/user/rpc/userservice"
	pb "esx/kitex_gen/search"
	"esx/pkg/errx"

	"esx/pkg/logging"
)

type SearchLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logging.Logger
}

func NewSearchLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SearchLogic {
	return &SearchLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logging.WithContext(ctx),
	}
}

// 综合搜索：帖子（含可见性回源）、用户、标签三路并行；帖子失败则取消其余分支并整体失败
// （DISC-022/041），用户或标签失败降级（DISC-023）。
func (l *SearchLogic) Search(in *pb.SearchReq) (*pb.SearchResp, error) {
	if in == nil || !validPage(in.Page, in.PageSize) {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	searchKeyword, err := keyword(in.Keyword)
	if err != nil {
		return nil, err
	}

	group, ctx := errgroup.WithContext(l.ctx)
	var posts []store.Post
	group.Go(func() error {
		result, err := l.svcCtx.Store.SearchPosts(ctx, store.PostQuery{
			Keyword: searchKeyword, Page: in.Page, PageSize: in.PageSize,
		})
		if err != nil {
			l.Errorw("combined search posts failed", logging.Field("err", err.Error()))
			return storeError(err)
		}
		posts, err = publishedSearchPosts(ctx, l.svcCtx.ContentService, result.Posts)
		if err != nil {
			l.Errorw("combined search visibility check failed", logging.Field("err", err.Error()))
			return storeError(err)
		}
		return nil
	})
	users := []*userservice.UserInfo{}
	userUnavailable := false
	group.Go(func() error {
		if l.svcCtx.UserService == nil {
			userUnavailable = true
			return nil
		}
		result, err := l.svcCtx.UserService.SearchUsers(ctx, &userservice.SearchUsersReq{
			Keyword: searchKeyword, Page: in.Page, PageSize: in.PageSize, SkipTotal: true,
		})
		if err != nil || result == nil {
			l.Errorw("combined search users RPC failed", logging.Field("err", err))
			userUnavailable = true
			return nil
		}
		users = result.Users
		return nil
	})
	tags := []store.Tag{}
	tagUnavailable := false
	group.Go(func() error {
		result, err := l.svcCtx.Store.SearchTags(ctx, searchKeyword, in.PageSize)
		if err != nil {
			l.Errorw("combined search tags failed", logging.Field("err", err.Error()))
			tagUnavailable = true
			return nil
		}
		tags = result
		return nil
	})
	if err := group.Wait(); err != nil {
		return nil, err
	}

	unavailableTypes := make([]string, 0, 2)
	if userUnavailable {
		unavailableTypes = append(unavailableTypes, "user")
	}
	if tagUnavailable {
		unavailableTypes = append(unavailableTypes, "tag")
	}
	profiles, err := loadAuthorCards(l.ctx, l.svcCtx.UserService, posts)
	if err != nil {
		l.Errorw("hydrate combined search authors failed", logging.Field("err", err.Error()))
		profiles = map[int64]*userservice.UserCard{}
	}
	return &pb.SearchResp{
		Posts:            postResults(posts, profiles),
		Users:            userResults(users),
		Tags:             tagResults(tags),
		Degraded:         len(unavailableTypes) > 0,
		UnavailableTypes: unavailableTypes,
	}, nil
}
