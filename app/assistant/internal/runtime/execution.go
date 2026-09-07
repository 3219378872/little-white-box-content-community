package runtime

import (
	"context"
	"esx/app/assistant/internal/llm"
	"esx/app/assistant/internal/memory"
	"esx/app/assistant/internal/prompt"
	"esx/app/assistant/internal/store"
	"esx/app/assistant/internal/tool"
	"time"
)

type iterationAction int

const (
	iterationNext iterationAction = iota
	iterationRestart
	iterationFinished
)

// Each Execute owns its mutable prompt, tool and model-round state.
type executionState struct {
	engine       *Engine
	run          store.Run
	session      *store.Session
	snapshot     prompt.Snapshot
	registry     *tool.Registry
	client       llm.Client
	reviewLive   []prompt.Turn
	started      time.Time
	now          int64
	convergence  string
	messages     []store.Message
	turns        []prompt.Turn
	suppressText bool
	result       llm.Result
}

func (e *Engine) prepareExecution(persistCtx context.Context, run store.Run) (*executionState, error) {
	if err := e.requireFrozenConsent(persistCtx, &run); err != nil {
		return nil, err
	}
	session, err := e.Store.GetSession(persistCtx, run.SessionID)
	if err != nil {
		return nil, e.fail(persistCtx, run, "SESSION_MISSING", err.Error())
	}
	snap, ok := prompt.DecodeSnapshot(session.PromptSnapshot)
	if !ok {
		var entries []memory.Entry
		if e.Memory != nil {
			entries, err = e.Memory.Active(persistCtx, run.UserID)
			if err != nil {
				return nil, err
			}
		}
		snap = prompt.BuildSnapshot(entries, nil, session.CompactSummary)
		session.PromptSnapshot = prompt.EncodeSnapshot(snap)
		if err := e.step(persistCtx, run, func(ctx context.Context, tx store.Store) error {
			return tx.UpdateSession(ctx, *session)
		}); err != nil {
			return nil, err
		}
	}
	if err := e.ensureStarted(persistCtx, run); err != nil {
		return nil, err
	}
	started := time.Now()
	registry, modelClient, err := e.loadCapabilities(persistCtx, run, session)
	if err != nil {
		return nil, err
	}
	if run.Source == store.SourceMemoryReview && e.ReviewLLM != nil {
		modelClient = e.ReviewLLM
	}

	return &executionState{engine: e, run: run, session: session, snapshot: snap, registry: registry, client: modelClient, started: started}, nil
}

func (s *executionState) iterate(workCtx, persistCtx context.Context) (iterationAction, error) {
	if action, err := s.loadRound(workCtx, persistCtx); action != iterationNext || err != nil {
		return action, err
	}
	if action, err := s.compactRound(workCtx, persistCtx); action != iterationNext || err != nil {
		return action, err
	}
	if action, err := s.prepareTurns(workCtx, persistCtx); action != iterationNext || err != nil {
		return action, err
	}
	if action, err := s.prepareRequest(workCtx, persistCtx); action != iterationNext || err != nil {
		return action, err
	}
	if action, err := s.callModel(workCtx, persistCtx); action != iterationNext || err != nil {
		return action, err
	}
	if action, err := s.consumeResult(workCtx, persistCtx); action != iterationNext || err != nil {
		return action, err
	}
	return s.executeCalls(workCtx, persistCtx)
}
