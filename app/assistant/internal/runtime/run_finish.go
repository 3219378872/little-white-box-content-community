package runtime

import (
	"context"
	"esx/app/assistant/internal/llm"
	"esx/app/assistant/internal/prompt"
	"esx/app/assistant/internal/store"
	"strconv"
	"strings"
)

func (e *Engine) ensureStarted(ctx context.Context, run store.Run) error {
	seq, err := e.Store.MaxEventSeq(ctx, run.ID)
	if err != nil {
		return err
	}
	if seq == 0 {
		_, err = e.appendEvent(ctx, run, store.EventRunStarted, store.EventPayload{})
	}
	return err
}

func (e *Engine) resourceLimit(ctx context.Context, run store.Run) error {
	return e.resourceLimitResult(ctx, run, llm.Result{})
}

func (e *Engine) stopAtResourceLimit(ctx context.Context, run store.Run) error {
	if err := e.resourceLimit(ctx, run); err != nil {
		return err
	}
	return errRunTerminated
}

func (e *Engine) resourceLimitResult(ctx context.Context, run store.Run, result llm.Result) error {
	journals, err := e.Store.ListSuccessfulJournal(ctx, run.UserID, run.RequestID)
	if err != nil {
		return err
	}
	summary := make([]string, 0, len(journals))
	for _, row := range journals {
		summary = append(summary, row.Tool)
	}
	payload := store.EventPayload{
		ErrorCode: "AGENT_RESOURCE_LIMIT", Text: "资源预算已耗尽", Journal: strings.Join(summary, ","),
	}
	if text := strings.TrimSpace(prompt.SanitizeOutput(result.Text)); text != "" && run.Source == store.SourceUser {
		payload.Partial = text
		return e.finishMessage(ctx, run, terminalOutcome{
			status: store.StatusError, eventType: store.EventError, payload: payload,
		}.withAssistantText(text, result))
	}
	return e.finish(ctx, run, store.StatusError, store.EventError, payload)
}

func (e *Engine) fail(ctx context.Context, run store.Run, code, text string) error {
	return e.finish(ctx, run, store.StatusError, store.EventError, store.EventPayload{ErrorCode: code, Text: text})
}

func (e *Engine) cancel(ctx context.Context, run store.Run) error {
	run.CancelRequested = true
	return e.finish(ctx, run, store.StatusCancelled, store.EventError, store.EventPayload{ErrorCode: "CANCELLED", Text: "run cancelled"})
}

// finish 结束 run 且不发布助手消息（失败、取消等）。
func (e *Engine) finish(ctx context.Context, run store.Run, status, eventType string, payload store.EventPayload) error {
	return e.finishMessage(ctx, run, terminalOutcome{status: status, eventType: eventType, payload: payload, emitToken: true})
}

// terminalOutcome 描述 run 的终态，以及随终态一起发布的助手消息（message 为空时不发布）。
type terminalOutcome struct {
	status    string
	eventType string
	payload   store.EventPayload
	// message 是可见的助手消息正文；apiContent 为空时按 message 生成模型侧内容。
	message    string
	apiContent []byte
	// emitToken 为 true 时补发一次完整 token 事件；已流式推送过的结果不再重复推送。
	emitToken bool
	streamID  string
	// before 在终态事务内、写入终态之前执行；被取消抢先时跳过。
	before func(context.Context, store.Store) error
}

// withAssistantText 让终态附带模型产出的助手文本（完整回答或失败前的部分回答）。
func (o terminalOutcome) withAssistantText(text string, result llm.Result) terminalOutcome {
	o.message = text
	o.apiContent = prompt.EncodeTurn(prompt.Turn{Role: store.RoleAssistant, Content: text})
	o.emitToken = !result.Streamed
	o.streamID = result.StreamID
	return o
}

// cancelledOutcome 是取消抢先提交时替换掉原终态的结果：不发布消息，也不执行 before。
func cancelledOutcome() terminalOutcome {
	return terminalOutcome{
		status: store.StatusCancelled, eventType: store.EventError,
		payload: store.EventPayload{ErrorCode: "CANCELLED", Text: "run cancelled"},
	}
}

