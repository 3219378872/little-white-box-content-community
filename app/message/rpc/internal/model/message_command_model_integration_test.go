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
