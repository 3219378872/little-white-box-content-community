package search

import (
	"context"

	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/app/search/rpc/searchservice"
	"esx/pkg/errx"

	"esx/pkg/logging"
)

// SearchTagsLogic 承载 SearchTags 接口的业务逻辑；每个请求新建一个实例。
type SearchTagsLogic struct {
	logging.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 搜索标签
func NewSearchTagsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SearchTagsLogic {
	return &SearchTagsLogic{
		Logger: logging.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// SearchTags 按关键词联想标签，返回条数受 Limit 约束。
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
		l.Errorw("SearchService.SearchTags RPC failed", logging.Field("err", err.Error()))
		return nil, errx.FromRPCError(err)
	}
	if result == nil {
		l.Error("SearchService.SearchTags returned a nil response")
		return nil, errx.NewWithCode(errx.SystemError)
	}

	return &types.SearchTagsResp{Tags: searchTags(result.Tags)}, nil
}
