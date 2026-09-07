package runtime

import (
	"context"
	"esx/app/assistant/internal/llm"
	"esx/app/assistant/internal/prompt"
	"esx/app/assistant/internal/store"
	"esx/app/assistant/internal/tool"
	"strings"
	"time"
)

func (e *Engine) incomplete(ctx context.Context, run store.Run, result llm.Result) error {
	partial := strings.TrimSpace(result.Text)
	reason := "UNKNOWN"
	switch strings.ToLower(strings.TrimSpace(result.IncompleteReason)) {
	case "max_output_tokens":
		reason = "MAX_OUTPUT_TOKENS"
	case "content_filter":
		reason = "CONTENT_FILTER"
	}
	payload := store.EventPayload{
		ErrorCode: "LLM_INCOMPLETE_" + reason,
		Text:      "模型响应未完整完成",
		Partial:   partial,
	}
	if partial != "" && run.Source == store.SourceUser {
		return e.finishWithMessageEvent(ctx, run, store.StatusError, store.EventError, payload, partial,
			prompt.EncodeTurn(prompt.Turn{Role: store.RoleAssistant, Content: partial}), !result.Streamed, result.StreamID)
	}
	return e.finish(ctx, run, store.StatusError, store.EventError, payload)
}

func keepsStreamedAnswer(names []string) bool {
	if len(names) == 0 {
		return false
	}
	for _, name := range names {
		if name != tool.PresentSources {
			return false
		}
	}
	return true
}

func toolCallNames(calls []llm.ToolCall) []string {
	names := make([]string, 0, len(calls))
	for _, call := range calls {
		names = append(names, call.Name)
	}
	return names
}

func turnToolNames(turn prompt.Turn) []string {
	names := make([]string, 0, len(turn.ToolCalls))
	for _, call := range turn.ToolCalls {
		names = append(names, call.Name)
	}
	return names
}

func visibleForPrompt(msgs []store.Message) []store.Message {
	out := make([]store.Message, 0, len(msgs))
	for _, msg := range msgs {
		if msg.Compacted {
			continue
		}
		out = append(out, msg)
	}
	return out
}

func (e *Engine) completeModel(workCtx, persistCtx context.Context, run store.Run, client llm.Client, req llm.Request) (llm.Result, error) {
	if err := e.step(persistCtx, run, func(context.Context, store.Store) error { return nil }); err != nil {
		return llm.Result{}, err
	}
	before, err := e.Store.GetRun(persistCtx, run.ID)
	if err != nil {
		return llm.Result{}, err
	}
	if before.CancelRequested {
		return llm.Result{}, errRunCancelled
	}
	if before.LeaseOwner != run.LeaseOwner || before.LeaseGeneration != run.LeaseGeneration {
		return llm.Result{}, store.ErrLeaseLost
	}
	if before.InputVersion != run.InputVersion {
		return llm.Result{}, errRunRedirected
	}
	callCtx, cancelCall := context.WithCancel(workCtx)
	stop := watchInputChange(persistCtx, e.Store, run.ID, run.InputVersion, cancelCall)
	writer := newModelStreamWriter(e, persistCtx, run)
	req.AttemptPrefix = writer.prefix
	previousObserver := req.Observer
	req.Observer = func(event llm.AttemptEvent) error {
		if err := writer.Observe(event); err != nil {
			return err
		}
		if previousObserver != nil {
			return previousObserver(event)
		}
		return nil
	}
	var result llm.Result
	if streaming, ok := client.(llm.StreamingClient); ok {
		delta := writer.Delta
		if req.SuppressText {
			delta = func(llm.Delta) error { return nil }
		}
		result, err = streaming.CompleteStream(callCtx, req, delta)
		if flushErr := writer.Finish(); err == nil && flushErr != nil {
			err = flushErr
		}
		if writer.Emitted() {
			result.Text = writer.Text()
			result.Streamed = true
			result.StreamID = writer.StreamID()
		}
		if err == nil && len(result.ToolCalls) > 0 && writer.Emitted() &&
			!keepsStreamedAnswer(toolCallNames(result.ToolCalls)) {
			if resetErr := writer.ResetWithRun(run); resetErr != nil {
				err = resetErr
			}
		}
	} else {
		result, err = client.Complete(callCtx, req)
		result.Text = prompt.SanitizeOutput(result.Text)
	}
	stop()
	if req.SuppressText {
		result.Text = ""
		result.Streamed = false
		result.StreamID = ""
	}
	fresh, getErr := e.Store.GetRun(persistCtx, run.ID)
	if getErr != nil {
		return llm.Result{}, getErr
	}
	if fresh.CancelRequested {
		return llm.Result{}, errRunCancelled
	}
	if fresh.LeaseOwner != run.LeaseOwner || fresh.LeaseGeneration != run.LeaseGeneration {
		return llm.Result{}, store.ErrLeaseLost
	}
	if fresh.InputVersion != run.InputVersion {
		_ = writer.ResetWithRun(*fresh)
		return llm.Result{}, errRunRedirected
	}
	return result, err
}

