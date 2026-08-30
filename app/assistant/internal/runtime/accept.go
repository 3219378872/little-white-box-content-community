package runtime

import (
	"context"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"esx/app/assistant/internal/memory"
	"esx/app/assistant/internal/prompt"
	"esx/app/assistant/internal/store"
	"esx/pkg/errx"
)

type Attachment struct {
	MediaID int64  `json:"media_id"`
	URL     string `json:"url"`
}

type inputPayload struct {
	Text          string       `json:"text"`
	MessageID     int64        `json:"message_id"`
	Attachments   []Attachment `json:"attachments,omitempty"`
	ContextPostID int64        `json:"context_post_id,omitempty"`
}

type AcceptInput struct {
	UserID         int64
	Message        string
	RequestID      string
	Attachments    []Attachment
	ContextPostID  int64
	ConsentOK      bool
	ConsentVersion int32
}

type AcceptResult struct {
	MessageID   int64
	SessionID   int64
	RunID       int64
	Disposition string
}

type Acceptor struct {
	Store    store.Store
	Memory   memory.Store
	Notify   store.Notifier
	MaxRunes int
}

func (a *Acceptor) Accept(ctx context.Context, in AcceptInput) (AcceptResult, error) {
	if in.UserID <= 0 {
		return AcceptResult{}, errx.NewWithCode(errx.LoginRequired)
	}
	if !in.ConsentOK || in.ConsentVersion <= 0 {
		return AcceptResult{}, errx.NewWithCode(errx.AgentNotAuthorized)
	}
	text := strings.TrimSpace(in.Message)
	maxRunes := a.MaxRunes
	if maxRunes <= 0 {
		maxRunes = 2000
	}
	if text == "" || utf8.RuneCountInString(text) > maxRunes {
		return AcceptResult{}, errx.NewWithCode(errx.ParamError)
	}
	if strings.TrimSpace(in.RequestID) == "" {
		in.RequestID = "msg-" + itoa(store.NowMs())
	}
	var out AcceptResult
	err := a.Store.Transact(ctx, func(ctx context.Context, tx store.Store) error {
		result, err := a.acceptTx(ctx, tx, in, text)
		out = result
		return err
	})
	if err == nil && a.Notify != nil && out.RunID > 0 {
		_ = a.Notify.Wake(ctx, out.RunID)
	}
	return out, err
}

