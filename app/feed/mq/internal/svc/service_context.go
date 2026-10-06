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

// UserService 是 fanout 用到的用户服务子集：作者粉丝数与粉丝分页。
type UserService interface {
	GetUser(ctx context.Context, in *userservice.GetUserReq, opts ...callopt.Option) (*userservice.GetUserResp, error)
	GetFollowers(ctx context.Context, in *userservice.GetFollowersReq, opts ...callopt.Option) (*userservice.GetFollowersResp, error)
}

// ServiceContext 持有 fanout 消费者的数据库模型、用户服务与推送阈值。
type ServiceContext struct {
	Config          config.Config
	Conn            sqlx.SqlConn
	OutboxModel     model.FeedOutboxModel
	InboxModel      model.FeedInboxModel
	UserService     UserService
	BigVThreshold   int64
	FanoutBatchSize int64
}

// NewServiceContext 装配 MySQL 模型与带内部鉴权的用户服务客户端。
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

// MustServiceContext 在数据库连接不可用时终止启动。
func MustServiceContext(c config.Config) *ServiceContext {
	ctx := NewServiceContext(c)
	if ctx.Conn == nil {
		panic("feed-consumer: mysql connection failed")
	}
	return ctx
}
