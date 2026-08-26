package logic

import (
	"context"
	"errors"

	"esx/app/user/rpc/internal/model"
	"esx/app/user/rpc/internal/svc"
	"esx/app/user/rpc/pb/xiaobaihe/user/pb"
	"esx/pkg/errx"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetAgentCapabilityConsentLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetAgentCapabilityConsentLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetAgentCapabilityConsentLogic {
	return &GetAgentCapabilityConsentLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// 查询 Agent 能力授权状态（AGNT-004/006）
func (l *GetAgentCapabilityConsentLogic) GetAgentCapabilityConsent(in *pb.GetAgentCapabilityConsentReq) (*pb.GetAgentCapabilityConsentResp, error) {
	if in == nil || in.UserId <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	if l.svcCtx.AgentConsent == nil {
		return nil, errx.NewWithCode(errx.SystemError)
	}
	consent, err := l.svcCtx.AgentConsent.Get(l.ctx, in.UserId)
	if err != nil {
		if errors.Is(err, model.ErrAgentCapabilityConsentNotFound) {
			// 默认未授权
			return &pb.GetAgentCapabilityConsentResp{Granted: false}, nil
		}
		l.Errorw("AgentConsent.Get failed", logx.Field("user_id", in.UserId), logx.Field("err", err.Error()))
		return nil, errx.Wrap(err, errx.SystemError)
	}
	return &pb.GetAgentCapabilityConsentResp{
		Granted:   consent.Granted,
		GrantedAt: consent.GrantedAt.Int64,
		RevokedAt: consent.RevokedAt.Int64,
	}, nil
}
