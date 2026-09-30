package svc

import (
	"context"
	"esx/app/recommend/mq/internal/config"
	"esx/app/recommend/mq/internal/store"
	"esx/app/user/rpc/userservice"

	"esx/pkg/rpcx"

	logx "esx/pkg/logging"
	redis "esx/pkg/redisstore"
)

type ServiceContext struct {
	Config                config.Config
	BehaviorStore         store.BehaviorStore
	CandidateStore        store.CandidateStore
	DeadLetters           store.DeadLetterRecorder
	UpgradeDedupRetention func(context.Context) (int, error)
}

func NewServiceContext(c config.Config) *ServiceContext {
	logx.Must(c.Validate())
	userClient := rpcx.MustNewClient(c.UserRpc,

		rpcx.WithInternalAuth(c.InternalSecret))
	userService := userservice.NewUserService(userClient)
	redisClient := redis.MustNewRedis(c.Redis)
	candidates := store.NewRedisCandidateStore(
		redisClient, c.FeatureVersion, c.RecallKeyPrefix, c.CandidateTTL, userService,
	)
	behavior := store.NewRedisBehaviorStoreWithRetention(redisClient, c.FeatureVersion, c.RecallKeyPrefix, c.FeatureTTL, c.DedupTTL, userService)
	return &ServiceContext{
		Config:        c,
		BehaviorStore: behavior,
		UpgradeDedupRetention: func(ctx context.Context) (int, error) {
			return behavior.UpgradeDedupRetention(ctx, c.LegacyDedupTTL)
		},
		CandidateStore: candidates,
		DeadLetters: store.NewRedisDeadLetterRecorder(
			redisClient, c.RecallKeyPrefix, c.FeatureVersion,
			c.DeadLetterTTL, c.DeadLetterMaxLength,
		),
	}
}
