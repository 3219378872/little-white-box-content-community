package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"esx/app/assistant/internal/canonical"
	"esx/app/assistant/internal/llm"
	"esx/app/assistant/internal/prompt"
	"esx/app/assistant/internal/store"
	"esx/app/assistant/internal/tool"
	"esx/pkg/errx"
	"strings"
)

// execTool 执行一次工具调用：准备参数 → 登记调用并为有副作用的工具预留 journal →
// 已成功的 journal 直接重放 → 高风险工具先取得用户确认 → 调用前复核取消、授权与预算 →
// 调用并落库结果 → 检查是否陷入无进展的重复调用。
func (e *Engine) execTool(workCtx, persistCtx context.Context, run *store.Run, registry *tool.Registry, call llm.ToolCall, reviewLive *[]prompt.Turn) error {
	if HardLimitExceeded(*run, store.NowMs()) {
		return e.stopAtResourceLimit(persistCtx, *run)
	}
	sess := e.toolSession(*run)
	if err := e.populateToolLiveMessageIDs(persistCtx, *run, sess); err != nil {
		return err
	}
	call, digest, prepErr := prepareCall(workCtx, registry, sess, call)

	// 只有参数合法的副作用工具才预留 journal，用于崩溃恢复后的去重与重放。
	journal, reserved, err := e.startToolStep(persistCtx, *run, call, digest, prepErr == nil && registry.SideEffect(call.Name))
	if err != nil {
		return err
	}
	if journal != nil && journal.Status == store.JournalSuccess {
		agentToolCalls.Inc(call.Name, "replay")
		sess.ChangeIDs = decodeToolResultChangeIDs(journal.ResultJSON)
		return e.completeToolStep(persistCtx, run, registry, call, toolStepResult{
			text: decodeToolResultText(journal.ResultJSON), changeIDs: sess.ChangeIDs,
			journal: journal, outcome: "replay",
		}, reviewLive)
	}
	// journal 已被另一个执行者持有且未完成时不能并发执行同一副作用。
	if journal != nil && !reserved && journal.Status == store.JournalPending {
		return errors.New("side effect command is already in progress")
	}
	if prepErr != nil {
		return e.completeToolStep(persistCtx, run, registry, call, toolStepResult{
			text: prepErr.Error(), err: prepErr, journal: journal, countCall: true, outcome: "invalid",
		}, reviewLive)
	}
	sess.Recovery = journal != nil && journal.Takeover
	if registry.HighRisk(call.Name) {
		if call, err = e.confirmHighRiskCall(workCtx, persistCtx, run, registry, sess, call, digest); err != nil {
			return err
		}
	}
	if err := e.checkBeforeInvoke(persistCtx, run); err != nil {
		return err
	}
	text, cards, callErr, err := e.invokeTool(workCtx, persistCtx, *run, registry, sess, call)
	if err != nil {
		return err
	}
	outcome := "success"
	if callErr != nil {
		if errors.Is(callErr, context.Canceled) && e.cancelled(persistCtx, run) {
			return errRunCancelled
		}
		// 工具失败不终止 run：错误文本作为工具结果交给模型自行调整。
		outcome = "unavailable"
		text = callErr.Error()
	}
	agentToolCalls.Inc(call.Name, outcome)
	// 追问与发布回答会让 run 进入等待或直接结束，不再写普通工具结果。
	if callErr == nil && sess.Question != nil {
		return e.waitForQuestions(persistCtx, run, call, *sess.Question)
	}
	if callErr == nil && sess.Answer != nil {
		return e.publishAnswer(persistCtx, run, call, *sess.Answer)
	}
	if err := e.completeToolStep(persistCtx, run, registry, call, toolStepResult{
		text: text, err: callErr, cards: cards, changeIDs: sess.ChangeIDs,
		journal: journal, countCall: true, outcome: outcome,
	}, reviewLive); err != nil {
		return err
	}
	if e.cancelled(persistCtx, run) {
		return errRunCancelled
	}
	return nil
}

// confirmHighRiskCall 等待用户确认高风险调用，并在确认后重新准备参数：
// 若目标在等待期间发生变化（摘要不同），拒绝执行而不是按旧确认操作新内容。
func (e *Engine) confirmHighRiskCall(
	workCtx, persistCtx context.Context,
	run *store.Run,
	registry *tool.Registry,
	sess *tool.Session,
	call llm.ToolCall,
	digest string,
) (llm.ToolCall, error) {
	if err := e.requireConfirm(workCtx, persistCtx, run, call, digest); err != nil {
		return call, err
	}
	rechecked, err := registry.Prepare(workCtx, sess, call.Name, call.Arguments)
	if err != nil {
		return call, err
	}
	recheckedDigest, err := canonical.DigestArgs(rechecked)
	if err != nil || recheckedDigest != digest {
		return call, errx.New(errx.ContentVersionConflict, "delete_post changed after confirmation")
	}
	call.Arguments = rechecked
	return call, nil
}

