package runtime

import (
	"context"
	"errors"
	"esx/app/assistant/internal/canonical"
	"esx/app/assistant/internal/llm"
	"esx/app/assistant/internal/prompt"
	"esx/app/assistant/internal/store"
	"esx/app/assistant/internal/tool"
	"esx/pkg/errx"
	"strings"
	"time"

	"github.com/zeromicro/go-zero/core/logx"
)

func (s *executionState) callModel(workCtx, persistCtx context.Context) (iterationAction, error) {
	e := s.engine
	var err error

	req := llm.Request{
		SuppressText: s.suppressText,
		Messages:     s.turns,
		Tools:        s.registry.Definitions(),
		MaxTokens:    remainingOutputLimit(s.run, s.client.MaxOutputTokens()),
		Convergence:  s.convergence,
	}
	if HardLimitExceeded(s.run, store.NowMs()) || !reviewInputFits(s.run, req) {
		return iterationFinished, e.resourceLimit(persistCtx, s.run)
	}
	s.result, err = e.completeModel(workCtx, persistCtx, s.run, s.client, req)
	if err != nil {
		if errors.Is(err, errRunRedirected) {
			return iterationRestart, nil
		}
		if errors.Is(err, errRunCancelled) {
			return iterationFinished, e.cancel(persistCtx, s.run)
		}
		if aborted, abortErr := e.abortIfRequested(persistCtx, s.run); aborted {
			return iterationFinished, abortErr
		}
		if persistCtx.Err() != nil {
			return iterationFinished, err
		}
		agentLLMCalls.Inc("failure")
		logx.WithContext(persistCtx).Errorw("assistant LLM complete failed",
			logx.Field("runId", s.run.ID), logx.Field("err", err.Error()))
		if strings.TrimSpace(s.result.Text) != "" && s.run.Source == store.SourceUser {
			payload := store.EventPayload{ErrorCode: "LLM_UNAVAILABLE", Text: "模型调用失败", Partial: s.result.Text}
			return iterationFinished, e.finishWithMessageEvent(persistCtx, s.run, store.StatusError, store.EventError, payload,
				s.result.Text, prompt.EncodeTurn(prompt.Turn{Role: store.RoleAssistant, Content: s.result.Text}), !s.result.Streamed, s.result.StreamID)
		}
		return iterationFinished, e.fail(persistCtx, s.run, "LLM_UNAVAILABLE", "model call failed")
	}
	if aborted, abortErr := e.abortIfRequested(persistCtx, s.run); aborted {
		return iterationFinished, abortErr
	}
	return iterationNext, nil
}

func (s *executionState) consumeResult(workCtx, persistCtx context.Context) (iterationAction, error) {
	e := s.engine

	s.run.Rounds++
	recordModelUsage(&s.run, s.result.Usage)
	if s.result.Usage.PromptTokens > 0 {
		s.run.LastPromptTokens = s.result.Usage.PromptTokens
	}
	s.run.LastActivityAtMs = store.NowMs()
	s.result.Text = prompt.SanitizeOutput(s.result.Text)
	if aborted, abortErr := e.abortIfRequested(persistCtx, s.run); aborted {
		return iterationFinished, abortErr
	}
	if HardLimitExceeded(s.run, store.NowMs()) {
		return iterationFinished, e.resourceLimitResult(persistCtx, s.run, s.result)
	}
	if s.result.IncompleteReason != "" {
		agentLLMCalls.Inc("incomplete")
		return iterationFinished, e.incomplete(persistCtx, s.run, s.result)
	}
	agentLLMCalls.Inc("success")

	if len(s.result.ToolCalls) == 0 {
		if s.suppressText {
			return iterationFinished, e.fail(persistCtx, s.run, "ANSWER_VALIDATION_FAILED", "检索回答缺少结构化引用，未发布未经校验的结论")
		}
		text := strings.TrimSpace(s.result.Text)
		s.result.Text = text
		agentFirstToken.ObserveFloat(time.Since(s.started).Seconds())
		return iterationFinished, e.completeModelText(persistCtx, s.run, s.result)
	}

	return iterationNext, nil
}

func (s *executionState) executeCalls(workCtx, persistCtx context.Context) (iterationAction, error) {
	e := s.engine

	calls := make([]llm.ToolCall, 0, len(s.result.ToolCalls))
	assistantTurn := prompt.Turn{Role: store.RoleAssistant, Content: strings.TrimSpace(s.result.Text)}
	for i, call := range s.result.ToolCalls {
		id := strings.TrimSpace(call.ID)
		if id == "" {
			id = "call_" + itoa(store.NowMs()) + "_" + itoa(int64(i+1))
		}
		args := canonical.UnwrapArgsJSON(call.Arguments)
		sess := &tool.Session{UserID: s.run.UserID, SessionID: s.run.SessionID, RunID: s.run.ID,
			RequestID: s.run.RequestID, Source: s.run.Source, ConsentVersion: s.run.ConsentVersion, ClientProtocolVersion: clientProtocol(s.run)}
		preparedOK := false
		var prepareErr error
		if prepared, prepErr := s.registry.Prepare(workCtx, sess, call.Name, args); prepErr == nil {
			args = prepared
			preparedOK = true
		} else {
			prepareErr = prepErr
		}
		calls = append(calls, llm.ToolCall{ID: id, Name: call.Name, Arguments: args, Prepared: preparedOK, PrepareError: prepareErr})
		assistantTurn.ToolCalls = append(assistantTurn.ToolCalls, prompt.ToolCall{ID: id, Name: call.Name, Arguments: args, Prepared: preparedOK})
	}
	s.run.Phase = store.PhaseToolExecuting
	if err := e.recordModelToolStep(persistCtx, s.run, assistantTurn, &s.reviewLive); err != nil {
		return iterationFinished, err
	}
	if len(calls) > 1 && requiresExclusiveRound(calls) {
		for _, call := range calls {
			if HardLimitExceeded(s.run, store.NowMs()) {
				return iterationFinished, e.resourceLimit(persistCtx, s.run)
			}
			digest, err := canonical.DigestArgs(call.Arguments)
			if err != nil {
				digest = "invalid:" + call.ID
			}
			if _, _, err := e.startToolStep(persistCtx, s.run, call, digest, false); err != nil {
				return iterationFinished, err
			}
			problem := errx.New(errx.ParamError, "ask_questions and publish_answer require an exclusive tool round")
			if err := e.finishToolStep(persistCtx, &s.run, call, problem.Error(), problem, nil, nil, nil, true, "invalid", &s.reviewLive); err != nil {
				return iterationFinished, err
			}
		}
		for _, call := range calls {
			if err := e.guardToolProgress(persistCtx, &s.run, s.registry, call, &s.reviewLive); err != nil {
				return iterationFinished, err
			}
		}
		return iterationRestart, nil
	}
	for _, call := range calls {
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
	return iterationNext, nil
}
