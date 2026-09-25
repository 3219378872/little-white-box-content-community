package svc

import (
	"esx/app/recommend/mq/internal/config"
	"esx/app/recommend/mq/internal/store"
	"esx/app/user/rpc/userservice"
	"esx/pkg/interceptor"

	"github.com/zeromicro/go-zero/zrpc"

	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/stores/redis"
)

type ServiceContext struct {
	Config         config.Config
	BehaviorStore  store.BehaviorStore
	CandidateStore store.CandidateStore
	DeadLetters    store.DeadLetterRecorder
}

func NewServiceContext(c config.Config) *ServiceContext {
	logx.Must(c.Validate())
	userClient := interceptor.MustNewClient(c.UserRpc,
		zrpc.WithUnaryClientInterceptor(interceptor.BizErrorUnaryInterceptor()),
		zrpc.WithUnaryClientInterceptor(interceptor.InternalAuthUnaryClientInterceptor(c.InternalSecret)))
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
