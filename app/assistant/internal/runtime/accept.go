package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"unicode/utf8"

	"esx/app/assistant/internal/memory"
	"esx/app/assistant/internal/prompt"
	"esx/app/assistant/internal/store"
	"esx/pkg/errx"
)

// Attachment 是用户随消息附带的媒体引用。
type Attachment struct {
	MediaID int64  `json:"media_id"`
	URL     string `json:"url"`
}

// inputPayload 是写入 run.queued_payload 的用户输入，worker 据此构造提示词。
type inputPayload struct {
	Text          string       `json:"text"`
	MessageID     int64        `json:"message_id"`
	Attachments   []Attachment `json:"attachments,omitempty"`
	ContextPostID int64        `json:"context_post_id,omitempty"`
}

// AcceptInput 是一次用户发言的入参；ConsentOK/ConsentVersion 来自调用方读取的授权状态。
type AcceptInput struct {
	ClientProtocolVersion int
	QuestionContext       *QuestionContext
	questionContextJSON   string
	UserID                int64
	Message               string
	RequestID             string
	Attachments           []Attachment
	ContextPostID         int64
	ConsentOK             bool
	ConsentVersion        int32
}

// AcceptResult 说明消息落在哪个 run 以及处置方式。
type AcceptResult struct {
	MessageID   int64
	SessionID   int64
	RunID       int64
	Disposition string
}

// Acceptor 处理用户输入与历史删除，与 worker 共用 store 的加锁顺序。
type Acceptor struct {
	Store    store.Store
	Memory   memory.Store
	Notify   store.Notifier
	MaxRunes int
}

