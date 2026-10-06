package search

import (
	"context"

	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/app/search/rpc/searchservice"
	"esx/pkg/errx"

	"esx/pkg/logging"
)

// SearchLogic 承载 Search 接口的业务逻辑；每个请求新建一个实例。
type SearchLogic struct {
	logging.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 综合搜索
func NewSearchLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SearchLogic {
	return &SearchLogic{
		Logger: logging.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// Search 执行综合搜索；部分索引不可用时由搜索服务标记 Degraded 与缺失类型，网关原样透出。
func (l *SearchLogic) Search(req *types.SearchReq) (resp *types.SearchResp, err error) {
	if req == nil || !validPage(req.Page, req.PageSize) {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	keyword, err := searchKeyword(req.Keyword)
	if err != nil {
		return nil, err
	}

	result, err := l.svcCtx.SearchService.Search(l.ctx, &searchservice.SearchReq{
		Keyword:  keyword,
		Page:     req.Page,
		PageSize: req.PageSize,
	})
	if err != nil {
		l.Errorw("SearchService.Search RPC failed", logging.Field("err", err.Error()))
		return nil, errx.FromRPCError(err)
	}
	if result == nil {
		l.Error("SearchService.Search returned a nil response")
		return nil, errx.NewWithCode(errx.SystemError)
	}

	return &types.SearchResp{
		Posts:            searchPosts(result.Posts),
		Users:            searchUsers(result.Users),
		Tags:             searchTags(result.Tags),
		Degraded:         result.Degraded,
		UnavailableTypes: append([]string(nil), result.UnavailableTypes...),
	}, nil
}
