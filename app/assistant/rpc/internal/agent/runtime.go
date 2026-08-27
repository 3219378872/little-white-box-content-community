package agent

import (
	"context"

	"github.com/zeromicro/go-zero/core/logx"
)

// Runtime 把 Intent / Policy / Planner 叠在既有 Runner（Executor）之上。
// 当前执行器仍是 OpenAIRunner；新工具分组落地后只改 Policy 与工具表。
type Runtime struct {
	Executor Runner
}

func NewRuntime(executor Runner) *Runtime {
	if executor == nil {
		return nil
	}
	return &Runtime{Executor: executor}
}

func (r *Runtime) Run(ctx context.Context, session *Session) (*Result, error) {
	if r == nil || r.Executor == nil {
		return nil, ErrLLMUnavailable
	}
	if session == nil {
		return nil, ErrLLMUnavailable
	}
	session.Plan = ClassifyIntent(session.UserMessage)
	session.Tools = RestrictToolsForConsent(session.Tools, session.ConsentVersion)
	logx.WithContext(ctx).Infow("agent runtime planned",
		logx.Field("intent", session.Plan.Intent),
		logx.Field("consentVersion", session.ConsentVersion),
		logx.Field("requestId", session.RequestID),
	)
	return r.Executor.Run(ctx, session)
}