// finishMessage 在一个事务内写入 run 终态、终态消息、线程摘要与终态事件。
// 事务内复读 run：若用户已请求取消，则取消胜出；模型阶段的结果若遇到已接受的改写输入则让位。
func (e *Engine) finishMessage(ctx context.Context, run store.Run, out terminalOutcome) error {
	modelResult := run.Phase == store.PhaseModelRequest
	now := store.NowMs()
	// 预算耗尽时即使模型给出了回答也按资源上限结束，回答作为部分结果保留。
	if out.status == store.StatusDone && HardLimitExceeded(run, now) {
		return e.resourceLimitResult(ctx, run, llm.Result{Text: out.message, Streamed: !out.emitToken, StreamID: out.streamID})
	}
	run.Status = out.status
	run.Phase = store.PhaseDone
	run.EndedAtMs = now
	run.LastActivityAtMs = now
	if out.status == store.StatusCancelled {
		run.CancelRequested = true
	}
	if out.payload.ErrorCode != "" {
		run.ErrorCode = out.payload.ErrorCode
	}
	err := e.step(ctx, run, func(ctx context.Context, tx store.Store) error {
		fresh, err := tx.GetRun(ctx, run.ID)
		if err != nil {
			return err
		}
		if out.status != store.StatusCancelled && fresh.CancelRequested {
			out = cancelledOutcome()
			run.Status = store.StatusCancelled
			run.CancelRequested = true
			run.ErrorCode = "CANCELLED"
		}
		// A model-stage terminal response must lose to an accepted redirect.
		// Tool-stage completions keep their established steer/recovery semantics.
		if modelResult && out.status != store.StatusCancelled && fresh.InputVersion != run.InputVersion {
			return errRunRedirected
		}
		// 非成功终态要把仍在运行的工具调用一并关闭。
		if out.status != store.StatusDone {
			if err := closePendingCalls(ctx, tx, run, out.status); err != nil {
				return err
			}
		}
		if out.before != nil {
			if err := out.before(ctx, tx); err != nil {
				return err
			}
		}
		if err := tx.UpdateRun(ctx, run); err != nil {
			return err
		}
		thread, err := tx.LockThread(ctx, run.UserID)
		if err != nil {
			return err
		}
		if run.Source == store.SourceMemoryReview {
			if err := publishMemoryChanges(ctx, tx, run, thread, now); err != nil {
				return err
			}
		}
		publication := terminalPublication{run: run, out: &out, now: now}
		if err := publication.write(ctx, tx, thread); err != nil {
			return err
		}

		if thread.ActiveRunID == run.ID {
			thread.ActiveRunID = 0
		}
		thread.UpdatedAtMs = now
		if err := tx.SaveThread(ctx, *thread); err != nil {
			return err
		}
		if run.Source == store.SourceUser && out.status == store.StatusDone {
			if err := countSuccessfulTurn(ctx, tx, run, now); err != nil {
				return err
			}
		}
		_, err = AppendEvent(ctx, tx, nil, run, out.eventType, out.payload)
		return err
	})
	if err != nil {
		return err
	}
	if e.Notify != nil {
		_ = e.Notify.Wake(ctx, run.ID)
	}
	return nil
}

// countSuccessfulTurn 累计会话的成功用户轮次；每满 10 轮排队一次后台记忆整理 run。
func countSuccessfulTurn(ctx context.Context, tx store.Store, run store.Run, now int64) error {
	session, err := tx.GetSession(ctx, run.SessionID)
	if err != nil {
		return err
	}
	session.SuccessfulUserTurns++
	if err := tx.UpdateSession(ctx, *session); err != nil {
		return err
	}
	if session.SuccessfulUserTurns%10 != 0 {
		return nil
	}
	_, err = tx.InsertRun(ctx, store.Run{
		UserID: run.UserID, SessionID: run.SessionID, RequestID: "review-" + strconv.FormatInt(now, 10),
		Source: store.SourceMemoryReview, Status: store.StatusQueued, Phase: store.PhaseQueued,
		Priority: store.PriorityMemoryReview, ConsentVersion: run.ConsentVersion, InputVersion: 1,
		PromptEpoch: session.PromptEpoch, CreatedAtMs: now, LastActivityAtMs: now,
	})
	return err
}

// terminalPublication 把终态消息写入会话：消息、回答展示、搜索索引 outbox 与线程摘要。
type terminalPublication struct {
	run store.Run
	out *terminalOutcome
	now int64
}

// write 只在有消息且不是后台记忆整理 run 时发布；会补全流 ID 与回答的消息 ID，供随后的终态事件引用。
func (p *terminalPublication) write(ctx context.Context, tx store.Store, thread *store.Thread) error {
	if p.out.message != "" && p.run.Source != store.SourceMemoryReview {
		if p.out.emitToken {
			if _, err := AppendEvent(ctx, tx, nil, p.run, store.EventToken, store.EventPayload{Text: p.out.message, StreamID: p.out.streamID}); err != nil {
				return err
			}
		}
		if len(p.out.apiContent) == 0 {
			p.out.apiContent = prompt.EncodeTurn(prompt.Turn{Role: store.RoleAssistant, Content: p.out.message})
		}
		if p.out.streamID != "" && p.out.payload.StreamID == "" {
			p.out.payload.StreamID = p.out.streamID
		}
		msg, err := tx.InsertMessage(ctx, store.Message{
			UserID: p.run.UserID, SessionID: p.run.SessionID, RunID: p.run.ID, Role: store.RoleAssistant,
			Kind: store.KindMessage, Content: p.out.message, APIContent: p.out.apiContent, Visible: true, CreatedAtMs: p.now,
		})
		if err != nil {
			return err
		}
		if p.out.payload.Answer != nil {
			p.out.payload.Answer.MessageID = msg.ID
			if err := tx.SavePresentation(ctx, *p.out.payload.Answer); err != nil {
				return err
			}
			if _, err := AppendEvent(ctx, tx, nil, p.run, store.EventAnswerCommitted, store.EventPayload{Answer: p.out.payload.Answer, Text: p.out.message}); err != nil {
				return err
			}
		}
		if err := tx.InsertOutbox(ctx, store.Outbox{
			UserID: p.run.UserID, MessageID: msg.ID, Op: store.IndexOpUpsert,
			PayloadJSON: string(mustJSON(map[string]any{
				"userId": p.run.UserID, "sessionId": p.run.SessionID, "messageId": msg.ID,
				"role": store.RoleAssistant, "content": p.out.message, "createdAtMs": p.now,
			})), CreatedAtMs: p.now,
		}); err != nil {
			return err
		}
		thread.LastMessageID = msg.ID
		thread.LastMessagePreview = store.Preview(p.out.message, 80)
		thread.LastMessageAtMs = p.now
	}
	return nil
}
