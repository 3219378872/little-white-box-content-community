# Message Module Correctness Repair Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Repair the `app/message` correctness issues around transactional message creation, read/unread consistency, MQ retry classification, targeted validation, and SQL indexes.

**Architecture:** Keep the go-zero handler/server/generated layers untouched. Add a small write-side command model in `app/message/internal/model` for cross-table mutations, keep request validation and error mapping in `internal/logic`, and keep MQ payload classification in `internal/mqs`.

**Tech Stack:** Go 1.26.1, go-zero v1.10.1, MySQL 8.0 via `sqlx`, Redis unread cache abstraction, RocketMQ consumer wrapper, `testcontainers-go` MySQL integration tests.

---

## File Structure

- Create `app/message/internal/model/message_command_model.go`
  - Owns cross-table writes for private messages and read marking.
  - Uses `sqlx.SqlConn.TransactCtx`.
- Create `app/message/internal/model/message_command_model_integration_test.go`
  - MySQL testcontainers coverage for transaction commit, rollback, and unread decrement.
- Modify `app/message/internal/svc/service_context.go`
  - Adds a narrow `MessageCommandModel` interface and wires `model.NewMessageCommandModel(conn)`.
- Modify `app/message/internal/logic/send_message_logic.go`
  - Uses `MessageCommandModel.CreateMessageWithConversations`.
  - Validates supported message type and content length.
- Modify `app/message/internal/logic/mark_read_logic.go`
  - Uses `MessageCommandModel.MarkConversationRead`.
  - Preserves not-found vs system-error mapping.
- Modify `app/message/internal/logic/get_messages_logic.go`
  - Preserves not-found vs system-error mapping.
- Create `app/message/internal/logic/validation.go`
  - Defines message and notification validation constants/helpers.
- Modify `app/message/internal/logic/send_notification_logic.go`
  - Validates notification type and title/content length before insert.
- Modify `app/message/internal/logic/message_logic_test.go`
  - Adds/updates fakes and tests for logic behavior.
- Modify `app/message/internal/mqs/message_consumer.go`
  - Adds permanent-event error classification and retry/ack split.
- Modify `app/message/internal/mqs/message_consumer_test.go`
  - Adds permanent vs transient MQ tests.
- Modify `deploy/sql/xbh_message.sql`
  - Adds composite indexes for current message/notification read paths.

Do not add or commit `docs/superpowers/specs/*` or `docs/superpowers/plans/*`. If execution uses commits, commit only code/test/SQL files.

---

### Task 1: Add Transactional Message Command Model

**Files:**
- Create: `app/message/internal/model/message_command_model_integration_test.go`
- Create: `app/message/internal/model/message_command_model.go`

- [ ] **Step 1: Write failing MySQL integration tests**

Create `app/message/internal/model/message_command_model_integration_test.go`:

