package logic

import (
	"context"

	"esx/app/assistant/rpc/internal/svc"
	pb "esx/kitex_gen/assistant"

	"esx/pkg/logging"
)

// MarkThreadReadLogic 承载 MarkThreadRead 接口的业务逻辑；每个请求新建一个实例。
type MarkThreadReadLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logging.Logger
}

// NewMarkThreadReadLogic 绑定请求上下文与服务依赖，日志自动携带请求追踪信息。
func NewMarkThreadReadLogic(ctx context.Context, svcCtx *svc.ServiceContext) *MarkThreadReadLogic {
	return &MarkThreadReadLogic{ctx: ctx, svcCtx: svcCtx, Logger: logging.WithContext(ctx)}
}

// MarkThreadRead 把助手会话标记为已读。
func (l *MarkThreadReadLogic) MarkThreadRead(in *pb.MarkThreadReadReq) (*pb.MarkThreadReadResp, error) {
	if in == nil {
		return nil, requireAgentUser(0)
	}
	if err := requireAgentUser(in.UserId); err != nil {
		return nil, err
	}
	if l.svcCtx == nil || l.svcCtx.Acceptor == nil || l.svcCtx.Store == nil {
		return nil, unavailableUntilStore()
	}
	unread, err := l.svcCtx.Acceptor.MarkRead(l.ctx, in.UserId)
	if err != nil {
		return nil, err
	}
	return &pb.MarkThreadReadResp{UnreadCount: unread}, nil
}