func (a *Acceptor) acceptTx(ctx context.Context, tx store.Store, in AcceptInput, text string) (AcceptResult, error) {
	now := store.NowMs()
	thread, err := tx.LockThread(ctx, in.UserID)
	if err != nil {
		return AcceptResult{}, err
	}
	consentVersion, granted, err := tx.AgentConsent(ctx, in.UserID)
	if err != nil {
		return AcceptResult{}, err
	}
	if !granted || consentVersion != in.ConsentVersion {
		return AcceptResult{}, errx.NewWithCode(errx.AgentNotAuthorized)
	}
	session, err := a.ensureSession(ctx, tx, thread, now)
	if err != nil {
		return AcceptResult{}, err
	}
	if existing, err := tx.GetRunByRequestID(ctx, in.UserID, in.RequestID); err == nil && existing != nil {
		return AcceptResult{MessageID: 0, SessionID: existing.SessionID, RunID: existing.ID, Disposition: store.DispositionStarted}, nil
	}

	api := prompt.EncodeTurn(prompt.Turn{Role: store.RoleUser, Content: providerUserContent(text, in.Attachments, in.ContextPostID)})
	msg, err := tx.InsertMessage(ctx, store.Message{
		UserID: in.UserID, SessionID: session.ID, Role: store.RoleUser, Kind: store.KindMessage,
		Content: text, APIContent: api, Visible: true, Unread: false, CreatedAtMs: now,
	})
	if err != nil {
		return AcceptResult{}, err
	}
	if err := tx.InsertOutbox(ctx, store.Outbox{
		UserID: in.UserID, MessageID: msg.ID, Op: store.IndexOpUpsert,
		PayloadJSON: string(mustJSON(map[string]any{"userId": in.UserID, "sessionId": session.ID, "messageId": msg.ID, "role": store.RoleUser, "content": text, "createdAtMs": now})),
		CreatedAtMs: now,
	}); err != nil {
		return AcceptResult{}, err
	}
	thread.LastMessageID = msg.ID
	thread.LastMessagePreview = store.Preview(text, 80)
	thread.LastMessageAtMs = now
	thread.SessionID = session.ID
	thread.UpdatedAtMs = now

	if _, err := tx.CancelOpenBackground(ctx, in.UserID, []string{store.SourceWatch, store.SourceMemoryReview}); err != nil {
		return AcceptResult{}, err
	}
	if err := tx.ResetUnsentBuckets(ctx, in.UserID); err != nil {
		return AcceptResult{}, err
	}

	var active *store.Run
	if thread.ActiveRunID > 0 {
		active, err = tx.GetRun(ctx, thread.ActiveRunID)
		if err != nil {
			active = nil
		}
	}
	disposition := DecideDisposition(active)
	payload := mustJSON(inputPayload{Text: text, MessageID: msg.ID, Attachments: in.Attachments, ContextPostID: in.ContextPostID})
	var runID int64
	switch disposition {
	case store.DispositionStarted:
		run, err := tx.InsertRun(ctx, store.Run{
			UserID: in.UserID, SessionID: session.ID, RequestID: in.RequestID, Source: store.SourceUser,
			Status: store.StatusQueued, Phase: store.PhaseQueued, Priority: store.PriorityUser,
			QueuedPayload: payload, ConsentVersion: in.ConsentVersion, InputVersion: 1,
			PromptEpoch: session.PromptEpoch, CreatedAtMs: now, LastActivityAtMs: now,
		})
		if err != nil {
			return AcceptResult{}, err
		}
		runID = run.ID
		thread.ActiveRunID = run.ID
		msg.RunID = run.ID
	case store.DispositionRedirected, store.DispositionSteered:
		runID = active.ID
		if err := tx.SetRunInput(ctx, active.ID, payload, now); err != nil {
			return AcceptResult{}, err
		}
	case store.DispositionQueued:
		n, err := tx.CountQueue(ctx, active.ID)
		if err != nil {
			return AcceptResult{}, err
		}
		if err := EnqueueOrReject(n); err != nil {
			return AcceptResult{}, err
		}
		if _, err := tx.Enqueue(ctx, store.QueueItem{UserID: in.UserID, RunID: active.ID, MessageID: msg.ID, CreatedAtMs: now}); err != nil {
			return AcceptResult{}, err
		}
		runID = active.ID
	}
	if err := tx.SaveThread(ctx, *thread); err != nil {
		return AcceptResult{}, err
	}
	return AcceptResult{MessageID: msg.ID, SessionID: session.ID, RunID: runID, Disposition: disposition}, nil
}

func (a *Acceptor) ensureSession(ctx context.Context, tx store.Store, thread *store.Thread, now int64) (*store.Session, error) {
	if thread.SessionID > 0 {
		session, err := tx.GetSession(ctx, thread.SessionID)
		if err == nil && session != nil && session.Status != store.SessionClosed {
			return session, nil
		}
	}
	session, err := a.newSession(ctx, tx, thread.UserID, now)
	if err != nil {
		return nil, err
	}
	thread.SessionID = session.ID
	return &session, nil
}

func (a *Acceptor) newSession(ctx context.Context, tx store.Store, userID, now int64) (store.Session, error) {
	var entries []memory.Entry
	if a.Memory != nil {
		listed, err := a.Memory.Active(ctx, userID)
		if err == nil {
			entries = listed
		}
	}
	snap := prompt.BuildSnapshot(entries, nil, "")
	return tx.CreateSession(ctx, store.Session{
		UserID: userID, PromptEpoch: 1, PromptSnapshot: prompt.EncodeSnapshot(snap),
		Status: store.SessionOpen, CreatedAtMs: now,
	})
}