```go
//go:build integration

package model

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	mysqlcontainer "github.com/testcontainers/testcontainers-go/modules/mysql"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

func newMessageTestDB(t *testing.T) (sqlx.SqlConn, func()) {
	t.Helper()
	ctx := context.Background()

	root, err := filepath.Abs("../../../..")
	require.NoError(t, err)
	scriptPath := filepath.Join(root, "deploy", "sql", "xbh_message.sql")

	password := os.Getenv("MYSQL_ROOT_PASSWORD")
	if password == "" {
		password = "Xbh@MySQL2024!"
	}

	container, err := mysqlcontainer.Run(ctx,
		"mysql:8.0",
		mysqlcontainer.WithDatabase("xbh_message"),
		mysqlcontainer.WithUsername("root"),
		mysqlcontainer.WithPassword(password),
		mysqlcontainer.WithScripts(scriptPath),
		testcontainers.WithEnv(map[string]string{
			"TZ":   "Asia/Shanghai",
			"LANG": "C.UTF-8",
		}),
		testcontainers.WithCmd(
			"--default-authentication-plugin=mysql_native_password",
			"--character-set-server=utf8mb4",
			"--collation-server=utf8mb4_unicode_ci",
			"--sql-mode=STRICT_TRANS_TABLES,NO_ENGINE_SUBSTITUTION",
		),
	)
	require.NoError(t, err)

	dsn, err := container.ConnectionString(ctx, "charset=utf8mb4", "parseTime=true", "loc=Asia%2FShanghai")
	require.NoError(t, err)
	db, err := sql.Open("mysql", dsn)
	require.NoError(t, err)
	require.NoError(t, db.PingContext(ctx))

	cleanup := func() {
		_ = db.Close()
		require.NoError(t, testcontainers.TerminateContainer(container))
	}
	return sqlx.NewSqlConnFromDB(db), cleanup
}

func countMessageRows(t *testing.T, conn sqlx.SqlConn, query string, args ...any) int64 {
	t.Helper()
	var count int64
	require.NoError(t, conn.QueryRowCtx(context.Background(), &count, query, args...))
	return count
}

func TestMessageCommandModelCreateMessageWithConversationsCommitsAllRows(t *testing.T) {
	conn, cleanup := newMessageTestDB(t)
	defer cleanup()

	command := NewMessageCommandModel(conn)
	messageID, err := command.CreateMessageWithConversations(context.Background(), 1, 2, "hello", 1)
	require.NoError(t, err)
	require.Positive(t, messageID)

	var senderUnread int64
	require.NoError(t, conn.QueryRowCtx(context.Background(), &senderUnread,
		"select unread_count from conversation where user_id = ? and target_user_id = ?", 1, 2))
	require.Equal(t, int64(0), senderUnread)

	var receiverConversationID int64
	var receiverUnread int64
	require.NoError(t, conn.QueryRowCtx(context.Background(), &receiverConversationID,
		"select id from conversation where user_id = ? and target_user_id = ?", 2, 1))
	require.NoError(t, conn.QueryRowCtx(context.Background(), &receiverUnread,
		"select unread_count from conversation where user_id = ? and target_user_id = ?", 2, 1))
	require.Equal(t, int64(1), receiverUnread)

	var storedConversationID int64
	var storedContent string
	require.NoError(t, conn.QueryRowCtx(context.Background(), &storedConversationID,
		"select conversation_id from message where id = ?", messageID))
	require.NoError(t, conn.QueryRowCtx(context.Background(), &storedContent,
		"select content from message where id = ?", messageID))
	require.Equal(t, receiverConversationID, storedConversationID)
	require.Equal(t, "hello", storedContent)
}

func TestMessageCommandModelCreateMessageRollsBackWhenMessageInsertFails(t *testing.T) {
	conn, cleanup := newMessageTestDB(t)
	defer cleanup()

	command := NewMessageCommandModel(conn)
	_, err := command.CreateMessageWithConversations(context.Background(), 1, 2, strings.Repeat("x", 1001), 1)
	require.Error(t, err)

	require.Equal(t, int64(0), countMessageRows(t, conn, "select count(*) from conversation"))
	require.Equal(t, int64(0), countMessageRows(t, conn, "select count(*) from message"))
}

func TestMessageCommandModelMarkConversationReadDecrementsUnreadByAffectedRows(t *testing.T) {
	conn, cleanup := newMessageTestDB(t)
	defer cleanup()

	command := NewMessageCommandModel(conn)
	_, err := command.CreateMessageWithConversations(context.Background(), 8, 7, "first", 1)
	require.NoError(t, err)
	_, err = command.CreateMessageWithConversations(context.Background(), 8, 7, "second", 1)
	require.NoError(t, err)

	var beforeUnread int64
	require.NoError(t, conn.QueryRowCtx(context.Background(), &beforeUnread,
		"select unread_count from conversation where user_id = ? and target_user_id = ?", 7, 8))
	require.Equal(t, int64(2), beforeUnread)

	affected, err := command.MarkConversationRead(context.Background(), 7, 8)
	require.NoError(t, err)
	require.Equal(t, int64(2), affected)

	var afterUnread int64
	require.NoError(t, conn.QueryRowCtx(context.Background(), &afterUnread,
		"select unread_count from conversation where user_id = ? and target_user_id = ?", 7, 8))
	require.Equal(t, int64(0), afterUnread)
	require.Equal(t, int64(0), countMessageRows(t, conn,
		"select count(*) from message where receiver_id = ? and sender_id = ? and status = 0", 7, 8))
}
```

- [ ] **Step 2: Run integration test to verify it fails**

Run:

```bash
GOCACHE=/tmp/go-build-cache go test -tags=integration ./app/message/internal/model -run TestMessageCommandModel -count=1
```

Expected: FAIL to compile with `undefined: NewMessageCommandModel`.

- [ ] **Step 3: Implement the command model**

Create `app/message/internal/model/message_command_model.go`:

