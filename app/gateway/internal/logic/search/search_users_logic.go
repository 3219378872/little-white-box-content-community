package search

import (
	"context"

	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/app/search/rpc/searchservice"
	"esx/pkg/errx"

	"esx/pkg/logging"
)

// SearchUsersLogic 承载 SearchUsers 接口的业务逻辑；每个请求新建一个实例。
type SearchUsersLogic struct {
	logging.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 搜索用户
func NewSearchUsersLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SearchUsersLogic {
	return &SearchUsersLogic{
		Logger: logging.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// SearchUsers 分页搜索用户。
func (l *SearchUsersLogic) SearchUsers(req *types.SearchUsersReq) (resp *types.SearchUsersResp, err error) {
	if req == nil || !validPage(req.Page, req.PageSize) {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	keyword, err := searchKeyword(req.Keyword)
	if err != nil {
		return nil, err
	}

	result, err := l.svcCtx.SearchService.SearchUsers(l.ctx, &searchservice.SearchUsersReq{
		Keyword:  keyword,
		Page:     req.Page,
		PageSize: req.PageSize,
	})
	if err != nil {
		l.Errorw("SearchService.SearchUsers RPC failed", logging.Field("err", err.Error()))
		return nil, errx.FromRPCError(err)
	}
	if result == nil {
		l.Error("SearchService.SearchUsers returned a nil response")
		return nil, errx.NewWithCode(errx.SystemError)
	}

	return &types.SearchUsersResp{
		Users: searchUsers(result.Users),
		Total: result.Total,
	}, nil
}
