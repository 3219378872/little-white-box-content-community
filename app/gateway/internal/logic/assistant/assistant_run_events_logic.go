package assistant

import (
	"context"
	"io"

	"esx/app/assistant/rpc/assistantservice"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/pkg/errx"
	"esx/pkg/jwtx"

	"esx/pkg/logging"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// AssistantRunEventsLogic 承载 AssistantRunEvents 接口的业务逻辑；每个请求新建一个实例。
type AssistantRunEventsLogic struct {
	logging.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewAssistantRunEventsLogic 绑定请求上下文与服务依赖，日志自动携带请求追踪信息。
func NewAssistantRunEventsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AssistantRunEventsLogic {
	return &AssistantRunEventsLogic{Logger: logging.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

// AssistantRunEvents 订阅运行事件 gRPC 流并逐条转发给 SSE 处理器；从 AfterSeq 之后续传。
func (l *AssistantRunEventsLogic) AssistantRunEvents(req *types.AssistantRunEventsReq, client chan<- *types.AssistantRunEvent) error {
	if client == nil {
		return errx.NewWithCode(errx.ServiceUnavailable)
	}
	userID, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return err
	}
	if req == nil || req.Id <= 0 {
		return errx.NewWithCode(errx.ParamError)
	}
	if l.svcCtx == nil || l.svcCtx.AssistantService == nil {
		return errx.NewWithCode(errx.ServiceUnavailable)
	}
	stream, err := l.svcCtx.AssistantService.SubscribeRunEvents(l.ctx, &assistantservice.SubscribeRunEventsReq{
		UserId: userID, RunId: req.Id, AfterSeq: req.AfterSeq,
	})
	if err != nil {
		return errx.FromRPCError(errx.FromGRPCError(err))
	}
	for {
		event, recvErr := stream.Recv()
		if recvErr == io.EOF {
			return nil
		}
		if recvErr != nil {
			// 客户端断开或上游取消属于正常结束，不再回报错误。
			if l.ctx.Err() != nil {
				return nil
			}
			if status.Code(recvErr) == codes.Canceled {
				return nil
			}
			return errx.FromRPCError(errx.FromGRPCError(recvErr))
		}
		mapped := mapRunEvent(event)
		if mapped == nil {
			continue
		}
		// 转发时同时监听 ctx，避免 SSE 写端退出后在满通道上永久阻塞。
		select {
		case <-l.ctx.Done():
			return nil
		case client <- mapped:
		}
	}
}
