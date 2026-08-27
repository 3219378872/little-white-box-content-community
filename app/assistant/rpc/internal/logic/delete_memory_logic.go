package logic

import (
	"context"
	"errors"

	"esx/app/assistant/rpc/internal/memory"
	"esx/app/assistant/rpc/internal/svc"
	"esx/app/assistant/rpc/xiaobaihe/assistant/pb"
	"esx/pkg/errx"

	"github.com/zeromicro/go-zero/core/logx"
)

type DeleteMemoryLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewDeleteMemoryLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteMemoryLogic {
	return &DeleteMemoryLogic{ctx: ctx, svcCtx: svcCtx, Logger: logx.WithContext(ctx)}
}

func (l *DeleteMemoryLogic) DeleteMemory(in *pb.DeleteMemoryReq) (*pb.DeleteMemoryResp, error) {
	if in == nil {
		return nil, requireAgentUser(0)
	}
	if err := requireAgentUser(in.UserId); err != nil {
		return nil, err
	}
	if in.Id <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	if l.svcCtx == nil || l.svcCtx.Memory == nil {
		return nil, unavailableUntilStore()
	}
	err := l.svcCtx.Memory.Delete(l.ctx, in.UserId, in.Id)
	if errors.Is(err, memory.ErrNotFound) {
		return nil, errx.NewWithCode(errx.NotFound)
	}
	if err != nil {
		return nil, err
	}
	return &pb.DeleteMemoryResp{}, nil
}
