package logic

import (
	"context"
	"database/sql"
	"time"

	"esx/app/user/rpc/internal/model"
	"esx/app/user/rpc/internal/svc"
	"esx/app/user/rpc/pb/xiaobaihe/user/pb"
	"esx/pkg/errx"

	"github.com/zeromicro/go-zero/core/logx"
)

type SetAgentCapabilityConsentLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewSetAgentCapabilityConsentLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SetAgentCapabilityConsentLogic {
	return &SetAgentCapabilityConsentLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// 记录或撤销 Agent 能力授权（AGNT-004/006）
func (l *SetAgentCapabilityConsentLogic) SetAgentCapabilityConsent(in *pb.SetAgentCapabilityConsentReq) (*pb.SetAgentCapabilityConsentResp, error) {
	if in == nil || in.UserId <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	if l.svcCtx.AgentConsent == nil {
		return nil, errx.NewWithCode(errx.SystemError)
	}
	nowMilli := time.Now().UnixMilli()
	consent := &model.AgentCapabilityConsent{
		UserID:  in.UserId,
		Granted: in.Granted,
	}
	if in.Granted {
		consent.GrantedAt = sql.NullInt64{Int64: nowMilli, Valid: true}
	} else {
		consent.RevokedAt = sql.NullInt64{Int64: nowMilli, Valid: true}
	}
	if err := l.svcCtx.AgentConsent.Upsert(l.ctx, consent); err != nil {
		l.Errorw("AgentConsent.Upsert failed", logx.Field("user_id", in.UserId), logx.Field("granted", in.Granted), logx.Field("err", err.Error()))
		return nil, errx.Wrap(err, errx.SystemError)
	}
	return &pb.SetAgentCapabilityConsentResp{}, nil
}
