package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"esx/app/assistant/internal/llm"
	"esx/app/assistant/internal/store"
)

const (
	streamFlushInterval = 200 * time.Millisecond
	streamFlushBytes    = 2 << 10
)

type modelStreamWriter struct {
	engine     *Engine
	ctx        context.Context
	run        store.Run
	modelRound int
	prefix     string

	mu        sync.Mutex
	streamID  string
	attempt   int
	pending   strings.Builder
	visible   strings.Builder
	lastFlush time.Time
	emitted   bool
}

func newModelStreamWriter(engine *Engine, ctx context.Context, run store.Run) *modelStreamWriter {
	round := run.Rounds + 1
	return &modelStreamWriter{
		engine: engine, ctx: ctx, run: run, modelRound: round,
		prefix: fmt.Sprintf("%d-%d-%d-%d", run.ID, run.LeaseGeneration, run.InputVersion, round),
	}
}

func (w *modelStreamWriter) Observe(event llm.AttemptEvent) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if event.StreamID == "" {
		event.StreamID = fmt.Sprintf("%s-%s-%d", w.prefix, event.RouteID, event.Attempt)
	}
	if event.Kind == llm.AttemptReset {
		if err := w.flushLocked(true); err != nil {
			return err
		}
		if w.emitted && event.StreamID == w.streamID {
			if err := w.appendPublicLocked(store.EventResponseReset, store.EventPayload{StreamID: event.StreamID}); err != nil {
				return err
			}
			w.visible.Reset()
			w.emitted = false
		}
	}
	if event.Kind == llm.AttemptStart {
		w.streamID = event.StreamID
		w.attempt = event.Attempt
		w.pending.Reset()
		w.lastFlush = time.Time{}
	}
	return w.appendInternalLocked(event)
}

func (w *modelStreamWriter) Delta(delta llm.Delta) error {
	if delta.Text == "" {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.streamID == "" {
		w.attempt = 1
		w.streamID = w.prefix + "-primary-1"
		if err := w.appendInternalLocked(llm.AttemptEvent{
			Kind: llm.AttemptStart, Attempt: 1, RouteID: "primary", StreamID: w.streamID,
		}); err != nil {
			return err
		}
	}
	w.pending.WriteString(delta.Text)
	first := !w.emitted
	if first || w.pending.Len() >= streamFlushBytes || time.Since(w.lastFlush) >= streamFlushInterval {
		return w.flushLocked(false)
	}
	return nil
}

func (w *modelStreamWriter) Finish() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.flushLocked(true)
}

func (w *modelStreamWriter) ResetWithRun(run store.Run) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.flushLocked(true); err != nil && err != errRunRedirected {
		return err
	}
	if !w.emitted || w.streamID == "" {
		w.pending.Reset()
		return nil
	}
	w.run = run
	w.pending.Reset()
	if err := w.appendPublicLocked(store.EventResponseReset, store.EventPayload{StreamID: w.streamID}); err != nil {
		return err
	}
	w.visible.Reset()
	w.emitted = false
	return nil
}

func (w *modelStreamWriter) StreamID() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.streamID
}

func (w *modelStreamWriter) Text() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.visible.String() + w.pending.String()
}

func (w *modelStreamWriter) Emitted() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.emitted || w.pending.Len() > 0
}

func (w *modelStreamWriter) flushLocked(force bool) error {
	if w.pending.Len() == 0 {
		return nil
	}
	if !force && w.emitted && w.pending.Len() < streamFlushBytes && time.Since(w.lastFlush) < streamFlushInterval {
		return nil
	}
	text := w.pending.String()
	if err := w.appendPublicLocked(store.EventToken, store.EventPayload{Text: text, StreamID: w.streamID}); err != nil {
		return err
	}
	w.visible.WriteString(text)
	w.pending.Reset()
	w.lastFlush = time.Now()
	w.emitted = true
	return nil
}

func (w *modelStreamWriter) appendPublicLocked(eventType string, payload store.EventPayload) error {
	err := w.engine.step(w.ctx, w.run, func(ctx context.Context, tx store.Store) error {
		if err := validateStreamIdentity(ctx, tx, w.run, w.modelRound); err != nil {
			return err
		}
		_, err := AppendEvent(ctx, tx, nil, w.run, eventType, payload)
		return err
	})
	if err == nil && w.engine.Notify != nil {
		_ = w.engine.Notify.Wake(w.ctx, w.run.ID)
	}
	return err
}

func (w *modelStreamWriter) appendInternalLocked(event llm.AttemptEvent) error {
	payload := store.EventPayload{
		StreamID: event.StreamID, RouteID: event.RouteID, Attempt: event.Attempt,
		ErrorClass: string(event.ErrorKind), StatusCode: event.StatusCode, Retryable: event.Retryable,
		Text: event.Kind,
	}
	raw, _ := json.Marshal(payload)
	return w.engine.step(w.ctx, w.run, func(ctx context.Context, tx store.Store) error {
		if err := validateStreamIdentity(ctx, tx, w.run, w.modelRound); err != nil {
			return err
		}
		_, err := tx.InsertEvent(ctx, w.run.ID, store.EventProviderAttempt, raw, store.NowMs())
		return err
	})
}

func validateStreamIdentity(ctx context.Context, tx store.Store, run store.Run, modelRound int) error {
	fresh, err := tx.GetRun(ctx, run.ID)
	if err != nil {
		return err
	}
	if fresh.InputVersion != run.InputVersion {
		return errRunRedirected
	}
	if fresh.Rounds != modelRound-1 {
		return store.ErrLeaseLost
	}
	return nil
}

func (e *Engine) resetRecoveredStreams(ctx context.Context, run store.Run) error {
	events, err := e.Store.ListEventsAfter(ctx, run.ID, 0)
	if err != nil {
		return err
	}
	open := make(map[string]struct{})
	for _, event := range events {
		var payload store.EventPayload
		_ = json.Unmarshal(event.PayloadJSON, &payload)
		if payload.StreamID == "" {
			continue
		}
		switch event.Type {
		case store.EventToken:
			open[payload.StreamID] = struct{}{}
		case store.EventResponseReset:
			delete(open, payload.StreamID)
		}
	}
	for streamID := range open {
		if _, err := e.appendEvent(ctx, run, store.EventResponseReset, store.EventPayload{StreamID: streamID}); err != nil {
			return err
		}
	}
	return nil
}
