package svc

import (
	"esx/app/recommend/mq/internal/config"
	"esx/app/recommend/mq/internal/store"
	"esx/app/user/rpc/userservice"

	"esx/pkg/rpcx"

	"esx/pkg/logging"
	redis "esx/pkg/redisstore"
)

// ServiceContext 持有推荐特征消费者的特征、候选与死信存储。
type ServiceContext struct {
	Config         config.Config
	BehaviorStore  store.BehaviorStore
	CandidateStore store.CandidateStore
	DeadLetters    store.DeadLetterRecorder
}

// NewServiceContext 装配用户服务（个性化偏好）与 Redis 存储；配置无效时终止启动。
func NewServiceContext(c config.Config) *ServiceContext {
	logging.Must(c.Validate())
	userClient := rpcx.MustNewClient(c.UserRpc,

		rpcx.WithInternalAuth(c.InternalSecret))
	userService := userservice.NewUserService(userClient)
	redisClient := redis.MustNewRedis(c.Redis)
	candidates := store.NewRedisCandidateStore(
		redisClient, c.FeatureVersion, c.RecallKeyPrefix, c.CandidateTTL, userService,
	)
	return &ServiceContext{
		Config: c,
		BehaviorStore: store.NewRedisBehaviorStore(
			redisClient, c.FeatureVersion, c.RecallKeyPrefix, c.FeatureTTL, userService,
		),
		CandidateStore: candidates,
		DeadLetters: store.NewRedisDeadLetterRecorder(
			redisClient, c.RecallKeyPrefix, c.FeatureVersion,
			c.DeadLetterTTL, c.DeadLetterMaxLength,
		),
	}
}