```go
package model

import (
	"context"
	"database/sql"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

var _ MessageCommandModel = (*customMessageCommandModel)(nil)

type (
	MessageCommandModel interface {
		CreateMessageWithConversations(ctx context.Context, senderID int64, receiverID int64, content string, msgType int64) (int64, error)
		MarkConversationRead(ctx context.Context, userID int64, targetUserID int64) (int64, error)
	}

	customMessageCommandModel struct {
		conn sqlx.SqlConn
	}
)

func NewMessageCommandModel(conn sqlx.SqlConn) MessageCommandModel {
	return &customMessageCommandModel{conn: conn}
}

func (m *customMessageCommandModel) CreateMessageWithConversations(ctx context.Context, senderID int64, receiverID int64, content string, msgType int64) (int64, error) {
	var messageID int64
	err := m.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		if err := upsertConversationForMessage(ctx, session, senderID, receiverID, content, 0); err != nil {
			return err
		}
		if err := upsertConversationForMessage(ctx, session, receiverID, senderID, content, 1); err != nil {
			return err
		}

		var receiverConversationID int64
		if err := session.QueryRowCtx(ctx, &receiverConversationID,
			"select `id` from `conversation` where `user_id` = ? and `target_user_id` = ? limit 1",
			receiverID, senderID); err != nil {
			return err
		}

		result, err := session.ExecCtx(ctx,
			"insert into `message` (`conversation_id`, `sender_id`, `receiver_id`, `content`, `msg_type`, `status`) values (?, ?, ?, ?, ?, 0)",
			receiverConversationID, senderID, receiverID, content, msgType)
		if err != nil {
			return err
		}
		messageID, err = result.LastInsertId()
		return err
	})
	if err != nil {
		return 0, err
	}
	return messageID, nil
}

func upsertConversationForMessage(ctx context.Context, session sqlx.Session, userID int64, targetUserID int64, content string, unreadIncrement int64) error {
	_, err := session.ExecCtx(ctx, `insert into conversation (user_id, target_user_id, last_message, last_message_time, unread_count)
values (?, ?, ?, now(), ?)
on duplicate key update last_message = values(last_message), last_message_time = values(last_message_time), unread_count = unread_count + ?`,
		userID, targetUserID, content, unreadIncrement, unreadIncrement)
	return err
}

func (m *customMessageCommandModel) MarkConversationRead(ctx context.Context, userID int64, targetUserID int64) (int64, error) {
	var affected int64
	err := m.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		result, err := session.ExecCtx(ctx,
			"update `message` set `status` = 1 where `receiver_id` = ? and `sender_id` = ? and `status` = 0",
			userID, targetUserID)
		if err != nil {
			return err
		}
		affected, err = result.RowsAffected()
		if err != nil {
			return err
		}
		if affected == 0 {
			return nil
		}
		_, err = session.ExecCtx(ctx,
			"update `conversation` set `unread_count` = greatest(`unread_count` - ?, 0) where `user_id` = ? and `target_user_id` = ?",
			affected, userID, targetUserID)
		return err
	})
	if err != nil {
		return 0, err
	}
	return affected, nil
}

var _ sql.Result
```

- [ ] **Step 4: Run integration test to verify it passes**

Run:

```bash
GOCACHE=/tmp/go-build-cache go test -tags=integration ./app/message/internal/model -run TestMessageCommandModel -count=1
```

Expected: PASS for the three `TestMessageCommandModel...` tests.

- [ ] **Step 5: Run non-integration model package tests**

Run:

```bash
GOCACHE=/tmp/go-build-cache go test ./app/message/internal/model
```

Expected: PASS or `[no test files]` with exit code 0.

- [ ] **Step 6: Commit code files only**

Run:

```bash
git add app/message/internal/model/message_command_model.go app/message/internal/model/message_command_model_integration_test.go
git commit -m "fix(message): add transactional message command model"
```

Do not add files under `docs/superpowers/`.

---

### Task 2: Wire Logic Through MessageCommandModel

**Files:**
- Modify: `app/message/internal/svc/service_context.go`
- Modify: `app/message/internal/logic/message_logic_test.go`
- Create: `app/message/internal/logic/validation.go`
- Modify: `app/message/internal/logic/send_message_logic.go`
- Modify: `app/message/internal/logic/mark_read_logic.go`

- [ ] **Step 1: Write failing logic tests for command-model usage and message validation**

Modify `app/message/internal/logic/message_logic_test.go`.

Add this fake after `fakeMessageModel`:

```go
type fakeMessageCommandModel struct {
	createdSenderID   int64
	createdReceiverID int64
	createdContent    string
	createdMsgType    int64
	createdMessageID  int64
	createCalls       int64
	createErr         error
	markUserID        int64
	markTargetID      int64
	markCalls         int64
	markErr           error
}

func (m *fakeMessageCommandModel) CreateMessageWithConversations(ctx context.Context, senderID int64, receiverID int64, content string, msgType int64) (int64, error) {
	m.createCalls++
	m.createdSenderID = senderID
	m.createdReceiverID = receiverID
	m.createdContent = content
	m.createdMsgType = msgType
	if m.createErr != nil {
		return 0, m.createErr
	}
	if m.createdMessageID == 0 {
		m.createdMessageID = 300
	}
	return m.createdMessageID, nil
}

func (m *fakeMessageCommandModel) MarkConversationRead(ctx context.Context, userID int64, targetUserID int64) (int64, error) {
	m.markCalls++
	m.markUserID = userID
	m.markTargetID = targetUserID
	if m.markErr != nil {
		return 0, m.markErr
	}
	return 3, nil
}
```

