package svc

import (
	"context"

	"esx/app/content/rpc/contentservice"
	"esx/app/feed/rpc/internal/config"
	"esx/app/feed/rpc/internal/model"
	"interceptor"
	"user/userservice"

	"github.com/zeromicro/go-zero/core/stores/redis"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc"
)

type InboxModel interface {
	BatchInsertIgnore(ctx context.Context, rows []*model.FeedInbox) (int64, error)
	FindByUserBefore(ctx context.Context, userID, cursorCreatedAt, cursorPostID int64, limit int64) ([]*model.FeedInbox, error)
}

type OutboxModel interface {
	InsertIgnore(ctx context.Context, row *model.FeedOutbox) error
	FindByAuthorsBefore(ctx context.Context, authorIDs []int64, cursorCreatedAt, cursorPostID int64, limit int64) ([]*model.FeedOutbox, error)
}

type UserService interface {
	GetUser(ctx context.Context, in *userservice.GetUserReq, opts ...grpc.CallOption) (*userservice.GetUserResp, error)
	GetFollowers(ctx context.Context, in *userservice.GetFollowersReq, opts ...grpc.CallOption) (*userservice.GetFollowersResp, error)
	GetFollowing(ctx context.Context, in *userservice.GetFollowingReq, opts ...grpc.CallOption) (*userservice.GetFollowingResp, error)
}

type ContentService interface {
	GetPostList(ctx context.Context, in *contentservice.GetPostListReq, opts ...grpc.CallOption) (*contentservice.GetPostListResp, error)
	GetPostsByIds(ctx context.Context, in *contentservice.GetPostsByIdsReq, opts ...grpc.CallOption) (*contentservice.GetPostsByIdsResp, error)
}

type ServiceContext struct {
	Config          config.Config
	Conn            sqlx.SqlConn
	Redis           *redis.Redis
	InboxModel      InboxModel
	OutboxModel     OutboxModel
	UserService     UserService
	ContentService  ContentService
	BigVThreshold   int64
	FanoutBatchSize int64
}

func NewServiceContext(c config.Config) *ServiceContext {
	conn := sqlx.NewMysql(c.DataSource)
	rds := redis.MustNewRedis(c.Redis)
	bizErrInterceptor := interceptor.BizErrorUnaryInterceptor()
	userClient := zrpc.MustNewClient(c.UserRpc, zrpc.WithUnaryClientInterceptor(bizErrInterceptor))
	contentClient := zrpc.MustNewClient(c.ContentRpc, zrpc.WithUnaryClientInterceptor(bizErrInterceptor))

	return &ServiceContext{
		Config:          c,
		Conn:            conn,
		Redis:           rds,
		InboxModel:      model.NewFeedInboxModel(conn),
		OutboxModel:     model.NewFeedOutboxModel(conn),
		UserService:     userservice.NewUserService(userClient),
		ContentService:  contentservice.NewContentService(contentClient),
		BigVThreshold:   c.BigVThreshold,
		FanoutBatchSize: c.FanoutBatchSize,
	}
}
