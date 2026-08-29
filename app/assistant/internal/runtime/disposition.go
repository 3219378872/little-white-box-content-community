package runtime

import (
	"esx/app/assistant/internal/store"
	"esx/pkg/errx"
)

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

func EnqueueOrReject(count int) error {
	if count >= store.MaxInputQueue {
		return errx.NewWithCode(errx.AgentQueueFull)
	}
	return nil
}
