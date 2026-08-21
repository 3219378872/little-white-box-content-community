// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package search

import (
	"context"

	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/app/search/rpc/searchservice"
	"esx/pkg/errx"

	"github.com/zeromicro/go-zero/core/logx"
)

type SearchLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 综合搜索
func NewSearchLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SearchLogic {
	return &SearchLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

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
		l.Errorw("SearchService.Search RPC failed", logx.Field("err", err.Error()))
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
