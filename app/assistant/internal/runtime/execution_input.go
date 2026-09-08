package runtime

import (
	"context"
	"errors"
	"esx/app/assistant/internal/prompt"
	"esx/app/assistant/internal/store"
	"esx/app/assistant/internal/tool"
)

func (s *executionState) loadRound(workCtx, persistCtx context.Context) (iterationAction, error) {
	e := s.engine

	fresh, err := e.ownedRun(persistCtx, s.run)
	if err != nil {
		return iterationFinished, err
	}
	s.run = *fresh
	if err := e.requireFrozenConsent(persistCtx, &s.run); err != nil {
		return iterationFinished, err
	}
	if aborted, abortErr := e.abortIfRequested(persistCtx, s.run); aborted {
		return iterationFinished, abortErr
	}
	s.now = store.NowMs()
	if HardLimitExceeded(s.run, s.now) {
		return iterationFinished, e.resourceLimit(persistCtx, s.run)
	}
	s.convergence = ""
	if err := e.step(persistCtx, s.run, func(ctx context.Context, tx store.Store) error {
		var err error
		s.convergence, err = RecordAlarms(ctx, tx, s.run, s.now)
		return err
	}); err != nil {
		return iterationFinished, err
	}

	if err := e.ensureWatchInput(persistCtx, s.run); err != nil {
		return iterationFinished, err
	}
	s.messages, err = e.Store.ListSessionMessages(persistCtx, s.run.UserID, s.run.SessionID, true)
	if err != nil {
		return iterationFinished, err
	}
	return iterationNext, nil
}

func (s *executionState) compactRound(workCtx, persistCtx context.Context) (iterationAction, error) {
	e := s.engine
	var err error

	window := e.Window
	if s.client != nil && s.client.ContextWindowTokens() > 0 {
		window = s.client.ContextWindowTokens()
	}

	if ShouldCompactWithAnchor(s.messages, window, s.run.ID, s.run.LastPromptTokens) {
		if err := e.compact(workCtx, persistCtx, &s.run, s.session, s.messages, s.client); err != nil {
			if errors.Is(err, errRunCancelled) {
				return iterationFinished, e.cancel(persistCtx, s.run)
			}
			if errors.Is(err, errCompactNoGain) {
				if finishErr := e.fail(persistCtx, s.run, "COMPACT_NO_GAIN", "会话压缩未能降低上下文"); finishErr != nil {
					return iterationFinished, finishErr
				}
				return iterationFinished, errRunTerminated
			}
			return iterationFinished, err
		}
		s.session, err = e.Store.GetSession(persistCtx, s.run.SessionID)
		if err != nil {
			return iterationFinished, err
		}
		var decoded bool
		s.snapshot, decoded = prompt.DecodeSnapshot(s.session.PromptSnapshot)
		if !decoded {
			return iterationFinished, errors.New("compacted prompt snapshot is invalid")
		}
		s.registry, s.client, err = e.loadCapabilities(persistCtx, s.run, s.session)
		if err != nil {
			return iterationFinished, err
		}
		if s.run.Source == store.SourceMemoryReview && e.ReviewLLM != nil {
			s.client = e.ReviewLLM
		}
		return iterationRestart, nil
	}

	return iterationNext, nil
}

func (s *executionState) prepareTurns(workCtx, persistCtx context.Context) (iterationAction, error) {
	e := s.engine
	var err error

	history := promptHistory(s.messages, s.run)
	if s.run.Source == store.SourceMemoryReview {
		history = append(history, s.reviewLive...)
	} else if open := unmatchedToolCalls(history); len(open) > 0 {
		for _, call := range open {
			if aborted, abortErr := e.abortIfRequested(persistCtx, s.run); aborted {
				return iterationFinished, abortErr
			}
			if err := e.execTool(workCtx, persistCtx, &s.run, s.registry, call, &s.reviewLive); err != nil {
				if errors.Is(err, errRunCancelled) {
					return iterationFinished, e.cancel(persistCtx, s.run)
				}
				return iterationFinished, err
			}
		}
		s.messages, err = e.Store.ListSessionMessages(persistCtx, s.run.UserID, s.run.SessionID, true)
		if err != nil {
			return iterationFinished, err
		}
		history = promptHistory(s.messages, s.run)
	}
	s.snapshot.History = history
	s.turns = prompt.Messages(s.snapshot)
	seen := make(map[int64]struct{}, len(s.messages))
	for _, msg := range s.messages {
		if !msg.Compacted {
			seen[msg.ID] = struct{}{}
		}
	}
	pending, queuedThrough, err := e.pendingUserTurns(persistCtx, s.run, seen, history)
	if err != nil {
		return iterationFinished, err
	}
	s.turns = append(s.turns, pending...)
	if queuedThrough > 0 {
		if err := e.step(persistCtx, s.run, func(ctx context.Context, tx store.Store) error {
			return tx.DeleteQueueThrough(ctx, s.run.ID, queuedThrough)
		}); err != nil {
			return iterationFinished, err
		}
	}

	return iterationNext, nil
}

func (s *executionState) prepareRequest(workCtx, persistCtx context.Context) (iterationAction, error) {
	e := s.engine

	s.run.Phase = store.PhaseModelRequest
	s.run.LastActivityAtMs = s.now
	if err := e.updateRun(persistCtx, s.run); err != nil {
		return iterationFinished, err
	}

	if s.client == nil {
		return iterationFinished, e.fail(persistCtx, s.run, "LLM_DISABLED", "model is not configured")
	}
	s.suppressText = false
	if s.registry.Has(tool.PublishAnswer) {
		sources, sourceErr := e.Store.ListSources(persistCtx, s.run.ID)
		if sourceErr != nil {
			return iterationFinished, sourceErr
		}
		s.suppressText = s.run.Source == store.SourceWatch || len(sources) > 0
	}
	return iterationNext, nil
}
