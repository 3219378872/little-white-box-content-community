package svc

import (
	"esx/app/message/mq/internal/config"
	"esx/app/message/mq/internal/model"

	"github.com/zeromicro/go-zero/core/stores/redis"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

type ServiceContext struct {
	Config            config.Config
	Conn              sqlx.SqlConn
	NotificationModel model.NotificationModel
	UnreadStore       UnreadStore
}

func NewServiceContext(c config.Config) *ServiceContext {
	conn := sqlx.NewMysql(c.DataSource)
	var store UnreadStore
	if c.Redis.Host != "" {
		r := redis.MustNewRedis(redis.RedisConf{
			Host: c.Redis.Host,
			Pass: c.Redis.Pass,
			Type: "node",
		})
		store = NewRedisUnreadStore(r)
	}
	return &ServiceContext{
		Config:            c,
		Conn:              conn,
		NotificationModel: model.NewNotificationModel(conn),
		UnreadStore:       store,
	}
}

func MustServiceContext(c config.Config) *ServiceContext {
	ctx := NewServiceContext(c)
	if ctx.Conn == nil {
		panic("message-consumer: mysql connection failed")
	}
	return ctx
}
