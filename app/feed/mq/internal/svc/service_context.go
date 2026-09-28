package svc

import (
	"context"
	"esx/app/feed/mq/internal/config"
	"esx/app/feed/mq/internal/model"
	"esx/app/user/rpc/userservice"

	"github.com/cloudwego/kitex/client/callopt"

	"esx/pkg/rpcx"
	sqlx "esx/pkg/sqlstore"
)

type UserService interface {
	GetUser(ctx context.Context, in *userservice.GetUserReq, opts ...callopt.Option) (*userservice.GetUserResp, error)
	GetFollowers(ctx context.Context, in *userservice.GetFollowersReq, opts ...callopt.Option) (*userservice.GetFollowersResp, error)
}

type ServiceContext struct {
	Config          config.Config
	Conn            sqlx.SqlConn
	OutboxModel     model.FeedOutboxModel
	InboxModel      model.FeedInboxModel
	UserService     UserService
	BigVThreshold   int64
	FanoutBatchSize int64
}

func NewServiceContext(c config.Config) *ServiceContext {
	conn := sqlx.NewMysql(c.DataSource)
	userRpcClient := rpcx.MustNewClient(c.UserRpc,
		rpcx.WithInternalAuth(c.InternalSecret))
	return &ServiceContext{
		Config:          c,
		Conn:            conn,
		OutboxModel:     model.NewFeedOutboxModel(conn),
		InboxModel:      model.NewFeedInboxModel(conn),
		UserService:     userservice.NewUserService(userRpcClient),
		BigVThreshold:   c.BigVThreshold,
		FanoutBatchSize: c.FanoutBatchSize,
	}
}

func MustServiceContext(c config.Config) *ServiceContext {
	ctx := NewServiceContext(c)
	if ctx.Conn == nil {
		panic("feed-consumer: mysql connection failed")
	}
	return ctx
}
