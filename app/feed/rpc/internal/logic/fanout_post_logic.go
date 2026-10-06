package logic

import (
	"context"
	"esx/app/feed/rpc/internal/fanout"
	"esx/pkg/errx"

	"esx/app/feed/rpc/internal/svc"
	pb "esx/kitex_gen/feed"

	"esx/pkg/logging"
)

// FanoutPostLogic 承载 FanoutPost 接口的业务逻辑；每个请求新建一个实例。
type FanoutPostLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logging.Logger
}

// NewFanoutPostLogic 绑定请求上下文与服务依赖，日志自动携带请求追踪信息。
func NewFanoutPostLogic(ctx context.Context, svcCtx *svc.ServiceContext) *FanoutPostLogic {
	return &FanoutPostLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logging.WithContext(ctx),
	}
}

// FanoutPost 手动触发一篇帖子的 fanout，返回新增的 inbox 行数；常规路径由 feed-consumer 消费 MQ 完成。
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
		l.Errorw("FanoutPost failed", logging.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}

	return &pb.FanoutPostResp{PushedCount: pushed}, nil
}