Replace `TestSendMessageCreatesConversationAndMessage` with:

```go
func TestSendMessageCreatesMessageThroughCommandModel(t *testing.T) {
	commands := &fakeMessageCommandModel{createdMessageID: 301}
	store := &fakeUnreadStore{}
	ctx := &svc.ServiceContext{MessageCommandModel: commands, UnreadStore: store}

	resp, err := NewSendMessageLogic(context.Background(), ctx).SendMessage(&pb.SendMessageReq{SenderId: 1, ReceiverId: 2, Content: " hello ", MsgType: 1})

	require.NoError(t, err)
	require.Equal(t, int64(301), resp.MessageId)
	require.Equal(t, int64(1), commands.createCalls)
	require.Equal(t, int64(1), commands.createdSenderID)
	require.Equal(t, int64(2), commands.createdReceiverID)
	require.Equal(t, "hello", commands.createdContent)
	require.Equal(t, int64(1), commands.createdMsgType)
	require.Equal(t, []int64{2}, store.deleted)
}
```

Add these tests near `TestSendMessageRejectsInvalidRequest`:

```go
func TestSendMessageRejectsUnsupportedMessageType(t *testing.T) {
	commands := &fakeMessageCommandModel{}
	_, err := NewSendMessageLogic(context.Background(), &svc.ServiceContext{MessageCommandModel: commands}).SendMessage(&pb.SendMessageReq{
		SenderId: 1, ReceiverId: 2, Content: "hello", MsgType: 9,
	})

	require.Error(t, err)
	require.True(t, errx.Is(err, errx.ParamError))
	require.Equal(t, int64(0), commands.createCalls)
}

func TestSendMessageRejectsOversizedContent(t *testing.T) {
	commands := &fakeMessageCommandModel{}
	_, err := NewSendMessageLogic(context.Background(), &svc.ServiceContext{MessageCommandModel: commands}).SendMessage(&pb.SendMessageReq{
		SenderId: 1, ReceiverId: 2, Content: strings.Repeat("x", 1001), MsgType: 1,
	})

	require.Error(t, err)
	require.True(t, errx.Is(err, errx.ParamError))
	require.Equal(t, int64(0), commands.createCalls)
}
```

Add `strings` to the test imports.

Replace `TestMarkReadWithConversationMarksOnlyConversationMessages` with:

```go
func TestMarkReadWithConversationMarksOnlyConversationMessages(t *testing.T) {
	conversations := &fakeConversationModel{conversation: &model.Conversation{Id: 11, UserId: 7, TargetUserId: 8}}
	commands := &fakeMessageCommandModel{}
	notifications := &fakeNotificationModel{}
	store := &fakeUnreadStore{}
	ctx := &svc.ServiceContext{ConversationModel: conversations, MessageCommandModel: commands, NotificationModel: notifications, UnreadStore: store}

	_, err := NewMarkReadLogic(context.Background(), ctx).MarkRead(&pb.MarkReadReq{UserId: 7, ConversationId: 11})

	require.NoError(t, err)
	require.Equal(t, int64(1), commands.markCalls)
	require.Equal(t, int64(7), commands.markUserID)
	require.Equal(t, int64(8), commands.markTargetID)
	require.Equal(t, int64(0), notifications.marked)
	require.Equal(t, []int64{7}, store.deleted)
}
```

- [ ] **Step 2: Run logic tests to verify they fail**

Run:

```bash
GOCACHE=/tmp/go-build-cache go test ./app/message/internal/logic -run 'TestSendMessage|TestMarkRead' -count=1
```

Expected: FAIL to compile because `svc.ServiceContext` has no `MessageCommandModel` field.

- [ ] **Step 3: Add service-context interface and wiring**

Modify `app/message/internal/svc/service_context.go`.

Add this interface after `MessageModel`:

```go
type MessageCommandModel interface {
	CreateMessageWithConversations(ctx context.Context, senderID int64, receiverID int64, content string, msgType int64) (int64, error)
	MarkConversationRead(ctx context.Context, userID int64, targetUserID int64) (int64, error)
}
```

Add the field:

```go
	MessageCommandModel MessageCommandModel
```

Set it in `NewServiceContext`:

```go
		MessageCommandModel: model.NewMessageCommandModel(conn),
```

- [ ] **Step 4: Add validation helpers**

Create `app/message/internal/logic/validation.go`:

```go
package logic

const (
	maxMessageContentLength      = 1000
	maxNotificationTitleLength   = 100
	maxNotificationContentLength = 500
)

func validMessageType(msgType int32) bool {
	return msgType >= 1 && msgType <= 4
}

func validNotificationType(notificationType int32) bool {
	return notificationType >= 1 && notificationType <= 5
}

func runeLen(value string) int {
	return len([]rune(value))
}
```

