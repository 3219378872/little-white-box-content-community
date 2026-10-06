package svc

import (
	"esx/app/recommend/mq/internal/config"
	"esx/app/recommend/mq/internal/store"
	"esx/app/user/rpc/userservice"

	"esx/pkg/rpcx"

	"esx/pkg/logging"
	redis "esx/pkg/redisstore"
)

type ServiceContext struct {
	Config         config.Config
	BehaviorStore  store.BehaviorStore
	CandidateStore store.CandidateStore
	DeadLetters    store.DeadLetterRecorder
}

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
