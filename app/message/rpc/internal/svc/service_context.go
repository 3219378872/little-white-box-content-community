package svc

import (
	"context"
	"database/sql"
	"esx/app/media/rpc/mediaservice"
	"esx/app/message/rpc/internal/config"
	model2 "esx/app/message/rpc/internal/model"
	"esx/app/user/rpc/userservice"

	"github.com/cloudwego/kitex/client/callopt"

	cache "esx/pkg/modelcache"
	"esx/pkg/rpcx"
	sqlx "esx/pkg/sqlstore"
)

// ConversationModel lists and loads a user's conversations.
type ConversationModel interface {
	FindByUser(ctx context.Context, userID int64, page int64, pageSize int64) ([]*model2.Conversation, int64, error)
	FindOneForUser(ctx context.Context, userID int64, conversationID int64) (*model2.Conversation, error)
}

// MessageModel reads messages and unread counts.
type MessageModel interface {
	FindByUserConversation(ctx context.Context, userID int64, targetUserID int64, lastID int64, limit int64) ([]*model2.Message, bool, error)
	CountUnreadByUser(ctx context.Context, userID int64) (int64, error)
}

// MessageCommandModel performs transactional message writes.
type MessageCommandModel interface {
	CreateMessageWithConversations(ctx context.Context, senderID int64, receiverID int64, content string, msgType int64, mediaID int64, idempotencyKey string) (model2.MessageCommandResult, error)
	MarkConversationRead(ctx context.Context, userID int64, targetUserID int64) (int64, error)
}

// NotificationModel writes and reads notifications.
type NotificationModel interface {
	Insert(ctx context.Context, data *model2.Notification) (sql.Result, error)
	FindByUser(ctx context.Context, userID int64, typ int64, page int64, pageSize int64) ([]*model2.Notification, int64, error)
	CountUnread(ctx context.Context, userID int64) (int64, error)
	MarkAllRead(ctx context.Context, userID int64) (int64, error)
}

// UserService is the user RPC subset used to render conversation peers.
type UserService interface {
	BatchGetUserCards(ctx context.Context, in *userservice.BatchGetUserCardsReq, opts ...callopt.Option) (*userservice.BatchGetUserCardsResp, error)
}

// ServiceContext holds message storage and downstream services.
type ServiceContext struct {
	Config              config.Config
	Conn                sqlx.SqlConn
	ConversationModel   ConversationModel
	MessageModel        MessageModel
	MessageCommandModel MessageCommandModel
	NotificationModel   NotificationModel
	UserService         UserService
	MediaService        mediaservice.MediaService
}

// NewServiceContext wires MySQL, the Redis-backed model cache and downstream clients. The media client
// is optional: without it media messages cannot be sent.
func NewServiceContext(c config.Config) *ServiceContext {
	// pkg/sqlstore never logs SQL statements, so private message text stays out of logs.
	conn := sqlx.NewMysql(c.DataSource)
	cacheConf := cache.CacheConf{
		cache.NodeConf{RedisConf: c.Redis.RedisConf, Weight: 100},
	}
	userClient := rpcx.MustNewClient(c.UserRpc, rpcx.WithInternalAuth(c.InternalSecret))
	var mediaService mediaservice.MediaService
	if len(c.MediaRpc.Etcd.Hosts) > 0 || len(c.MediaRpc.Endpoints) > 0 || c.MediaRpc.Target != "" {
		mediaClient := rpcx.MustNewClient(c.MediaRpc,

			rpcx.WithInternalAuth(c.InternalSecret))
		mediaService = mediaservice.NewMediaService(mediaClient)
	}

	return &ServiceContext{
		Config:              c,
		Conn:                conn,
		ConversationModel:   model2.NewConversationModel(conn, cacheConf),
		MessageModel:        model2.NewMessageModel(conn, cacheConf),
		MessageCommandModel: model2.NewMessageCommandModel(conn),
		NotificationModel:   model2.NewNotificationModel(conn, cacheConf),
		UserService:         userservice.NewUserService(userClient),
		MediaService:        mediaService,
	}
}