func watchInputChange(ctx context.Context, st store.Store, runID, inputVersion int64, cancel context.CancelFunc) func() {
	watchCtx, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(cancelWatchInterval)
		defer ticker.Stop()
		for {
			select {
			case <-watchCtx.Done():
				return
			case <-ticker.C:
				run, err := st.GetRun(watchCtx, runID)
				if err == nil && run != nil && (run.CancelRequested || run.InputVersion != inputVersion) {
					cancel()
					return
				}
			}
		}
	}()
	return func() {
		stop()
		<-done
		cancel()
	}
}

func (e *Engine) recordModelToolStep(ctx context.Context, run store.Run, turn prompt.Turn, reviewLive *[]prompt.Turn) error {
	if run.Source == store.SourceMemoryReview {
		if reviewLive != nil {
			*reviewLive = append(*reviewLive, turn)
		}
		return e.updateRun(ctx, run)
	}
	return e.step(ctx, run, func(ctx context.Context, tx store.Store) error {
		if err := tx.UpdateRun(ctx, run); err != nil {
			return err
		}
		_, err := tx.InsertMessage(ctx, store.Message{
			UserID: run.UserID, SessionID: run.SessionID, RunID: run.ID,
			Role: turn.Role, Kind: store.KindTool, Content: turn.Content, APIContent: prompt.EncodeTurn(turn),
			Visible:     strings.TrimSpace(turn.Content) != "" && keepsStreamedAnswer(turnToolNames(turn)),
			CreatedAtMs: store.NowMs(),
		})
		return err
	})
}

func (e *Engine) pendingUserTurns(ctx context.Context, run store.Run, seen map[int64]struct{}, history []prompt.Turn) ([]prompt.Turn, int64, error) {
	out := make([]prompt.Turn, 0)
	if run.Source == store.SourceWatch {
		if !historyHasWatchInput(history) {
			turn, err := e.watchInputTurn(ctx, run)
			if err != nil {
				return nil, 0, err
			}
			out = append(out, turn)
		}
	} else if len(run.QueuedPayload) > 0 {
		payload := decodeInputPayload(run.QueuedPayload)
		if payload.Text != "" {
			content := providerUserContent(payload.Text, payload.Attachments, payload.ContextPostID)
			if payload.MessageID == 0 {
				out = append(out, prompt.Turn{Role: store.RoleUser, Content: content})
			} else if _, ok := seen[payload.MessageID]; !ok {
				out = append(out, prompt.Turn{Role: store.RoleUser, Content: content})
			}
		}
	}
	items, err := e.Store.ListQueue(ctx, run.ID)
	if err != nil {
		return nil, 0, err
	}
	var queuedThrough int64
	for _, item := range items {
		if item.ID > queuedThrough {
			queuedThrough = item.ID
		}
		if _, ok := seen[item.MessageID]; ok {
			continue
		}
		msg, err := e.Store.GetMessage(ctx, run.UserID, item.MessageID)
		if err != nil {
			return nil, 0, err
		}
		if msg != nil {
			if turn, ok := turnFromMessage(*msg); ok {
				out = append(out, turn)
			} else {
				out = append(out, prompt.Turn{Role: store.RoleUser, Content: msg.Content})
			}
		}
	}
	return out, queuedThrough, nil
}
