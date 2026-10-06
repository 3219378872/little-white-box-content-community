package logic

import (
	"context"

	"esx/app/search/rpc/internal/svc"
	pb "esx/kitex_gen/search"
	"esx/pkg/errx"

	"esx/pkg/logging"
)

// SearchTagsLogic 承载 SearchTags 接口的业务逻辑；每个请求新建一个实例。
type SearchTagsLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logging.Logger
}

// NewSearchTagsLogic 绑定请求上下文与服务依赖，日志自动携带请求追踪信息。
func NewSearchTagsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SearchTagsLogic {
	return &SearchTagsLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logging.WithContext(ctx),
	}
}

// 搜索标签
func (l *SearchTagsLogic) SearchTags(in *pb.SearchTagsReq) (*pb.SearchTagsResp, error) {
	if in == nil || in.Limit <= 0 || in.Limit > maxPageSize {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	searchKeyword, err := keyword(in.Keyword)
	if err != nil {
		return nil, err
	}
	result, err := l.svcCtx.Store.SearchTags(l.ctx, searchKeyword, in.Limit)
	if err != nil {
		l.Errorw("search tags from post index failed", logging.Field("err", err.Error()))
		return nil, storeError(err)
	}
	return &pb.SearchTagsResp{Tags: tagResults(result)}, nil
}
