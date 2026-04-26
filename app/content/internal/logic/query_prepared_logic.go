package logic

import (
	"context"
	"errx"

	"esx/app/content/internal/svc"
	"esx/app/content/pb/xiaobaihe/content/pb"

	"github.com/dtm-labs/dtm/client/dtmgrpc"
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
	barrier, err := dtmgrpc.BarrierFromGrpc(l.ctx)
	if err != nil {
		l.Errorw("DTM BarrierFromGrpc failed", logx.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}
	if err := barrier.QueryPrepared(l.svcCtx.DB); err != nil {
		l.Errorw("DTM QueryPrepared failed", logx.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}

	return &pb.QueryPreparedResp{}, nil
}
