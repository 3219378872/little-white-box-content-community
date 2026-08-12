//go:build integration

package model

import (
	"context"
	"database/sql"
	"esx/pkg/testutil"
	"os"
	"sync"
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

	scriptPath := testutil.SchemaPath("xbh_message.sql")

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
	result, err := command.CreateMessageWithConversations(context.Background(), 1, 2, "hello", 1, 0, "message-1")
	require.NoError(t, err)
	require.True(t, result.Created)
	require.Positive(t, result.MessageID)

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
		"select conversation_id from message where id = ?", result.MessageID))
	require.NoError(t, conn.QueryRowCtx(context.Background(), &storedContent,
		"select content from message where id = ?", result.MessageID))
	require.Equal(t, receiverConversationID, storedConversationID)
	require.Equal(t, "hello", storedContent)
}

func TestMessageCommandModelCreateMessageIsIdempotentAndRejectsConflicts(t *testing.T) {
	conn, cleanup := newMessageTestDB(t)
	defer cleanup()

	command := NewMessageCommandModel(conn)
	first, err := command.CreateMessageWithConversations(context.Background(), 1, 2, "hello", 1, 0, "message-1")
	require.NoError(t, err)
	require.True(t, first.Created)

	replay, err := command.CreateMessageWithConversations(context.Background(), 1, 2, "hello", 1, 0, "message-1")
	require.NoError(t, err)
	require.False(t, replay.Created)
	require.Equal(t, first.MessageID, replay.MessageID)
	require.Equal(t, int64(1), countMessageRows(t, conn, "select count(*) from message"))

	var unread int64
	require.NoError(t, conn.QueryRowCtx(context.Background(), &unread,
		"select unread_count from conversation where user_id = ? and target_user_id = ?", 2, 1))
	require.Equal(t, int64(1), unread)

	_, err = command.CreateMessageWithConversations(context.Background(), 1, 2, "different", 1, 0, "message-1")
	require.Error(t, err)
	require.True(t, IsIdempotencyConflict(err))
	require.Equal(t, int64(1), countMessageRows(t, conn, "select count(*) from message"))

	// CORE-042：同一幂等键绑定不同媒体（media_id 不同）必须冲突，且不得静默返回原消息。
	// 使用独立的收发对，避免影响后续会话计数断言。
	_, err = command.CreateMessageWithConversations(context.Background(), 3, 4, "img", 2, 55, "message-media")
	require.NoError(t, err)
	mediaReplay, err := command.CreateMessageWithConversations(context.Background(), 3, 4, "img", 2, 55, "message-media")
	require.NoError(t, err)
	require.False(t, mediaReplay.Created)
	_, err = command.CreateMessageWithConversations(context.Background(), 3, 4, "img", 2, 99, "message-media")
	require.Error(t, err)
	require.True(t, IsIdempotencyConflict(err), "same key with a different media_id must conflict")
	require.Equal(t, int64(2), countMessageRows(t, conn, "select count(*) from message"))

	var storedMediaID int64
	require.NoError(t, conn.QueryRowCtx(context.Background(), &storedMediaID,
		"select media_id from message where idempotency_key = ?", "message-media"))
	require.Equal(t, int64(55), storedMediaID, "media message must persist its media_id (CORE-041)")

	var lastMessage string
	require.NoError(t, conn.QueryRowCtx(context.Background(), &lastMessage,
		"select last_message from conversation where user_id = ? and target_user_id = ?", 2, 1))
	require.Equal(t, "hello", lastMessage)
	require.NoError(t, conn.QueryRowCtx(context.Background(), &unread,
		"select unread_count from conversation where user_id = ? and target_user_id = ?", 2, 1))
	require.Equal(t, int64(1), unread)

	results := make([]MessageCommandResult, 2)
	errs := make([]error, 2)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			results[index], errs[index] = command.CreateMessageWithConversations(
				context.Background(), 1, 2, "concurrent", 1, 0, "message-concurrent",
			)
		}(i)
	}
	close(start)
	wg.Wait()

	require.NoError(t, errs[0])
	require.NoError(t, errs[1])
	require.Equal(t, results[0].MessageID, results[1].MessageID)
	require.NotEqual(t, results[0].Created, results[1].Created)
	require.Equal(t, int64(1), countMessageRows(t, conn,
		"select count(*) from message where sender_id = ? and idempotency_key = ?", 1, "message-concurrent"))
	require.NoError(t, conn.QueryRowCtx(context.Background(), &unread,
		"select unread_count from conversation where user_id = ? and target_user_id = ?", 2, 1))
	require.Equal(t, int64(2), unread)
}

func TestMessageCommandModelCreateMessageRollsBackWhenMessageInsertFails(t *testing.T) {
	conn, cleanup := newMessageTestDB(t)
	defer cleanup()

	command := NewMessageCommandModel(conn)
	_, err := command.CreateMessageWithConversations(context.Background(), 1, 2, "hello", 256, 0, "message-1")
	require.Error(t, err)

	require.Equal(t, int64(0), countMessageRows(t, conn, "select count(*) from conversation"))
	require.Equal(t, int64(0), countMessageRows(t, conn, "select count(*) from message"))
}

func TestMessageCommandModelMarkConversationReadDecrementsUnreadByAffectedRows(t *testing.T) {
	conn, cleanup := newMessageTestDB(t)
	defer cleanup()

	command := NewMessageCommandModel(conn)
	_, err := command.CreateMessageWithConversations(context.Background(), 8, 7, "first", 1, 0, "message-1")
	require.NoError(t, err)
	_, err = command.CreateMessageWithConversations(context.Background(), 8, 7, "second", 1, 0, "message-2")
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
