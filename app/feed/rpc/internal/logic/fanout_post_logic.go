package logic

import (
	"context"
	"errx"
	"esx/app/feed/rpc/internal/fanout"

	"esx/app/feed/rpc/internal/svc"
	"esx/app/feed/rpc/xiaobaihe/feed/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type FanoutPostLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewFanoutPostLogic(ctx context.Context, svcCtx *svc.ServiceContext) *FanoutPostLogic {
	return &FanoutPostLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *FanoutPostLogic) FanoutPost(in *pb.FanoutPostReq) (*pb.FanoutPostResp, error) {
	if in.AuthorId <= 0 || in.PostId <= 0 || in.CreatedAt <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	pushed, err := fanout.HandlePostPublished(l.ctx, l.svcCtx, fanout.PostPublished{
		AuthorId:  in.AuthorId,
		PostId:    in.PostId,
		CreatedAt: in.CreatedAt,
	})
	if err != nil {
		l.Errorw("FanoutPost failed", logx.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}

	return &pb.FanoutPostResp{PushedCount: pushed}, nil
}