// Accept 校验并接收一条用户消息，交给新 run 或正在运行的 run。
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
	// 旧客户端可能不带请求 ID，用时间戳兜底以便仍能落库。
	if strings.TrimSpace(in.RequestID) == "" {
		in.RequestID = "msg-" + strconv.FormatInt(store.NowMs(), 10)
	}
	if in.ClientProtocolVersion == 0 {
		in.ClientProtocolVersion = 1
	}
	if in.ClientProtocolVersion < 1 || in.ClientProtocolVersion > 2 {
		return AcceptResult{}, errx.NewWithCode(errx.ParamError)
	}
	if in.QuestionContext != nil {
		if in.ClientProtocolVersion < 2 {
			return AcceptResult{}, errx.NewWithCode(errx.ParamError)
		}
		var err error
		in.questionContextJSON, err = a.questionContext(ctx, in.UserID, in.QuestionContext)
		if err != nil {
			return AcceptResult{}, err
		}
	}
	// 先结算已超时的追问等待，使后续处置基于最新的 run 状态。
	if thread, err := a.Store.GetThread(ctx, in.UserID); err == nil && thread.ActiveRunID > 0 {
		if err := ResolveWaiting(ctx, a.Store, a.Notify, thread.ActiveRunID, store.NowMs()); err != nil {
			return AcceptResult{}, err
		}
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

// acceptTx 在一个事务内接收用户输入：复核授权 → 幂等重放 → 抢占后台 run 并锁线程 →
// 写入用户消息 → 按当前活跃 run 的状态决定处置并交给 run → 记录输入命令与线程摘要。
func (a *Acceptor) acceptTx(ctx context.Context, tx store.Store, in AcceptInput, text string) (AcceptResult, error) {
	now := store.NowMs()
	consentVersion, granted, err := tx.AgentConsent(ctx, in.UserID)
	if err != nil {
		return AcceptResult{}, err
	}
	if !granted || consentVersion != in.ConsentVersion {
		return AcceptResult{}, errx.NewWithCode(errx.AgentNotAuthorized)
	}
	if result, found, err := replayAcceptedInput(ctx, tx, in, text); found || err != nil {
		return result, err
	}

	// Worker steps lock agent_run before assistant_thread. Preemption must take
	// background run locks first to keep the same order during final delivery.
	if _, err := tx.CancelOpenBackground(ctx, in.UserID, []string{store.SourceMemoryReview}); err != nil {
		return AcceptResult{}, err
	}
	thread, err := tx.LockThread(ctx, in.UserID)
	if err != nil {
		return AcceptResult{}, err
	}
	// A concurrent retry can finish while this transaction waits for the thread.
	if result, found, err := replayAcceptedInput(ctx, tx, in, text); found || err != nil {
		return result, err
	}

	session, err := ensureForegroundSession(ctx, tx, a.Memory, thread, now)
	if err != nil {
		return AcceptResult{}, err
	}
	cold := isColdConversation(thread, now)

	msg, err := insertAcceptedMessage(ctx, tx, in, text, session.ID, now)
	if err != nil {
		return AcceptResult{}, err
	}

	thread.LastMessageID = msg.ID
	thread.LastMessagePreview = store.Preview(text, 80)
	thread.LastMessageAtMs = now
	thread.SessionID = session.ID
	thread.UpdatedAtMs = now

	active, disposition, err := activeInputDisposition(ctx, tx, in.UserID, thread.ActiveRunID, in.ClientProtocolVersion)
	if err != nil {
		return AcceptResult{}, err
	}

	payload := mustJSON(inputPayload{Text: text, MessageID: msg.ID, Attachments: in.Attachments, ContextPostID: in.ContextPostID})
	route := acceptedRoute{in: in, thread: thread, session: session, messageID: msg.ID, cold: cold, payload: payload, now: now}
	runID, err := a.routeInput(ctx, tx, &route, active, disposition)
	if err != nil {
		return AcceptResult{}, err
	}
	session = route.session
	// 记录输入命令，同一 requestID 的重试据此直接返回本次结果。
	if _, err := tx.InsertInputCommand(ctx, store.InputCommand{
		UserID: in.UserID, RequestID: in.RequestID, SessionID: session.ID,
		MessageID: msg.ID, RunID: runID, Disposition: disposition, CreatedAtMs: now,
	}); err != nil {
		return AcceptResult{}, err
	}
	if err := tx.SaveThread(ctx, *thread); err != nil {
		return AcceptResult{}, err
	}
	return AcceptResult{MessageID: msg.ID, SessionID: session.ID, RunID: runID, Disposition: disposition}, nil
}

// acceptedRoute 是已写入的用户消息交给 run 时需要的上下文；routeInput 可能替换 session（冷会话拼接）
// 并更新 thread 的活跃 run。
type acceptedRoute struct {
	in        AcceptInput
	thread    *store.Thread
	session   *store.Session
	messageID int64
	cold      bool
	payload   []byte
	now       int64
}

// routeInput 按处置把输入交给 run，返回承接输入的 run ID：
// started 新建排队的用户 run（冷会话先拼接新会话）；redirected/steered 改写当前 run 的输入；
// queued 排到当前 run 之后，队列已满时拒绝。
func (a *Acceptor) routeInput(ctx context.Context, tx store.Store, route *acceptedRoute, active *store.Run, disposition string) (int64, error) {
	in, now := route.in, route.now
	switch disposition {
	case store.DispositionStarted:
		if route.cold {
			session, err := spliceColdSession(ctx, tx, a.Memory, route.session)
			if err != nil {
				return 0, err
			}
			route.session = session
		}
		run, err := tx.InsertRun(ctx, store.Run{
			ClientProtocolVersion: in.ClientProtocolVersion,
			UserID:                in.UserID, SessionID: route.session.ID, RequestID: in.RequestID, Source: store.SourceUser,
			Status: store.StatusQueued, Phase: store.PhaseQueued, Priority: store.PriorityUser,
			QueuedPayload: route.payload, ConsentVersion: in.ConsentVersion, InputVersion: 1,
			PromptEpoch: route.session.PromptEpoch, CreatedAtMs: now, LastActivityAtMs: now,
		})
		if err != nil {
			return 0, err
		}
		route.thread.ActiveRunID = run.ID
		return run.ID, nil
	case store.DispositionRedirected, store.DispositionSteered:
		if err := tx.SetRunInput(ctx, active.ID, route.payload, now); err != nil {
			return 0, err
		}
		return active.ID, nil
	case store.DispositionQueued:
		n, err := tx.CountQueue(ctx, active.ID)
		if err != nil {
			return 0, err
		}
		if err := EnqueueOrReject(n); err != nil {
			return 0, err
		}
		if _, err := tx.Enqueue(ctx, store.QueueItem{UserID: in.UserID, RunID: active.ID, MessageID: route.messageID, CreatedAtMs: now}); err != nil {
			return 0, err
		}
		return active.ID, nil
	}
	return 0, nil
}

// MarkRead 把用户的助手消息全部标为已读并清零线程未读数。
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

// mustJSON 编码内部结构；这些结构总能序列化，故忽略错误。
func mustJSON(v any) []byte {
	raw, _ := json.Marshal(v)
	return raw
}

// acceptedUserContent 是写入提示词的用户内容；追问续答以不可信 JSON 附在正文之后。
func acceptedUserContent(text string, in AcceptInput) string {
	value := providerUserContent(text, in.Attachments, in.ContextPostID)
	if in.questionContextJSON != "" {
		value += "\n\nUNTRUSTED_QUESTION_CONTINUATION_JSON:\n" + in.questionContextJSON
	}
	return value
}

// verifyInputReplay 确认同一请求 ID 的重试内容与首次一致（协议 v2 起），不一致即幂等冲突。
func verifyInputReplay(ctx context.Context, st store.Store, in AcceptInput, text string, existing store.InputCommand) error {
	if in.ClientProtocolVersion < 2 {
		return nil
	}
	msg, err := st.GetMessage(ctx, in.UserID, existing.MessageID)
	if err != nil {
		return err
	}
	if msg.DeletedAtMs > 0 {
		return errx.NewWithCode(errx.NotFound)
	}
	expected := prompt.EncodeTurn(prompt.Turn{Role: store.RoleUser, Content: acceptedUserContent(text, in)})
	if !bytes.Equal(expected, msg.APIContent) {
		return errx.NewWithCode(errx.IdempotencyConflict)
	}
	return nil
}

// providerUserContent 把附件与上下文帖子以标注为不可信的 JSON 附在用户正文后，防止被当作指令。
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

// decodeInputPayload 解码 queued_payload；损坏时返回零值。
func decodeInputPayload(raw []byte) inputPayload {
	var payload inputPayload
	_ = json.Unmarshal(raw, &payload)
	return payload
}

// replayAcceptedInput 查找同一请求 ID 的既有结果；found=true 时调用方直接返回该结果。
func replayAcceptedInput(ctx context.Context, tx store.Store, in AcceptInput, text string) (AcceptResult, bool, error) {
	if existing, err := tx.GetInputCommand(ctx, in.UserID, in.RequestID); err != nil {
		return AcceptResult{}, true, err
	} else if existing != nil {
		if err := verifyInputReplay(ctx, tx, in, text, *existing); err != nil {
			return AcceptResult{}, true, err
		}
		return AcceptResult{MessageID: existing.MessageID, SessionID: existing.SessionID, RunID: existing.RunID, Disposition: existing.Disposition}, true, nil
	}
	// 没有输入命令记录但已有同请求 ID 的 run：v1 客户端按已启动返回，v2 视为幂等冲突。
	if existing, err := tx.GetRunByRequestID(ctx, in.UserID, in.RequestID); err != nil {
		return AcceptResult{}, true, err
	} else if existing != nil {
		if in.ClientProtocolVersion >= 2 {
			return AcceptResult{}, true, errx.NewWithCode(errx.IdempotencyConflict)
		}
		return AcceptResult{SessionID: existing.SessionID, RunID: existing.ID, Disposition: store.DispositionStarted}, true, nil
	}
	return AcceptResult{}, false, nil
}

// insertAcceptedMessage 写入用户消息，并在同一事务登记搜索索引 outbox。
func insertAcceptedMessage(ctx context.Context, tx store.Store, in AcceptInput, text string, sessionID, now int64) (store.Message, error) {
	apiText := acceptedUserContent(text, in)
	api := prompt.EncodeTurn(prompt.Turn{Role: store.RoleUser, Content: apiText})
	msg, err := tx.InsertMessage(ctx, store.Message{
		UserID: in.UserID, SessionID: sessionID, Role: store.RoleUser, Kind: store.KindMessage,
		Content: text, APIContent: api, Visible: true, Unread: false, CreatedAtMs: now,
	})
	if err != nil {
		return store.Message{}, err
	}
	if err := tx.InsertOutbox(ctx, store.Outbox{
		UserID: in.UserID, MessageID: msg.ID, Op: store.IndexOpUpsert,
		PayloadJSON: string(mustJSON(map[string]any{"userId": in.UserID, "sessionId": sessionID, "messageId": msg.ID, "role": store.RoleUser, "content": text, "createdAtMs": now})),
		CreatedAtMs: now,
	}); err != nil {
		return store.Message{}, err
	}
	return msg, nil
}

// activeInputDisposition 读取当前活跃 run 的最新状态并决定本次输入的处置方式；
// 处于等待追问的 run 会先结束旧追问并回到排队。
func activeInputDisposition(ctx context.Context, tx store.Store, userID, activeRunID int64, protocolVersion int) (*store.Run, string, error) {
	var active *store.Run
	if activeRunID > 0 {
		// Acceptance has already performed snapshot reads (consent/idempotency).
		// Under MySQL REPEATABLE READ a plain GetRun can still see the phase
		// from before a worker commit. Decide from a current locking read.
		runs, lockErr := tx.LockOpenRuns(ctx, userID)
		if lockErr != nil {
			return nil, "", lockErr
		}
		for i := range runs {
			if runs[i].ID == activeRunID {
				active = &runs[i]
				break
			}
		}
	}
	disposition := DecideDisposition(active)
	if active != nil && active.Status == store.StatusWaitingInput {
		if protocolVersion < 2 {
			return nil, "", errx.New(errx.ParamError, "client update required for this interaction")
		}
		if err := supersedeQuestionsTx(ctx, tx, active); err != nil {
			return nil, "", err
		}
		disposition = store.DispositionSteered
	}
	return active, disposition, nil
}
