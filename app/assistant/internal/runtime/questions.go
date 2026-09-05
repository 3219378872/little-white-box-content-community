package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"esx/app/assistant/internal/canonical"
	"esx/app/assistant/internal/llm"
	"esx/app/assistant/internal/prompt"
	"esx/app/assistant/internal/store"
	"esx/app/assistant/internal/tool"
	"esx/pkg/errx"
)

var errRunWaiting = errors.New("run yielded for user input")

type QuestionContext struct {
	RunID             int64                  `json:"runId"`
	QuestionRequestID string                 `json:"questionRequestId"`
	Answers           []store.QuestionAnswer `json:"answers"`
}

func (e *Engine) waitForQuestions(ctx context.Context, run *store.Run, call llm.ToolCall, question store.QuestionRequest) error {
	now := store.NowMs()
	question.CreatedAtMs = now
	question.DeadlineMs = now + HardIdle.Milliseconds()
	if run.StartedAtMs > 0 {
		question.DeadlineMs = min(question.DeadlineMs, run.StartedAtMs+HardAbsolute.Milliseconds())
	}
	question.Answers = []store.QuestionAnswer{}
	err := e.step(ctx, *run, func(ctx context.Context, tx store.Store) error {
		questions, err := tx.ListQuestions(ctx, run.ID)
		if err != nil {
			return err
		}
		for _, old := range questions {
			if old.CallID == call.ID {
				return errRunWaiting
			}
		}
		var text strings.Builder
		for i, q := range question.Questions {
			if i > 0 {
				text.WriteString("\n\n")
			}
			text.WriteString(q.Text)
			for _, option := range q.Options {
				fmt.Fprintf(&text, "\n- %s", option.Label)
			}
		}
		msg, err := tx.InsertMessage(ctx, store.Message{UserID: run.UserID, SessionID: run.SessionID, RunID: run.ID, Role: store.RoleAssistant, Kind: store.KindQuestion, Content: text.String(), Visible: true, CreatedAtMs: now})
		if err != nil {
			return err
		}
		question.MessageID = msg.ID
		if err := tx.SaveQuestion(ctx, question); err != nil {
			return err
		}
		if err := insertMessageOutbox(ctx, tx, msg); err != nil {
			return err
		}
		run.Status = store.StatusWaitingInput
		run.Phase = store.PhaseWaitingInput
		run.LastActivityAtMs = now
		run.ToolCalls++
		if err := tx.UpdateRun(ctx, *run); err != nil {
			return err
		}
		if err := tx.UpdateToolCall(ctx, store.ToolCall{RunID: run.ID, CallID: call.ID, Status: store.StatusWaitingInput}); err != nil {
			return err
		}
		thread, err := tx.LockThread(ctx, run.UserID)
		if err != nil {
			return err
		}
		thread.LastMessageID = msg.ID
		thread.LastMessagePreview = store.Preview(msg.Content, 80)
		thread.LastMessageAtMs = now
		thread.UpdatedAtMs = now
		if err := tx.SaveThread(ctx, *thread); err != nil {
			return err
		}
		_, err = AppendEvent(ctx, tx, nil, *run, store.EventQuestionsRequired, store.EventPayload{Question: &question})
		return err
	})
	if err != nil {
		return err
	}
	e.wake(ctx, run.ID)
	return errRunWaiting
}

