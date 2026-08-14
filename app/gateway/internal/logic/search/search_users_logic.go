// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package search

import (
	"context"

	"errx"
	"esx/app/search/rpc/searchservice"
	"gateway/internal/svc"
	"gateway/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type SearchUsersLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 搜索用户
func NewSearchUsersLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SearchUsersLogic {
	return &SearchUsersLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

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
		l.Errorw("SearchService.SearchUsers RPC failed", logx.Field("err", err.Error()))
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
