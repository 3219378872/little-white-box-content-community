package logic

import (
	"context"

	"esx/app/search/rpc/internal/svc"
	"esx/app/search/rpc/xiaobaihe/search/pb"
	"esx/pkg/errx"

	"github.com/zeromicro/go-zero/core/logx"
)

type SearchTagsLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewSearchTagsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SearchTagsLogic {
	return &SearchTagsLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
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
		l.Errorw("search tags from post index failed", logx.Field("err", err.Error()))
		return nil, storeError(err)
	}
	return &pb.SearchTagsResp{Tags: tagResults(result)}, nil
}