- [ ] **Step 5: Route `SendMessageLogic` through the command model**

Replace the body of `SendMessage` in `app/message/internal/logic/send_message_logic.go` with:

```go
func (l *SendMessageLogic) SendMessage(in *pb.SendMessageReq) (*pb.SendMessageResp, error) {
	content := strings.TrimSpace(in.Content)
	if in.SenderId <= 0 ||
		in.ReceiverId <= 0 ||
		in.SenderId == in.ReceiverId ||
		content == "" ||
		!validMessageType(in.MsgType) ||
		runeLen(content) > maxMessageContentLength {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	id, err := l.svcCtx.MessageCommandModel.CreateMessageWithConversations(l.ctx, in.SenderId, in.ReceiverId, content, int64(in.MsgType))
	if err != nil {
		l.Errorw("MessageCommandModel.CreateMessageWithConversations failed", logx.Field("err", err.Error()))
		return nil, errx.Wrap(err, errx.SystemError)
	}
	if l.svcCtx.UnreadStore != nil {
		if err := l.svcCtx.UnreadStore.DeleteUserUnread(l.ctx, in.ReceiverId); err != nil {
			l.Errorw("UnreadStore.DeleteUserUnread failed", logx.Field("err", err.Error()))
		}
	}
	return &pb.SendMessageResp{MessageId: id}, nil
}
```

Remove the now-unused `esx/app/message/internal/model` import from `send_message_logic.go`.

- [ ] **Step 6: Route conversation `MarkRead` through the command model**

In `app/message/internal/logic/mark_read_logic.go`, change this line:

```go
if _, err := l.svcCtx.MessageModel.MarkConversationReadForUser(l.ctx, in.UserId, conversation.TargetUserId); err != nil {
```

to:

```go
if _, err := l.svcCtx.MessageCommandModel.MarkConversationRead(l.ctx, in.UserId, conversation.TargetUserId); err != nil {
```

Change the log message to:

```go
l.Errorw("MessageCommandModel.MarkConversationRead failed", logx.Field("err", err.Error()))
```

- [ ] **Step 7: Run logic tests to verify they pass**

Run:

```bash
GOCACHE=/tmp/go-build-cache go test ./app/message/internal/logic -run 'TestSendMessage|TestMarkRead' -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit code files only**

Run:

```bash
git add app/message/internal/svc/service_context.go app/message/internal/logic/validation.go app/message/internal/logic/send_message_logic.go app/message/internal/logic/mark_read_logic.go app/message/internal/logic/message_logic_test.go
git commit -m "fix(message): route private message writes through command model"
```

Do not add files under `docs/superpowers/`.

---

### Task 3: Fix Logic Error Classification and Notification Validation

**Files:**
- Modify: `app/message/internal/logic/get_messages_logic.go`
- Modify: `app/message/internal/logic/mark_read_logic.go`
- Modify: `app/message/internal/logic/send_notification_logic.go`
- Modify: `app/message/internal/logic/message_logic_test.go`

- [ ] **Step 1: Write failing tests for DB-error mapping and notification validation**

Modify `app/message/internal/logic/message_logic_test.go`.

Add `errors` to imports.

Add these tests:

```go
func TestGetMessagesReturnsSystemErrorForConversationLookupFailure(t *testing.T) {
	conversations := &fakeConversationModel{findOneErr: errors.New("db offline")}
	ctx := &svc.ServiceContext{ConversationModel: conversations, MessageModel: &fakeMessageModel{}}

	_, err := NewGetMessagesLogic(context.Background(), ctx).GetMessages(&pb.GetMessagesReq{UserId: 7, ConversationId: 12, PageSize: 20})

	require.Error(t, err)
	require.True(t, errx.Is(err, errx.SystemError))
}

func TestMarkReadReturnsSystemErrorForConversationLookupFailure(t *testing.T) {
	conversations := &fakeConversationModel{findOneErr: errors.New("db offline")}
	ctx := &svc.ServiceContext{ConversationModel: conversations, MessageCommandModel: &fakeMessageCommandModel{}}

	_, err := NewMarkReadLogic(context.Background(), ctx).MarkRead(&pb.MarkReadReq{UserId: 7, ConversationId: 12})

	require.Error(t, err)
	require.True(t, errx.Is(err, errx.SystemError))
}

func TestSendNotificationRejectsUnsupportedType(t *testing.T) {
	notifications := &fakeNotificationModel{}
	_, err := NewSendNotificationLogic(context.Background(), &svc.ServiceContext{NotificationModel: notifications}).SendNotification(&pb.SendNotificationReq{
		UserId: 7, Type: 9, Content: "bad type",
	})

	require.Error(t, err)
	require.True(t, errx.Is(err, errx.ParamError))
	require.Len(t, notifications.inserted, 0)
}