// checkBeforeInvoke 在真正调用工具前复核：run 未被取消、授权快照未变、预算未耗尽。
// 确认等待可能很久，这些条件都可能在此期间变化。
func (e *Engine) checkBeforeInvoke(ctx context.Context, run *store.Run) error {
	if e.cancelled(ctx, run) {
		return errRunCancelled
	}
	if err := e.requireFrozenConsent(ctx, run); err != nil {
		return err
	}
	if HardLimitExceeded(*run, store.NowMs()) {
		return e.stopAtResourceLimit(ctx, *run)
	}
	return nil
}

// invokeTool 调用工具。副作用工具在租约栅栏校验的同一步骤内调用，失去租约的执行者不会产生副作用；
// 只读工具先校验栅栏再在事务外调用。返回的 err 是栅栏错误，callErr 是工具自身的错误。
func (e *Engine) invokeTool(
	workCtx, persistCtx context.Context,
	run store.Run,
	registry *tool.Registry,
	sess *tool.Session,
	call llm.ToolCall,
) (text string, cards []store.SourceRef, callErr error, err error) {
	invoke := func() {
		text, cards, callErr = registry.Call(workCtx, sess, call.Name, call.ID, call.Arguments)
	}
	if registry.SideEffect(call.Name) {
		err = e.step(persistCtx, run, func(context.Context, store.Store) error {
			invoke()
			return nil
		})
		return text, cards, callErr, err
	}
	if err = e.step(persistCtx, run, func(context.Context, store.Store) error { return nil }); err != nil {
		return "", nil, nil, err
	}
	invoke()
	return text, cards, callErr, nil
}

// completeToolStep 落库工具结果后检查重复调用；两步总是成对出现。
func (e *Engine) completeToolStep(ctx context.Context, run *store.Run, registry *tool.Registry, call llm.ToolCall, result toolStepResult, reviewLive *[]prompt.Turn) error {
	if err := e.finishToolStep(ctx, run, call, result, reviewLive); err != nil {
		return err
	}
	return e.guardToolProgress(ctx, run, registry, call, reviewLive)
}

// populateToolLiveMessageIDs 告诉工具哪些消息仍在上下文中，供历史检索排除。
func (e *Engine) populateToolLiveMessageIDs(ctx context.Context, run store.Run, sess *tool.Session) error {
	if e == nil || e.Store == nil || sess == nil {
		return nil
	}
	messages, err := e.Store.ListSessionMessages(ctx, run.UserID, run.SessionID, true)
	if err != nil {
		return err
	}
	for _, message := range messages {
		if message.DeletedAtMs == 0 && !message.Compacted {
			sess.LiveMessageIDs = append(sess.LiveMessageIDs, message.ID)
		}
	}
	return nil
}

// guardToolProgress 检测同一工具以相同参数反复得到相同结果：第二次重复时注入提示让模型换方法，
// 再重复则以 TOOL_NO_PROGRESS 结束 run。轮询类工具不受限。
func (e *Engine) guardToolProgress(ctx context.Context, run *store.Run, registry *tool.Registry, current llm.ToolCall, reviewLive *[]prompt.Turn) error {
	if run == nil || registry == nil || registry.Poller(current.Name) {
		return nil
	}
	calls, err := e.Store.ListToolCalls(ctx, run.ID)
	if err != nil {
		return err
	}
	var currentRow *store.ToolCall
	for i := range calls {
		if calls[i].CallID == current.ID {
			currentRow = &calls[i]
			break
		}
	}
	if currentRow == nil || currentRow.Status == "running" || currentRow.ResultJSON == "" {
		return nil
	}
	resultDigest, err := canonical.DigestArgs(normalizeEvidenceResult(currentRow.ResultJSON))
	if err != nil {
		resultDigest = strings.Join(strings.Fields(currentRow.ResultJSON), " ")
	}
	count := 0
	for _, call := range calls {
		if call.Tool != currentRow.Tool || call.CanonicalArgsDigest != currentRow.CanonicalArgsDigest || call.Status == "running" {
			continue
		}
		digest, digestErr := canonical.DigestArgs(normalizeEvidenceResult(call.ResultJSON))
		if digestErr != nil {
			digest = strings.Join(strings.Fields(call.ResultJSON), " ")
		}
		if digest == resultDigest {
			count++
		}
	}
	if count < 2 {
		return nil
	}
	if count == 2 {
		turn := prompt.Turn{Role: store.RoleSystem, Content: "工具无进展：相同工具、参数和结果已重复。请改变方法、根据现有结果作答，或明确说明限制；不要原样再次调用。"}
		if run.Source == store.SourceMemoryReview {
			if reviewLive != nil {
				*reviewLive = append(*reviewLive, turn)
			}
			return nil
		}
		return e.step(ctx, *run, func(ctx context.Context, tx store.Store) error {
			_, err := tx.InsertMessage(ctx, store.Message{
				UserID: run.UserID, SessionID: run.SessionID, RunID: run.ID, Role: store.RoleSystem,
				Kind: store.KindTool, Content: "", APIContent: prompt.EncodeTurn(turn), Visible: false, CreatedAtMs: store.NowMs(),
			})
			return err
		})
	}
	if err := e.fail(ctx, *run, "TOOL_NO_PROGRESS", "工具调用连续重复且没有进展"); err != nil {
		return err
	}
	return errRunTerminated
}

