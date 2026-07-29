package svc

import (
	"esx/app/recommend/mq/internal/config"
	"esx/app/recommend/mq/internal/store"

	"github.com/zeromicro/go-zero/core/stores/redis"
)

type ServiceContext struct {
	Config         config.Config
	BehaviorStore  store.BehaviorStore
	CandidateStore store.CandidateStore
	DeadLetters    store.DeadLetterRecorder
}

func NewServiceContext(c config.Config) *ServiceContext {
	redisClient := redis.MustNewRedis(c.Redis)
	candidates := store.NewRedisCandidateStore(
		redisClient, c.FeatureVersion, c.RecallKeyPrefix, c.CandidateTTL,
	)
	return &ServiceContext{
		Config: c,
		BehaviorStore: store.NewRedisBehaviorStore(
			redisClient, c.FeatureVersion, c.RecallKeyPrefix, c.FeatureTTL,
		),
		CandidateStore: candidates,
		DeadLetters: store.NewRedisDeadLetterRecorder(
			redisClient, c.RecallKeyPrefix, c.FeatureVersion,
			c.DeadLetterTTL, c.DeadLetterMaxLength,
		),
	}
}
