package runtime

import (
	"esx/app/assistant/internal/store"
	"esx/pkg/errx"
)

// DecideDisposition 决定新消息如何并入活跃 run：无活跃 run 时新建；模型请求中重定向；
// 工具执行中作为补充输入；其余阶段排队。
func DecideDisposition(active *store.Run) string {
	if active == nil || active.ID == 0 || store.IsTerminalStatus(active.Status) {
		return store.DispositionStarted
	}
	switch active.Phase {
	case store.PhaseModelRequest:
		return store.DispositionRedirected
	case store.PhaseToolExecuting:
		return store.DispositionSteered
	case store.PhaseCompact, store.PhaseAttachment:
		return store.DispositionQueued
	default:
		return store.DispositionQueued
	}
}

// EnqueueOrReject 在排队消息达到上限时拒绝新输入。
func EnqueueOrReject(count int) error {
	if count >= store.MaxInputQueue {
		return errx.NewWithCode(errx.AgentQueueFull)
	}
	return nil
}
