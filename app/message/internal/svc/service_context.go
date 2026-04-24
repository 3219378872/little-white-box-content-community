package svc

import (
	"context"
	"database/sql"

	"esx/app/message/internal/config"
	"esx/app/message/internal/model"

	"github.com/zeromicro/go-zero/core/stores/cache"
	"github.com/zeromicro/go-zero/core/stores/redis"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

type ConversationModel interface {
	UpsertPairForMessage(ctx context.Context, senderID int64, receiverID int64, content string) (int64, int64, error)
	FindByUser(ctx context.Context, userID int64, page int64, pageSize int64) ([]*model.Conversation, int64, error)
}

type MessageModel interface {
	Insert(ctx context.Context, data *model.Message) (sql.Result, error)
	FindByConversation(ctx context.Context, conversationID int64, lastID int64, limit int64) ([]*model.Message, bool, error)
	CountUnreadByUser(ctx context.Context, userID int64) (int64, error)
	MarkConversationRead(ctx context.Context, userID int64, conversationID int64) (int64, error)
}

type NotificationModel interface {
	Insert(ctx context.Context, data *model.Notification) (sql.Result, error)
	FindByUser(ctx context.Context, userID int64, typ int64, page int64, pageSize int64) ([]*model.Notification, int64, error)
	CountUnread(ctx context.Context, userID int64) (int64, error)
	MarkAllRead(ctx context.Context, userID int64) (int64, error)
}

type ServiceContext struct {
	Config            config.Config
	Conn              sqlx.SqlConn
	Redis             *redis.Redis
	ConversationModel ConversationModel
	MessageModel      MessageModel
	NotificationModel NotificationModel
	UnreadStore       UnreadStore
}

func NewServiceContext(c config.Config) *ServiceContext {
	conn := sqlx.NewMysql(c.DataSource)
	cacheConf := cache.CacheConf{
		cache.NodeConf{RedisConf: c.Redis, Weight: 100},
	}
	redisClient := redis.MustNewRedis(c.Redis)

	return &ServiceContext{
		Config:            c,
		Conn:              conn,
		Redis:             redisClient,
		ConversationModel: model.NewConversationModel(conn, cacheConf),
		MessageModel:      model.NewMessageModel(conn, cacheConf),
		NotificationModel: model.NewNotificationModel(conn, cacheConf),
		UnreadStore:       NewRedisUnreadStore(redisClient),
	}
}
