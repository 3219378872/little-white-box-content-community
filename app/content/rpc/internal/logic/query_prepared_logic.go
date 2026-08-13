package logic

import (
	"context"

	"errx"
	"esx/app/content/rpc/internal/svc"
	"esx/app/content/rpc/pb/xiaobaihe/content/pb"

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

func (l *QueryPreparedLogic) QueryPrepared(_ *pb.QueryPreparedReq) (*pb.QueryPreparedResp, error) {
	l.Error("QueryPrepared is not part of the authoritative write path")
	return nil, errx.NewWithCode(errx.SystemError)
}
