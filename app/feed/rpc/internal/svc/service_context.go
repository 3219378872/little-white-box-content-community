package svc

import (
	"context"
	"time"

	"github.com/cloudwego/kitex/client/callopt"

	"esx/app/content/rpc/contentservice"
	"esx/app/feed/rpc/internal/config"
	"esx/app/feed/rpc/internal/model"
	"esx/app/recommend/feedback"
	"esx/app/recommend/rpc/recommendservice"
	"esx/app/user/rpc/userservice"

	redis "esx/pkg/redisstore"
	"esx/pkg/rpcx"
	sqlx "esx/pkg/sqlstore"
)

// InboxModel 是关注流读写 inbox 的操作。
type InboxModel interface {
	BatchInsertIgnore(ctx context.Context, rows []*model.FeedInbox) (int64, error)
	FindByUserBefore(ctx context.Context, userID, cursorCreatedAt, cursorPostID int64, limit int64) ([]*model.FeedInbox, error)
}

// OutboxModel 是关注流读写 outbox 的操作。
type OutboxModel interface {
	InsertIgnore(ctx context.Context, row *model.FeedOutbox) error
	FindByAuthorsBefore(ctx context.Context, authorIDs []int64, cursorCreatedAt, cursorPostID int64, limit int64) ([]*model.FeedOutbox, error)
}

// UserService 是 Feed 用到的用户服务子集：粉丝与关注列表。
type UserService interface {
	GetUser(ctx context.Context, in *userservice.GetUserReq, opts ...callopt.Option) (*userservice.GetUserResp, error)
	GetFollowers(ctx context.Context, in *userservice.GetFollowersReq, opts ...callopt.Option) (*userservice.GetFollowersResp, error)
	GetFollowing(ctx context.Context, in *userservice.GetFollowingReq, opts ...callopt.Option) (*userservice.GetFollowingResp, error)
}

// ContentService 是 Feed 用到的内容服务子集：兜底候选与帖子详情补全。
type ContentService interface {
	GetPostList(ctx context.Context, in *contentservice.GetPostListReq, opts ...callopt.Option) (*contentservice.GetPostListResp, error)
	GetPostsByIds(ctx context.Context, in *contentservice.GetPostsByIdsReq, opts ...callopt.Option) (*contentservice.GetPostsByIdsResp, error)
}

// RecommendService 是推荐流的上游。
type RecommendService interface {
	GetRecommendPosts(ctx context.Context, in *recommendservice.GetRecommendPostsReq, opts ...callopt.Option) (*recommendservice.GetRecommendPostsResp, error)
}

// NegativeFeedback 返回用户标记为不感兴趣的帖子，兜底流也必须排除它们。
type NegativeFeedback interface {
	HiddenPosts(context.Context, int64) (map[int64]struct{}, error)
}

// ServiceContext 持有 Feed RPC 的存储与下游依赖；Now 可在测试中替换以控制游标过期。
type ServiceContext struct {
	Config           config.Config
	Conn             sqlx.SqlConn
	Redis            *redis.Redis
	InboxModel       InboxModel
	OutboxModel      OutboxModel
	UserService      UserService
	ContentService   ContentService
	RecommendService RecommendService
	FallbackStates   model.FallbackStateStore
	NegativeFeedback NegativeFeedback
	Now              func() time.Time
	BigVThreshold    int64
	FanoutBatchSize  int64
}

// NewServiceContext 装配 MySQL、Redis 与带内部鉴权的下游客户端。
func NewServiceContext(c config.Config) *ServiceContext {
	conn := sqlx.NewMysql(c.DataSource)
	rds := redis.MustNewRedis(c.Redis.RedisConf)

	internalAuthOption := rpcx.WithInternalAuth(c.InternalSecret)
	userClient := rpcx.MustNewClient(c.UserRpc, internalAuthOption)
	contentClient := rpcx.MustNewClient(c.ContentRpc, internalAuthOption)
	recommendClient := rpcx.MustNewClient(c.RecommendRpc, internalAuthOption)

	return &ServiceContext{
		Config:           c,
		Conn:             conn,
		Redis:            rds,
		InboxModel:       model.NewFeedInboxModel(conn),
		OutboxModel:      model.NewFeedOutboxModel(conn),
		UserService:      userservice.NewUserService(userClient),
		ContentService:   contentservice.NewContentService(contentClient),
		RecommendService: recommendservice.NewRecommendService(recommendClient),
		FallbackStates:   model.NewFallbackStateStore(rds),
		NegativeFeedback: feedback.NewHiddenPostReader(rds, c.FeatureVersion),
		Now:              time.Now,
		BigVThreshold:    c.BigVThreshold,
		FanoutBatchSize:  c.FanoutBatchSize,
	}
}