// toolSession 构造工具执行所需的会话上下文，包括租约栅栏与用户附件。
func (e *Engine) toolSession(run store.Run) *tool.Session {
	sess := &tool.Session{
		ClientProtocolVersion: clientProtocol(run),
		UserID:                run.UserID, SessionID: run.SessionID, RunID: run.ID, RequestID: run.RequestID,
		Source: run.Source, ConsentVersion: run.ConsentVersion, Fence: run.Fence(),
	}
	payload := decodeInputPayload(run.QueuedPayload)
	sess.ContextPostID = payload.ContextPostID
	sess.Attachments = make([]tool.Attachment, 0, len(payload.Attachments))
	for _, item := range payload.Attachments {
		sess.Attachments = append(sess.Attachments, tool.Attachment{MediaID: item.MediaID, URL: item.URL})
	}
	return sess
}

// startToolStep 登记工具调用；有副作用的调用同时预留命令日志，返回的 reserved 表示本次是否应真正执行。
func (e *Engine) startToolStep(
	ctx context.Context,
	run store.Run,
	call llm.ToolCall,
	digest string,
	reserveSideEffect bool,
) (*store.Journal, bool, error) {
	var (
		journal  *store.Journal
		reserved bool
	)
	now := store.NowMs()
	err := e.step(ctx, run, func(ctx context.Context, tx store.Store) error {
		existing, err := tx.GetToolCall(ctx, run.ID, call.ID)
		if err != nil {
			return err
		}
		if existing == nil {
			if _, err := tx.InsertToolCall(ctx, store.ToolCall{
				RunID: run.ID, CallID: call.ID, Tool: call.Name, ArgsJSON: call.Arguments,
				CanonicalArgsDigest: digest, Status: "running", CreatedAtMs: now,
			}); err != nil {
				return err
			}
			if _, err := AppendEvent(ctx, tx, run, store.EventToolCall, store.EventPayload{
				ToolCall: &store.ToolInfo{CallID: call.ID, Tool: call.Name, Summary: call.Name, PayloadJSON: call.Arguments},
			}); err != nil {
				return err
			}
		} else if existing.Tool != call.Name || existing.CanonicalArgsDigest != digest {
			return errx.New(errx.PermissionDenied, "tool call identity changed during recovery")
		}
		if !reserveSideEffect {
			return nil
		}
		journal, reserved, err = tx.ReserveJournal(ctx, store.Journal{
			UserID: run.UserID, RequestID: run.RequestID, Tool: call.Name, CanonicalArgsDigest: digest,
			RunID: run.ID, LeaseGeneration: run.LeaseGeneration, Status: store.JournalPending,
			CreatedAtMs: now, UpdatedAtMs: now,
		})
		return err
	})
	return journal, reserved, err
}

// toolStepResult 是一次工具调用要落库的结果。
type toolStepResult struct {
	text      string
	err       error
	cards     []store.SourceRef
	changeIDs []int64
	// journal 非空时同时完成副作用 journal，记录成功或失败。
	journal *store.Journal
	// countCall 决定是否计入工具调用预算；重放已计过数，不再重复计。
	countCall bool
	// outcome 是工具调用行与事件中的状态：success、unavailable、invalid、replay。
	outcome string
}

