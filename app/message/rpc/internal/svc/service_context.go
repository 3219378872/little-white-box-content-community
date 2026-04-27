package svc

import (
	"context"
	"database/sql"
	"esx/app/message/rpc/internal/config"
	model2 "esx/app/message/rpc/internal/model"

	"github.com/zeromicro/go-zero/core/stores/cache"
	"github.com/zeromicro/go-zero/core/stores/redis"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

type ConversationModel interface {
	UpsertPairForMessage(ctx context.Context, senderID int64, receiverID int64, content string) (int64, int64, error)
	FindByUser(ctx context.Context, userID int64, page int64, pageSize int64) ([]*model2.Conversation, int64, error)
	FindOneForUser(ctx context.Context, userID int64, conversationID int64) (*model2.Conversation, error)
}

type MessageModel interface {
	Insert(ctx context.Context, data *model2.Message) (sql.Result, error)
	FindByUserConversation(ctx context.Context, userID int64, targetUserID int64, lastID int64, limit int64) ([]*model2.Message, bool, error)
	CountUnreadByUser(ctx context.Context, userID int64) (int64, error)
	MarkConversationReadForUser(ctx context.Context, userID int64, targetUserID int64) (int64, error)
}

type MessageCommandModel interface {
	CreateMessageWithConversations(ctx context.Context, senderID int64, receiverID int64, content string, msgType int64) (int64, error)
	MarkConversationRead(ctx context.Context, userID int64, targetUserID int64) (int64, error)
}

type NotificationModel interface {
	Insert(ctx context.Context, data *model2.Notification) (sql.Result, error)
	FindByUser(ctx context.Context, userID int64, typ int64, page int64, pageSize int64) ([]*model2.Notification, int64, error)
	CountUnread(ctx context.Context, userID int64) (int64, error)
	MarkAllRead(ctx context.Context, userID int64) (int64, error)
}

type ServiceContext struct {
	Config              config.Config
	Conn                sqlx.SqlConn
	Redis               *redis.Redis
	ConversationModel   ConversationModel
	MessageModel        MessageModel
	MessageCommandModel MessageCommandModel
	NotificationModel   NotificationModel
	UnreadStore         UnreadStore
}

func NewServiceContext(c config.Config) *ServiceContext {
	conn := sqlx.NewMysql(c.DataSource)
	cacheConf := cache.CacheConf{
		cache.NodeConf{RedisConf: c.Redis, Weight: 100},
	}
	redisClient := redis.MustNewRedis(c.Redis)

	return &ServiceContext{
		Config:              c,
		Conn:                conn,
		Redis:               redisClient,
		ConversationModel:   model2.NewConversationModel(conn, cacheConf),
		MessageModel:        model2.NewMessageModel(conn, cacheConf),
		MessageCommandModel: model2.NewMessageCommandModel(conn),
		NotificationModel:   model2.NewNotificationModel(conn, cacheConf),
		UnreadStore:         NewRedisUnreadStore(redisClient),
	}
}