func ValidateAnswers(questions []store.Question, answers []store.QuestionAnswer) ([]store.QuestionAnswer, error) {
	if len(answers) != len(questions) {
		return nil, errx.New(errx.ParamError, "each question requires an explicit disposition")
	}
	byID := map[string]store.QuestionAnswer{}
	for _, answer := range answers {
		if _, exists := byID[answer.QuestionID]; exists {
			return nil, errx.New(errx.ParamError, "duplicate question answer")
		}
		byID[answer.QuestionID] = answer
	}
	result := make([]store.QuestionAnswer, 0, len(questions))
	total := 0
	for _, question := range questions {
		answer, ok := byID[question.ID]
		if !ok {
			return nil, errx.New(errx.ParamError, "answer belongs to another question")
		}
		answer.Text = strings.TrimSpace(answer.Text)
		total += utf8.RuneCountInString(answer.Text)
		if total > 2000 {
			return nil, errx.New(errx.ParamError, "answer text exceeds 2000 characters")
		}
		switch answer.Disposition {
		case "answered":
			if len(answer.SelectedOptionIDs) == 0 && answer.Text == "" {
				return nil, errx.New(errx.ParamError, "empty answer")
			}
		case "unknown", "no_preference", "skipped":
			if len(answer.SelectedOptionIDs) > 0 {
				return nil, errx.New(errx.ParamError, "unknown or skipped answers cannot select options")
			}
		default:
			return nil, errx.New(errx.ParamError, "invalid answer disposition")
		}
		if question.Selection == "single" && len(answer.SelectedOptionIDs) > 1 {
			return nil, errx.New(errx.ParamError, "single choice accepts one option")
		}
		allowed := map[string]bool{}
		for _, option := range question.Options {
			allowed[option.ID] = true
		}
		seen := map[string]bool{}
		for _, id := range answer.SelectedOptionIDs {
			if !allowed[id] || seen[id] {
				return nil, errx.New(errx.ParamError, "invalid or duplicate option")
			}
			seen[id] = true
		}
		answer.SelectedOptionIDs = append([]string{}, answer.SelectedOptionIDs...)
		sort.Strings(answer.SelectedOptionIDs)
		result = append(result, answer)
	}
	return result, nil
}

func closeQuestionTx(ctx context.Context, tx store.Store, run store.Run, q *store.QuestionRequest, status string) error {
	q.Status = status
	if err := tx.SaveQuestion(ctx, *q); err != nil {
		return err
	}
	result := string(mustJSON(map[string]any{"status": status, "questions": q.Questions, "answers": q.Answers}))
	turn := prompt.Turn{Role: store.RoleTool, Name: tool.AskQuestions, ToolCallID: q.CallID, Content: result}
	if _, err := tx.InsertMessage(ctx, store.Message{UserID: run.UserID, SessionID: run.SessionID, RunID: run.ID, Role: store.RoleTool, Kind: store.KindTool, Content: result, APIContent: prompt.EncodeTurn(turn), Visible: false, CreatedAtMs: store.NowMs()}); err != nil {
		return err
	}
	if err := tx.UpdateToolCall(ctx, store.ToolCall{RunID: run.ID, CallID: q.CallID, Status: status, ResultJSON: encodeToolResultJSONWithChanges(result, nil, nil)}); err != nil {
		return err
	}
	if _, err := AppendEvent(ctx, tx, nil, run, store.EventToolResult, store.EventPayload{ToolCall: &store.ToolInfo{CallID: q.CallID, Tool: tool.AskQuestions, Summary: status}}); err != nil {
		return err
	}
	_, err := AppendEvent(ctx, tx, nil, run, store.EventQuestionsResolved, store.EventPayload{Question: q})
	return err
}

func AnswerQuestions(ctx context.Context, st store.Store, notify store.Notifier, userID, runID int64, questionID, requestID string, answers []store.QuestionAnswer) (*store.QuestionRequest, error) {
	if userID <= 0 || runID <= 0 || questionID == "" || strings.TrimSpace(requestID) == "" || len(requestID) > 64 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	var out *store.QuestionRequest
	err := st.Transact(ctx, func(ctx context.Context, tx store.Store) error {
		run, err := tx.LockRun(ctx, runID)
		if err != nil || run.UserID != userID {
			return errx.NewWithCode(errx.NotFound)
		}
		version, granted, err := tx.AgentConsent(ctx, userID)
		if err != nil {
			return err
		}
		if !granted || version != run.ConsentVersion {
			return errx.NewWithCode(errx.AgentNotAuthorized)
		}
		questions, err := tx.ListQuestions(ctx, runID)
		if err != nil {
			return err
		}
		for _, question := range questions {
			if question.ID != questionID {
				continue
			}
			values, err := ValidateAnswers(question.Questions, answers)
			if err != nil {
				return err
			}
			digest, err := canonical.DigestArgs(string(mustJSON(values)))
			if err != nil {
				return err
			}
			if question.AnswerRequestID == requestID && question.AnswerDigest == digest {
				copy := question
				out = &copy
				return nil
			}
			if question.Status != "pending" || run.Status != store.StatusWaitingInput || run.CancelRequested || store.NowMs() >= question.DeadlineMs {
				return errx.New(errx.ContentVersionConflict, "question is resolved, expired or cancelled")
			}
			question.Answers = values
			question.AnswerRequestID = requestID
			question.AnswerDigest = digest
			if err := closeQuestionTx(ctx, tx, *run, &question, "answered"); err != nil {
				return err
			}
			run.Status = store.StatusQueued
			run.Phase = store.PhaseQueued
			run.LastActivityAtMs = store.NowMs()
			if err := tx.UpdateRun(ctx, *run); err != nil {
				return err
			}
			copy := question
			out = &copy
			return nil
		}
		return errx.NewWithCode(errx.NotFound)
	})
	if err == nil && notify != nil {
		_ = notify.Wake(ctx, runID)
	}
	return out, err
}