// finishToolStep 在一个步骤内写入 run 计数、journal、工具调用结果、隐藏的工具消息与结果事件。
// 后台记忆整理 run 不落工具消息，而是把结果追加到内存中的 reviewLive 对话。
func (e *Engine) finishToolStep(ctx context.Context, run *store.Run, call llm.ToolCall, result toolStepResult, reviewLive *[]prompt.Turn) error {
	if result.countCall {
		run.ToolCalls++
	}
	run.LastActivityAtMs = store.NowMs()
	resultJSON := encodeToolResultJSONWithChanges(result.text, result.err, result.changeIDs)
	turn := prompt.Turn{Role: store.RoleTool, Content: result.text, ToolCallID: call.ID, Name: call.Name}
	err := e.step(ctx, *run, func(ctx context.Context, tx store.Store) error {
		if err := tx.UpdateRun(ctx, *run); err != nil {
			return err
		}
		if result.journal != nil {
			status := store.JournalSuccess
			if result.err != nil {
				status = store.JournalError
			}
			if err := tx.CompleteJournal(ctx, result.journal.ID, status, resultJSON); err != nil {
				return err
			}
		}
		if err := tx.UpdateToolCall(ctx, store.ToolCall{
			RunID: run.ID, CallID: call.ID, Status: result.outcome, ResultJSON: resultJSON,
		}); err != nil {
			return err
		}
		if run.Source != store.SourceMemoryReview {
			if _, err := tx.InsertMessage(ctx, store.Message{
				UserID: run.UserID, SessionID: run.SessionID, RunID: run.ID,
				Role: turn.Role, Kind: store.KindTool, Content: turn.Content, APIContent: prompt.EncodeTurn(turn),
				Visible: false, CreatedAtMs: store.NowMs(),
			}); err != nil {
				return err
			}
		}
		if _, err := AppendEvent(ctx, tx, *run, store.EventToolResult, store.EventPayload{
			ToolCall: &store.ToolInfo{CallID: call.ID, Tool: call.Name, Summary: result.outcome, PayloadJSON: result.text}, Text: result.text,
		}); err != nil {
			return err
		}
		for i := range result.cards {
			if _, err := AppendEvent(ctx, tx, *run, store.EventSourceCard, store.EventPayload{SourceCard: &result.cards[i]}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if run.Source == store.SourceMemoryReview && reviewLive != nil {
		*reviewLive = append(*reviewLive, turn)
	}
	return nil
}

// decodeToolResultChangeIDs 从工具结果中取出记忆变更 ID。
func decodeToolResultChangeIDs(raw string) []int64 {
	var payload struct {
		ChangeIDs []int64 `json:"change_ids"`
	}
	if json.Unmarshal([]byte(raw), &payload) != nil {
		return nil
	}
	return payload.ChangeIDs
}

// unmatchedToolCalls 找出历史中已发起但还没有结果的工具调用，按首次出现顺序去重。
func unmatchedToolCalls(history []prompt.Turn) []llm.ToolCall {
	done := make(map[string]struct{})
	for _, turn := range history {
		if id := strings.TrimSpace(turn.ToolCallID); id != "" {
			done[id] = struct{}{}
		}
	}
	out := make([]llm.ToolCall, 0)
	seen := make(map[string]struct{})
	for _, turn := range history {
		for _, call := range turn.ToolCalls {
			id := strings.TrimSpace(call.ID)
			if id == "" {
				continue
			}
			if _, ok := done[id]; ok {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			out = append(out, llm.ToolCall{ID: id, Name: call.Name, Arguments: call.Arguments, Prepared: call.Prepared})
		}
	}
	return out
}

// encodeToolResultJSONWithChanges 编码工具结果；出错且无正文时用错误信息作为给模型的正文。
func encodeToolResultJSONWithChanges(text string, callErr error, changeIDs []int64) string {
	payload := map[string]any{"ok": callErr == nil, "text": text}
	if len(changeIDs) > 0 {
		payload["change_ids"] = changeIDs
	}
	if callErr != nil {
		payload["error"] = callErr.Error()
		if text == "" {
			payload["text"] = callErr.Error()
		}
	}
	raw, _ := json.Marshal(payload)
	return string(raw)
}

// decodeToolResultText 从工具结果中取出给模型的正文，兼容对象、JSON 字符串与纯文本三种旧格式。
func decodeToolResultText(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var payload struct {
		Text string `json:"text"`
	}
	if json.Unmarshal([]byte(raw), &payload) == nil && payload.Text != "" {
		return payload.Text
	}
	var asString string
	if json.Unmarshal([]byte(raw), &asString) == nil && asString != "" {
		return asString
	}
	return raw
}

// prepareCall 预处理工具参数并计算规范化摘要；参数无法解析时用调用 ID 生成唯一摘要，避免误命中幂等记录。
func prepareCall(ctx context.Context, registry *tool.Registry, sess *tool.Session, call llm.ToolCall) (llm.ToolCall, string, error) {
	prepErr := call.PrepareError
	if !call.Prepared && prepErr == nil {
		prepared, err := registry.Prepare(ctx, sess, call.Name, call.Arguments)
		prepErr = err
		if prepErr == nil {
			call.Arguments = prepared
			call.Prepared = true
		} else {
			call.Arguments = canonical.UnwrapArgsJSON(call.Arguments)
		}
	} else {
		call.Arguments = canonical.UnwrapArgsJSON(call.Arguments)
	}
	digest, digestErr := canonical.DigestArgs(call.Arguments)
	if digestErr != nil {
		digest = "invalid:" + call.ID
	}

	return call, digest, prepErr
}
