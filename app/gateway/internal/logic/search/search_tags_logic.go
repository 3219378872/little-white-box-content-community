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

type SearchTagsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 搜索标签
func NewSearchTagsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SearchTagsLogic {
	return &SearchTagsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *SearchTagsLogic) SearchTags(req *types.SearchTagsReq) (resp *types.SearchTagsResp, err error) {
	if req == nil || req.Limit <= 0 || req.Limit > maxPageSize {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	keyword, err := searchKeyword(req.Keyword)
	if err != nil {
		return nil, err
	}

	result, err := l.svcCtx.SearchService.SearchTags(l.ctx, &searchservice.SearchTagsReq{
		Keyword: keyword,
		Limit:   req.Limit,
	})
	if err != nil {
		l.Errorw("SearchService.SearchTags RPC failed", logx.Field("err", err.Error()))
		return nil, errx.FromRPCError(err)
	}
	if result == nil {
		l.Error("SearchService.SearchTags returned a nil response")
		return nil, errx.NewWithCode(errx.SystemError)
	}

	return &types.SearchTagsResp{Tags: searchTags(result.Tags)}, nil
}