func supersedeQuestionsTx(ctx context.Context, tx store.Store, run *store.Run) error {
	questions, err := tx.ListQuestions(ctx, run.ID)
	if err != nil {
		return err
	}
	for i := range questions {
		if questions[i].Status == "pending" {
			if err := closeQuestionTx(ctx, tx, *run, &questions[i], "superseded"); err != nil {
				return err
			}
		}
	}
	run.Status = store.StatusQueued
	run.Phase = store.PhaseQueued
	run.LastActivityAtMs = store.NowMs()
	return tx.UpdateRun(ctx, *run)
}

func ResolveWaiting(ctx context.Context, st store.Store, notify store.Notifier, runID, now int64) error {
	err := st.Transact(ctx, func(ctx context.Context, tx store.Store) error {
		run, err := tx.LockRun(ctx, runID)
		if err != nil {
			return err
		}
		if run.Status != store.StatusWaitingInput {
			return nil
		}
		version, granted, err := tx.AgentConsent(ctx, run.UserID)
		if err != nil {
			return err
		}
		cancelled := run.CancelRequested || !granted || version != run.ConsentVersion
		questions, err := tx.ListQuestions(ctx, runID)
		if err != nil {
			return err
		}
		var pending *store.QuestionRequest
		for i := range questions {
			if questions[i].Status == "pending" {
				pending = &questions[i]
				break
			}
		}
		if pending == nil {
			return errx.New(errx.SystemError, "waiting question missing")
		}
		if !cancelled && now < pending.DeadlineMs {
			return nil
		}
		status, code, text := "expired", "AGENT_RESOURCE_LIMIT", "等待回答已到达运行时限"
		run.Status = store.StatusError
		if cancelled {
			status, code, text = "cancelled", "CANCELLED", "已停止"
			run.Status = store.StatusCancelled
		}
		if err := closeQuestionTx(ctx, tx, *run, pending, status); err != nil {
			return err
		}
		run.Phase = store.PhaseDone
		run.ErrorCode = code
		run.EndedAtMs = now
		if err := tx.UpdateRun(ctx, *run); err != nil {
			return err
		}
		thread, err := tx.LockThread(ctx, run.UserID)
		if err != nil {
			return err
		}
		if thread.ActiveRunID == run.ID {
			thread.ActiveRunID = 0
		}
		thread.UpdatedAtMs = now
		if err := tx.SaveThread(ctx, *thread); err != nil {
			return err
		}
		_, err = AppendEvent(ctx, tx, nil, *run, store.EventError, store.EventPayload{ErrorCode: code, Text: text})
		return err
	})
	if err == nil && notify != nil {
		_ = notify.Wake(ctx, runID)
	}
	return err
}

func (a *Acceptor) questionContext(ctx context.Context, userID int64, context *QuestionContext) (string, error) {
	if context == nil {
		return "", nil
	}
	run, err := a.Store.GetRun(ctx, context.RunID)
	if err != nil || run.UserID != userID {
		return "", errx.NewWithCode(errx.NotFound)
	}
	if !store.IsTerminalStatus(run.Status) {
		return "", errx.New(errx.ContentVersionConflict, "original run is still active")
	}
	questions, err := a.Store.ListQuestions(ctx, run.ID)
	if err != nil {
		return "", err
	}
	for _, q := range questions {
		if q.ID == context.QuestionRequestID {
			msg, err := a.Store.GetMessage(ctx, userID, q.MessageID)
			if err != nil || msg.DeletedAtMs > 0 {
				return "", errx.NewWithCode(errx.NotFound)
			}
			answers, err := ValidateAnswers(q.Questions, context.Answers)
			if err != nil {
				return "", err
			}
			value := struct {
				Questions []store.Question       `json:"questions"`
				Answers   []store.QuestionAnswer `json:"answers"`
			}{q.Questions, answers}
			raw, err := json.Marshal(value)
			return string(raw), err
		}
	}
	return "", errx.NewWithCode(errx.NotFound)
}