func TestSendNotificationRejectsOversizedFields(t *testing.T) {
	notifications := &fakeNotificationModel{}
	_, err := NewSendNotificationLogic(context.Background(), &svc.ServiceContext{NotificationModel: notifications}).SendNotification(&pb.SendNotificationReq{
		UserId: 7, Type: 4, Title: strings.Repeat("t", 101), Content: "system",
	})
	require.Error(t, err)
	require.True(t, errx.Is(err, errx.ParamError))

	_, err = NewSendNotificationLogic(context.Background(), &svc.ServiceContext{NotificationModel: notifications}).SendNotification(&pb.SendNotificationReq{
		UserId: 7, Type: 4, Content: strings.Repeat("c", 501),
	})
	require.Error(t, err)
	require.True(t, errx.Is(err, errx.ParamError))
	require.Len(t, notifications.inserted, 0)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
GOCACHE=/tmp/go-build-cache go test ./app/message/internal/logic -run 'TestGetMessagesReturnsSystemError|TestMarkReadReturnsSystemError|TestSendNotificationRejects' -count=1
```

Expected: FAIL because lookup errors are still mapped to `PermissionDenied` and notification validation is too loose.

- [ ] **Step 3: Fix `GetMessagesLogic` error mapping**

Modify imports in `app/message/internal/logic/get_messages_logic.go`:

```go
import (
	"context"
	"errors"

	"errx"
	"esx/app/message/internal/model"
	"esx/app/message/internal/svc"
	"esx/app/message/xiaobaihe/message/pb"

	"github.com/zeromicro/go-zero/core/logx"
)
```

Replace the ownership error block with:

```go
	if err != nil {
		l.Errorw("ConversationModel.FindOneForUser failed", logx.Field("err", err.Error()))
		if errors.Is(err, model.ErrNotFound) {
			return nil, errx.NewWithCode(errx.PermissionDenied)
		}
		return nil, errx.Wrap(err, errx.SystemError)
	}
```

- [ ] **Step 4: Fix `MarkReadLogic` error mapping**

Modify imports in `app/message/internal/logic/mark_read_logic.go`:

```go
import (
	"context"
	"errors"

	"errx"
	"esx/app/message/internal/model"
	"esx/app/message/internal/svc"
	"esx/app/message/xiaobaihe/message/pb"

	"github.com/zeromicro/go-zero/core/logx"
)
```

Replace the ownership error block with:

```go
			if err != nil {
				l.Errorw("ConversationModel.FindOneForUser failed", logx.Field("err", err.Error()))
				if errors.Is(err, model.ErrNotFound) {
					return nil, errx.NewWithCode(errx.PermissionDenied)
				}
				return nil, errx.Wrap(err, errx.SystemError)
			}
```

- [ ] **Step 5: Add notification validation**

Replace the first validation block in `app/message/internal/logic/send_notification_logic.go` with:

```go
title := strings.TrimSpace(in.Title)
content := strings.TrimSpace(in.Content)
if in.UserId <= 0 ||
	!validNotificationType(in.Type) ||
	content == "" ||
	runeLen(title) > maxNotificationTitleLength ||
	runeLen(content) > maxNotificationContentLength {
	return nil, errx.NewWithCode(errx.ParamError)
}
```

Then set the row fields from the trimmed values:

```go
row := &model.Notification{
	UserId:   in.UserId,
	Type:     int64(in.Type),
	Title:    nullableString(title),
	Content:  nullableString(content),
	TargetId: sql.NullInt64{Int64: in.TargetId, Valid: in.TargetId > 0},
	Status:   0,
}
```

Run `gofmt` after editing; the multi-line `if` must be indented by `gofmt`.

- [ ] **Step 6: Run tests to verify they pass**

Run:

```bash
GOCACHE=/tmp/go-build-cache go test ./app/message/internal/logic -run 'TestGetMessagesReturnsSystemError|TestMarkReadReturnsSystemError|TestSendNotificationRejects|TestGetMessagesRejectsConversationNotOwnedByUser' -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit code files only**

Run:

```bash
git add app/message/internal/logic/get_messages_logic.go app/message/internal/logic/mark_read_logic.go app/message/internal/logic/send_notification_logic.go app/message/internal/logic/message_logic_test.go
git commit -m "fix(message): preserve lookup errors and validate notifications"
```

Do not add files under `docs/superpowers/`.

---

### Task 4: Split Permanent MQ Payload Errors From Retryable Persistence Errors

**Files:**
- Modify: `app/message/internal/mqs/message_consumer.go`
- Modify: `app/message/internal/mqs/message_consumer_test.go`

- [ ] **Step 1: Write failing MQ tests**

Modify `app/message/internal/mqs/message_consumer_test.go`.

Add `errors` to imports.

Change `fakeNotificationModel` to:

```go
type fakeNotificationModel struct {
	inserted  []*model.Notification
	insertErr error
}

func (m *fakeNotificationModel) Insert(ctx context.Context, data *model.Notification) (sql.Result, error) {
	if m.insertErr != nil {
		return nil, m.insertErr
	}
	m.inserted = append(m.inserted, data)
	return fakeResult{id: 1}, nil
}
```

Replace `TestMessageConsumerRejectsMalformedPayload` with:

```go
func TestMessageConsumerClassifiesMalformedPayloadAsPermanent(t *testing.T) {
	consumer := NewMessageConsumer(&svc.ServiceContext{})

	err := consumer.Consume(context.Background(), []byte(`not-json`))

	require.Error(t, err)
	require.True(t, IsPermanentEventError(err))
}
```

Add these tests:

```go
func TestMessageConsumerClassifiesUnsupportedActionAsPermanent(t *testing.T) {
	consumer := NewMessageConsumer(&svc.ServiceContext{})

	err := consumer.Consume(context.Background(), []byte(`{"target_user_id":9,"action_type":99}`))

	require.Error(t, err)
	require.True(t, IsPermanentEventError(err))
}

func TestConsumeResultForErrorAcknowledgesPermanentError(t *testing.T) {
	result := consumeResultForError(context.Background(), "msg-1", newPermanentEventError("bad payload"))

	require.Equal(t, consumer.ConsumeSuccess, result)
}

func TestConsumeResultForErrorRetriesTransientError(t *testing.T) {
	result := consumeResultForError(context.Background(), "msg-1", errors.New("db offline"))

	require.Equal(t, consumer.ConsumeRetryLater, result)
}

func TestMessageConsumerReturnsTransientInsertError(t *testing.T) {
	insertErr := errors.New("db offline")
	notifications := &fakeNotificationModel{insertErr: insertErr}
	consumer := NewMessageConsumer(&svc.ServiceContext{NotificationModel: notifications})

	err := consumer.Consume(context.Background(), []byte(`{"target_user_id":9,"action_type":1,"user_id":7,"username":"小白","target_id":99}`))

	require.ErrorIs(t, err, insertErr)
	require.False(t, IsPermanentEventError(err))
}
```

Because these tests reference `consumer.ConsumeSuccess`, add this import:

```go
"github.com/apache/rocketmq-client-go/v2/consumer"
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
GOCACHE=/tmp/go-build-cache go test ./app/message/internal/mqs -run 'TestMessageConsumerClassifies|TestConsumeResultForError|TestMessageConsumerReturnsTransient' -count=1
```

Expected: FAIL to compile because `IsPermanentEventError`, `newPermanentEventError`, and `consumeResultForError` do not exist.

- [ ] **Step 3: Implement permanent event error helpers and handler split**

Modify `app/message/internal/mqs/message_consumer.go`.

Add `errors` to imports.

Add this near the constants:

```go
var ErrPermanentEvent = errors.New("permanent message event error")

func newPermanentEventError(message string) error {
	return fmt.Errorf("%w: %s", ErrPermanentEvent, message)
}

func IsPermanentEventError(err error) bool {
	return errors.Is(err, ErrPermanentEvent)
}

func consumeResultForError(ctx context.Context, msgID string, err error) consumer.ConsumeResult {
	if IsPermanentEventError(err) {
		logx.WithContext(ctx).Errorw("skip permanent message notification event", logx.Field("msg_id", msgID), logx.Field("err", err.Error()))
		return consumer.ConsumeSuccess
	}
	logx.WithContext(ctx).Errorw("consume message notification event failed", logx.Field("msg_id", msgID), logx.Field("err", err.Error()))
	return consumer.ConsumeRetryLater
}
```

Replace the handler error branch in `NewRocketMQConsumer` with:

```go
			if err := messageConsumer.Consume(ctx, msg.Body); err != nil {
				return consumeResultForError(ctx, msg.MsgId, err), nil
			}
```

Replace the first half of `Consume` with:

```go
func (c *MessageConsumer) Consume(ctx context.Context, body []byte) error {
	var event UserActionEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return fmt.Errorf("%w: unmarshal user action event: %v", ErrPermanentEvent, err)
	}
	if event.TargetUserID <= 0 {
		return newPermanentEventError("missing target_user_id")
	}
	if event.ActionType <= 0 {
		return newPermanentEventError("missing action_type")
	}
	title, content := RenderNotificationContent(event)
	if title == "" {
		return newPermanentEventError("unsupported action_type")
	}
	if strings.TrimSpace(content) == "" {
		return newPermanentEventError("empty notification content")
	}
```

Keep the insert/cache-invalidating code after this block unchanged.

- [ ] **Step 4: Run MQ tests to verify they pass**

Run:

```bash
GOCACHE=/tmp/go-build-cache go test ./app/message/internal/mqs -run 'TestMessageConsumer|TestConsumeResultForError' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit code files only**

Run:

```bash
git add app/message/internal/mqs/message_consumer.go app/message/internal/mqs/message_consumer_test.go
git commit -m "fix(message): skip permanent notification events"
```

Do not add files under `docs/superpowers/`.

---

### Task 5: Add SQL Indexes for Current Read Paths

**Files:**
- Modify: `deploy/sql/xbh_message.sql`

- [ ] **Step 1: Update message-table indexes**

In `deploy/sql/xbh_message.sql`, replace the message table index block:

```sql
    KEY `idx_conversation_id` (`conversation_id`),
    KEY `idx_sender_id` (`sender_id`),
    KEY `idx_receiver_id` (`receiver_id`),
    KEY `idx_created_at` (`created_at`)
```

with:

```sql
    KEY `idx_conversation_id` (`conversation_id`),
    KEY `idx_sender_receiver_id` (`sender_id`, `receiver_id`, `id`),
    KEY `idx_receiver_sender_id` (`receiver_id`, `sender_id`, `id`),
    KEY `idx_receiver_status_sender` (`receiver_id`, `status`, `sender_id`),
    KEY `idx_created_at` (`created_at`)
```

- [ ] **Step 2: Update notification-table indexes**

In `deploy/sql/xbh_message.sql`, replace the notification table index block:

```sql
    KEY `idx_user_id` (`user_id`),
    KEY `idx_type` (`type`),
    KEY `idx_status` (`status`),
    KEY `idx_created_at` (`created_at`)
```

with:

```sql
    KEY `idx_user_id` (`user_id`),
    KEY `idx_user_type_id` (`user_id`, `type`, `id`),
    KEY `idx_user_status` (`user_id`, `status`),
    KEY `idx_type` (`type`),
    KEY `idx_status` (`status`),
    KEY `idx_created_at` (`created_at`)
```

- [ ] **Step 3: Verify the SQL script through the model integration tests**

Run:

```bash
GOCACHE=/tmp/go-build-cache go test -tags=integration ./app/message/internal/model -run TestMessageCommandModel -count=1
```

Expected: PASS. This confirms the modified DDL can initialize MySQL and still supports command-model behavior.

- [ ] **Step 4: Commit SQL file only**

Run:

```bash
git add deploy/sql/xbh_message.sql
git commit -m "fix(message): add message read-path indexes"
```

Do not add files under `docs/superpowers/`.

---

### Task 6: Final Verification

**Files:**
- No new edits expected.

- [ ] **Step 1: Format changed Go files**

Run:

```bash
gofmt -w app/message/internal/model/message_command_model.go app/message/internal/model/message_command_model_integration_test.go app/message/internal/svc/service_context.go app/message/internal/logic/validation.go app/message/internal/logic/send_message_logic.go app/message/internal/logic/mark_read_logic.go app/message/internal/logic/get_messages_logic.go app/message/internal/logic/send_notification_logic.go app/message/internal/logic/message_logic_test.go app/message/internal/mqs/message_consumer.go app/message/internal/mqs/message_consumer_test.go
```

Expected: exit code 0.

- [ ] **Step 2: Run message unit tests**

Run:

```bash
GOCACHE=/tmp/go-build-cache go test ./app/message/...
```

Expected: PASS for all `app/message` packages.

- [ ] **Step 3: Run race tests for packages with unit tests**

Run:

```bash
GOCACHE=/tmp/go-build-cache go test -race ./app/message/internal/logic ./app/message/internal/mqs
```

Expected: PASS for `logic` and `mqs`.

- [ ] **Step 4: Run targeted MySQL integration tests**

Run:

```bash
GOCACHE=/tmp/go-build-cache go test -tags=integration ./app/message/internal/model -run TestMessageCommandModel -count=1
```

Expected: PASS for all `TestMessageCommandModel...` tests. If Docker is unavailable, report the Docker error and do not claim integration coverage passed.

- [ ] **Step 5: Run vet**

Run:

```bash
GOCACHE=/tmp/go-build-cache go vet ./app/message/...
```

Expected: exit code 0.

- [ ] **Step 6: Run scoped golangci-lint**

Run:

```bash
GOCACHE=/tmp/go-build-cache golangci-lint run ./app/message/...
```

Expected: `0 issues.` If snap sandboxing fails with `snap-confine`, rerun the same command with approval outside the sandbox.

- [ ] **Step 7: Inspect git status**

Run:

```bash
git status --short
```

Expected: only intentional code/test/SQL changes if not committed during execution. `docs/superpowers/specs/*` and `docs/superpowers/plans/*` must not be staged or committed.
