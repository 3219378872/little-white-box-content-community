package agent

import (
	"context"
	"time"

	"esx/app/assistant/rpc/internal/memory"

	"github.com/zeromicro/go-zero/core/logx"
)

// Runtime 把 Intent / Policy / Planner 叠在既有 Runner（Executor）之上。
// 当前执行器仍是 OpenAIRunner；新工具分组落地后只改 Policy 与工具表。
type Runtime struct {
	Executor Runner
	Memory   memory.Store
}

func NewRuntime(executor Runner, store memory.Store) *Runtime {
	if executor == nil {
		return nil
	}
	return &Runtime{Executor: executor, Memory: store}
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
	if r.Memory != nil && session.UserID > 0 {
		if block, err := r.Memory.ContextBlock(ctx, session.UserID, session.Plan.Intent, time.Now()); err != nil {
			logx.WithContext(ctx).Infow("agent memory context skipped", logx.Field("err", err.Error()))
		} else if block != "" {
			if session.SystemPrompt != "" {
				session.SystemPrompt += "\n\n"
			}
			session.SystemPrompt += block
		}
	}
	logx.WithContext(ctx).Infow("agent runtime planned",
		logx.Field("intent", session.Plan.Intent),
		logx.Field("consentVersion", session.ConsentVersion),
		logx.Field("requestId", session.RequestID),
	)
	result, err := r.Executor.Run(ctx, session)
	if err == nil && r.Memory != nil && session.UserID > 0 {
		r.persistMemory(session)
	}
	return result, err
}

func (r *Runtime) persistMemory(session *Session) {
	candidates := memory.Extract(session.UserMessage)
	if len(candidates) == 0 {
		return
	}
	store := r.Memory
	userID := session.UserID
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		now := time.Now()
		for _, candidate := range candidates {
			if err := store.Apply(ctx, userID, candidate, now); err != nil {
				logx.Errorw("agent memory apply failed", logx.Field("err", err.Error()))
			}
		}
	}()
}
