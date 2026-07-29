package svc

import (
	"esx/app/assistant/rpc/internal/config"
	"esx/app/assistant/rpc/internal/llm"
	"esx/app/assistant/rpc/internal/safety"
	"esx/app/assistant/rpc/internal/store"
	"esx/app/assistant/rpc/internal/tool"
	"esx/app/content/rpc/contentservice"
	"esx/app/recommend/rpc/recommendservice"
	"esx/app/search/rpc/searchservice"
	"interceptor"
	"time"
	"user/userservice"

	"github.com/zeromicro/go-zero/core/stores/redis"
	"github.com/zeromicro/go-zero/zrpc"
)

type ServiceContext struct {
	Config        config.Config
	Tools         tool.Executor
	Conversations store.ConversationStore
	Quota         store.QuotaLimiter
	Generator     llm.Generator
	Safety        safety.Filter
}

func NewServiceContext(c config.Config) *ServiceContext {
	bizErrInterceptor := interceptor.BizErrorUnaryInterceptor()
	searchClient := zrpc.MustNewClient(c.SearchRpc, zrpc.WithUnaryClientInterceptor(bizErrInterceptor))
	contentClient := zrpc.MustNewClient(c.ContentRpc, zrpc.WithUnaryClientInterceptor(bizErrInterceptor))
	recommendClient := zrpc.MustNewClient(c.RecommendRpc, zrpc.WithUnaryClientInterceptor(bizErrInterceptor))
	userClient := zrpc.MustNewClient(c.UserRpc, zrpc.WithUnaryClientInterceptor(bizErrInterceptor))

	tools, err := tool.NewRegistry(c.AllowedTools, tool.Clients{
		Search:    searchservice.NewSearchService(searchClient),
		Content:   contentservice.NewContentService(contentClient),
		Recommend: recommendservice.NewRecommendService(recommendClient),
		User:      userservice.NewUserService(userClient),
	}, c.MaxSources)
	if err != nil {
		panic(err)
	}
	redisClient := redis.MustNewRedis(c.Redis.RedisConf)
	state, err := store.NewRedisState(
		redisClient, c.StateKeyPrefix, c.ConversationTTLSeconds,
		c.ConversationMaxMessages, c.QuotaWindowSeconds, c.QuotaRequests,
	)
	if err != nil {
		panic(err)
	}
	var generator llm.Generator
	if c.LLM.Enabled {
		generator, err = llm.NewOpenAICompatible(
			c.LLM.Endpoint, c.LLM.WireAPI, c.LLM.APIKey, c.LLM.Model,
			time.Duration(c.LLM.TimeoutMs)*time.Millisecond,
			c.LLM.MaxContextRunes, c.LLM.MaxOutputRunes, c.LLM.MaxOutputTokens,
			c.LLM.PromptCostPerMillionTokens, c.LLM.CompletionCostPerMillionTokens,
		)
		if err != nil {
			panic(err)
		}
	}
	var safetyFilter safety.Filter
	if c.Safety.Enabled {
		safetyFilter, err = safety.NewKeywordFilter(c.Safety.BlockedTerms, c.Safety.MaxScanRunes)
		if err != nil {
			panic(err)
		}
	}

	return &ServiceContext{
		Config: c, Tools: tools, Conversations: state, Quota: state, Generator: generator, Safety: safetyFilter,
	}
}
