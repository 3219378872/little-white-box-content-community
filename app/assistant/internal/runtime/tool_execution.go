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

func (e *Engine) execTool(workCtx, persistCtx context.Context, run *store.Run, registry *tool.Registry, call llm.ToolCall, reviewLive *[]prompt.Turn) error {
	if HardLimitExceeded(*run, store.NowMs()) {
		return e.stopAtResourceLimit(persistCtx, *run)
	}
	sess := e.toolSession(*run)
	if err := e.populateToolLiveMessageIDs(persistCtx, *run, sess); err != nil {
		return err
	}
	call, digest, prepErr := prepareCall(workCtx, registry, sess, call)

	journal, reserved, err := e.startToolStep(persistCtx, *run, call, digest, prepErr == nil && registry.SideEffect(call.Name))
	if err != nil {
		return err
	}
	if journal != nil && journal.Status == store.JournalSuccess {
		agentToolCalls.Inc(call.Name, "replay")
		text := decodeToolResultText(journal.ResultJSON)
		sess.ChangeIDs = decodeToolResultChangeIDs(journal.ResultJSON)
		if err := e.finishToolStep(persistCtx, run, call, text, nil, nil, sess.ChangeIDs, journal, false, "replay", reviewLive); err != nil {
			return err
		}
		return e.guardToolProgress(persistCtx, run, registry, call, reviewLive)
	}
	if journal != nil && !reserved && journal.Status == store.JournalPending {
		return errors.New("side effect command is already in progress")
	}
	if prepErr != nil {
		text := prepErr.Error()
		if err := e.finishToolStep(persistCtx, run, call, text, prepErr, nil, nil, journal, true, "invalid", reviewLive); err != nil {
			return err
		}
		return e.guardToolProgress(persistCtx, run, registry, call, reviewLive)
	}
	sess.Recovery = journal != nil && journal.Takeover
	if registry.HighRisk(call.Name) {
		if err := e.requireConfirm(workCtx, persistCtx, run, call, digest); err != nil {
			return err
		}
		if journal == nil || !journal.Takeover {
			rechecked, err := registry.Prepare(workCtx, sess, call.Name, call.Arguments)
			if err != nil {
				return err
			}
			recheckedDigest, err := canonical.DigestArgs(rechecked)
			if err != nil || recheckedDigest != digest {
				return errx.New(errx.ContentVersionConflict, "delete_post changed after confirmation")
			}
			call.Arguments = rechecked
		}
	}
	if e.cancelled(persistCtx, run) {
		return errRunCancelled
	}
	if err := e.requireFrozenConsent(persistCtx, run); err != nil {
		return err
	}
	if HardLimitExceeded(*run, store.NowMs()) {
		return e.stopAtResourceLimit(persistCtx, *run)
	}
	var (
		text    string
		cards   []store.SourceRef
		callErr error
	)
	invoke := func() {
		text, cards, callErr = registry.Call(workCtx, sess, call.Name, call.ID, call.Arguments)
	}
	if registry.SideEffect(call.Name) {
		if err := e.step(persistCtx, *run, func(context.Context, store.Store) error {
			invoke()
			return nil
		}); err != nil {
			return err
		}
	} else {
		if err := e.step(persistCtx, *run, func(context.Context, store.Store) error { return nil }); err != nil {
			return err
		}
		invoke()
	}
	outcome := "success"
	if callErr != nil {
		if errors.Is(callErr, context.Canceled) && e.cancelled(persistCtx, run) {
			return errRunCancelled
		}
		outcome = "unavailable"
		text = callErr.Error()
	}
	agentToolCalls.Inc(call.Name, outcome)
	if callErr == nil && sess.Question != nil {
		return e.waitForQuestions(persistCtx, run, call, *sess.Question)
	}
	if callErr == nil && sess.Answer != nil {
		return e.publishAnswer(persistCtx, run, call, *sess.Answer)
	}
	if err := e.finishToolStep(persistCtx, run, call, text, callErr, cards, sess.ChangeIDs, journal, true, outcome, reviewLive); err != nil {
		return err
	}
	if err := e.guardToolProgress(persistCtx, run, registry, call, reviewLive); err != nil {
		return err
	}
	if e.cancelled(persistCtx, run) {
		return errRunCancelled
	}
	return nil
}

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

func (e *Engine) toolSession(run store.Run) *tool.Session {
	sess := &tool.Session{
		ClientProtocolVersion: clientProtocol(run),
		UserID:                run.UserID, SessionID: run.SessionID, RunID: run.ID, RequestID: run.RequestID,
		Source: run.Source, ConsentVersion: run.ConsentVersion, Fence: run.Fence(),
	}
	if run.Source == store.SourceWatch {
		watchPayload := decodeWatchRunPayload(run.QueuedPayload)
		sess.WatchPostIDs = append([]int64(nil), watchPayload.PostIDs...)
	} else {
		payload := decodeInputPayload(run.QueuedPayload)
		sess.ContextPostID = payload.ContextPostID
		sess.Attachments = make([]tool.Attachment, 0, len(payload.Attachments))
		for _, item := range payload.Attachments {
			sess.Attachments = append(sess.Attachments, tool.Attachment{MediaID: item.MediaID, URL: item.URL})
		}
	}
	return sess
}

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
		newCall  bool
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
			newCall = true
			if _, err := AppendEvent(ctx, tx, nil, run, store.EventToolCall, store.EventPayload{
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
	if err == nil && newCall && e.Notify != nil {
		_ = e.Notify.Wake(ctx, run.ID)
	}
	return journal, reserved, err
}

func (e *Engine) finishToolStep(
	ctx context.Context,
	run *store.Run,
	call llm.ToolCall,
	text string,
	callErr error,
	cards []store.SourceRef,
	changeIDs []int64,
	journal *store.Journal,
	countCall bool,
	outcome string,
	reviewLive *[]prompt.Turn,
) error {
	if countCall {
		run.ToolCalls++
	}
	run.LastActivityAtMs = store.NowMs()
	resultJSON := encodeToolResultJSONWithChanges(text, callErr, changeIDs)
	turn := prompt.Turn{Role: store.RoleTool, Content: text, ToolCallID: call.ID, Name: call.Name}
	err := e.step(ctx, *run, func(ctx context.Context, tx store.Store) error {
		if err := tx.UpdateRun(ctx, *run); err != nil {
			return err
		}
		if journal != nil {
			status := store.JournalSuccess
			if callErr != nil {
				status = store.JournalError
			}
			if err := tx.CompleteJournal(ctx, journal.ID, status, resultJSON); err != nil {
				return err
			}
		}
		if err := tx.UpdateToolCall(ctx, store.ToolCall{
			RunID: run.ID, CallID: call.ID, Status: outcome, ResultJSON: resultJSON,
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
		if _, err := AppendEvent(ctx, tx, nil, *run, store.EventToolResult, store.EventPayload{
			ToolCall: &store.ToolInfo{CallID: call.ID, Tool: call.Name, Summary: outcome, PayloadJSON: text}, Text: text,
		}); err != nil {
			return err
		}
		for i := range cards {
			if _, err := AppendEvent(ctx, tx, nil, *run, store.EventSourceCard, store.EventPayload{SourceCard: &cards[i]}); err != nil {
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
	if e.Notify != nil {
		_ = e.Notify.Wake(ctx, run.ID)
	}
	return nil
}

func decodeToolResultChangeIDs(raw string) []int64 {
	var payload struct {
		ChangeIDs []int64 `json:"change_ids"`
	}
	if json.Unmarshal([]byte(raw), &payload) != nil {
		return nil
	}
	return payload.ChangeIDs
}

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
