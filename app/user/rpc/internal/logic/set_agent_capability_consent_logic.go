package logic

import (
	"context"
	"database/sql"
	"time"

	"esx/app/user/rpc/internal/model"
	"esx/app/user/rpc/internal/svc"
	pb "esx/kitex_gen/user"
	"esx/pkg/errx"

	"esx/pkg/logging"
)

// SetAgentCapabilityConsentLogic 承载 SetAgentCapabilityConsent 接口的业务逻辑；每个请求新建一个实例。
type SetAgentCapabilityConsentLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logging.Logger
}

// NewSetAgentCapabilityConsentLogic 绑定请求上下文与服务依赖，日志自动携带请求追踪信息。
func NewSetAgentCapabilityConsentLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SetAgentCapabilityConsentLogic {
	return &SetAgentCapabilityConsentLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logging.WithContext(ctx),
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
		consent.ConsentVersion = model.CurrentAgentConsentVersion
	} else {
		consent.RevokedAt = sql.NullInt64{Int64: nowMilli, Valid: true}
		consent.ConsentVersion = 0
	}
	if err := l.svcCtx.AgentConsent.Upsert(l.ctx, consent); err != nil {
		l.Errorw("AgentConsent.Upsert failed", logging.Field("user_id", in.UserId), logging.Field("granted", in.Granted), logging.Field("err", err.Error()))
		return nil, errx.Wrap(err, errx.SystemError)
	}
	return &pb.SetAgentCapabilityConsentResp{}, nil
}
