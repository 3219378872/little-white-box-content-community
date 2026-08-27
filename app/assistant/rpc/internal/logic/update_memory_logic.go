package logic

import (
	"context"
	"errors"
	"time"

	"esx/app/assistant/rpc/internal/memory"
	"esx/app/assistant/rpc/internal/svc"
	"esx/app/assistant/rpc/xiaobaihe/assistant/pb"
	"esx/pkg/errx"

	"github.com/zeromicro/go-zero/core/logx"
)

type UpdateMemoryLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewUpdateMemoryLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateMemoryLogic {
	return &UpdateMemoryLogic{ctx: ctx, svcCtx: svcCtx, Logger: logx.WithContext(ctx)}
}

func (l *UpdateMemoryLogic) UpdateMemory(in *pb.UpdateMemoryReq) (*pb.UpdateMemoryResp, error) {
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
	err := l.svcCtx.Memory.Update(l.ctx, in.UserId, in.Id, memory.Patch{
		Value:      in.Value,
		Score:      in.Score,
		Suppressed: in.Suppressed,
	}, time.Now())
	if errors.Is(err, memory.ErrNotFound) {
		return nil, errx.NewWithCode(errx.NotFound)
	}
	if err != nil {
		return nil, err
	}
	return &pb.UpdateMemoryResp{}, nil
}
