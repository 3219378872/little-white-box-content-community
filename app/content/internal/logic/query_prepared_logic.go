package logic

import (
	"context"

	"esx/app/content/internal/svc"
	"esx/app/content/pb/xiaobaihe/content/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type QueryPreparedLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewQueryPreparedLogic(ctx context.Context, svcCtx *svc.ServiceContext) *QueryPreparedLogic {
	return &QueryPreparedLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// DTM reliable message query-prepared checkback
func (l *QueryPreparedLogic) QueryPrepared(in *pb.QueryPreparedReq) (*pb.QueryPreparedResp, error) {
	// todo: add your logic here and delete this line

	return &pb.QueryPreparedResp{}, nil
}