func (a *Acceptor) CreateSession(ctx context.Context, userID int64) (int64, error) {
	if userID <= 0 {
		return 0, errx.NewWithCode(errx.LoginRequired)
	}
	var id int64
	err := a.Store.Transact(ctx, func(ctx context.Context, tx store.Store) error {
		now := store.NowMs()
		thread, err := tx.LockThread(ctx, userID)
		if err != nil {
			return err
		}
		if thread.SessionID > 0 {
			session, err := tx.GetSession(ctx, thread.SessionID)
			if err == nil && session != nil {
				session.PromptEpoch++
				session.Status = store.SessionClosed
				session.ClosedAtMs = now
				if err := tx.UpdateSession(ctx, *session); err != nil {
					return err
				}
			} else {
				_ = tx.CloseSession(ctx, thread.SessionID, now)
			}
		}
		created, err := a.newSession(ctx, tx, userID, now)
		if err != nil {
			return err
		}
		thread.SessionID = created.ID
		thread.UpdatedAtMs = now
		id = created.ID
		return tx.SaveThread(ctx, *thread)
	})
	return id, err
}

func (a *Acceptor) DeleteHistory(ctx context.Context, userID int64) error {
	if userID <= 0 {
		return errx.NewWithCode(errx.LoginRequired)
	}
	return a.Store.Transact(ctx, func(ctx context.Context, tx store.Store) error {
		now := store.NowMs()
		ids, err := tx.SoftDeleteMessages(ctx, userID, now)
		if err != nil {
			return err
		}
		for _, id := range ids {
			_ = tx.InsertOutbox(ctx, store.Outbox{UserID: userID, MessageID: id, Op: store.IndexOpDelete, CreatedAtMs: now})
		}
		thread, err := tx.LockThread(ctx, userID)
		if err != nil {
			return err
		}
		thread.LastMessageID = 0
		thread.LastMessagePreview = ""
		thread.LastMessageAtMs = 0
		thread.UnreadCount = 0
		thread.UpdatedAtMs = now
		return tx.SaveThread(ctx, *thread)
	})
}

func (a *Acceptor) MarkRead(ctx context.Context, userID int64) (int32, error) {
	if userID <= 0 {
		return 0, errx.NewWithCode(errx.LoginRequired)
	}
	var unread int32
	err := a.Store.Transact(ctx, func(ctx context.Context, tx store.Store) error {
		if err := tx.MarkMessagesRead(ctx, userID); err != nil {
			return err
		}
		thread, err := tx.LockThread(ctx, userID)
		if err != nil {
			return err
		}
		thread.UnreadCount = 0
		thread.UpdatedAtMs = store.NowMs()
		unread = 0
		return tx.SaveThread(ctx, *thread)
	})
	return unread, err
}

func mustJSON(v any) []byte {
	raw, _ := json.Marshal(v)
	return raw
}

func providerUserContent(text string, attachments []Attachment, contextPostID int64) string {
	if len(attachments) == 0 && contextPostID <= 0 {
		return text
	}
	contextJSON, _ := json.Marshal(struct {
		Attachments   []Attachment `json:"attachments,omitempty"`
		ContextPostID int64        `json:"context_post_id,omitempty"`
	}{Attachments: attachments, ContextPostID: contextPostID})
	return text + "\n\nUNTRUSTED_USER_INPUT_CONTEXT_JSON:\n" + string(contextJSON)
}

func decodeInputPayload(raw []byte) inputPayload {
	var payload inputPayload
	_ = json.Unmarshal(raw, &payload)
	return payload
}

func itoa(v int64) string {
	if v == 0 {
		return "0"
	}
	neg := false
	if v < 0 {
		neg = true
		v = -v
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
