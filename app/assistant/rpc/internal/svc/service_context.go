package svc

import (
	"context"
	"strings"
	"time"

	"esx/app/assistant/rpc/internal/agent"
	"esx/app/assistant/rpc/internal/config"
	"esx/app/assistant/rpc/internal/llm"
	"esx/app/assistant/rpc/internal/memory"
	"esx/app/assistant/rpc/internal/safety"
	"esx/app/assistant/rpc/internal/store"
	"esx/app/assistant/rpc/internal/tool"
	"esx/app/assistant/rpc/internal/websearch"
	"esx/app/assistant/watch"
	"esx/app/content/rpc/contentservice"
	"esx/app/interaction/rpc/interactionservice"
	"esx/app/media/rpc/mediaservice"
	"esx/app/recommend/rpc/recommendservice"
	"esx/app/search/rpc/searchservice"
	"esx/app/user/rpc/userservice"
	"esx/pkg/interceptor"

	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/stores/redis"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
	"github.com/zeromicro/go-zero/zrpc"
)

type ServiceContext struct {
	Config             config.Config
	Tools              tool.Executor
	Conversations      store.ConversationStore
	Quota              store.QuotaLimiter
	Generator          llm.Generator
	Safety             safety.Filter
	ContentService     contentservice.ContentService
	SearchService      searchservice.SearchService
	InteractionService interactionservice.InteractionService

	// Agent 模式装配（SPEC-assistant-agent-mode）；AgentRunner 为 nil 表示未启用。
	AgentRunner   agent.Runner
	AgentTools    *agent.ToolRegistry
	AgentConfirms agent.ConfirmBroker
	AgentQuota    store.QuotaLimiter
	UserService   userservice.UserService
	Memory        memory.Store
	Watch         watch.Store
	Audit         agent.AuditStore
}

func NewServiceContext(c config.Config) *ServiceContext {
	bizErrInterceptor := interceptor.BizErrorUnaryInterceptor()
	internalAuthInterceptor := interceptor.InternalAuthUnaryClientInterceptor(c.InternalSecret)
	newClient := func(conf zrpc.RpcClientConf) zrpc.Client {
		conf.Middlewares.Duration = false
		return zrpc.MustNewClient(conf,
			zrpc.WithUnaryClientInterceptor(bizErrInterceptor),
			zrpc.WithUnaryClientInterceptor(internalAuthInterceptor),
			zrpc.WithUnaryClientInterceptor(interceptor.SafeDurationUnaryClientInterceptor()),
		)
	}
	searchService := searchservice.NewSearchService(newClient(c.SearchRpc))
	contentService := contentservice.NewContentService(newClient(c.ContentRpc))
	recommendService := recommendservice.NewRecommendService(newClient(c.RecommendRpc))
	mediaService := mediaservice.NewMediaService(newClient(c.MediaRpc))
	interactionService := interactionservice.NewInteractionService(newClient(c.InteractionRpc))
	userService := userservice.NewUserService(newClient(c.UserRpc))

	tools, err := tool.NewRegistry(c.AllowedTools, tool.Clients{
		Search:    searchService,
		Content:   contentService,
		Recommend: recommendService,
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

	memoryStore := buildMemoryStore(c, userService)
	watchStore := buildWatchStore(c)
	auditStore := buildAuditStore(c)
	return &ServiceContext{
		Config: c, Tools: tools, Conversations: state, Quota: state,
		Generator: generator, Safety: safetyFilter,
		ContentService:     contentService,
		SearchService:      searchService,
		InteractionService: interactionService,
		Memory:             memoryStore,
		Watch:              watchStore,
		Audit:              auditStore,
		AgentTools:         buildAgentTools(c, searchService, contentService, mediaService, recommendService, interactionService, userService, memoryStore, watchStore),
		// AgentConfirms 始终可用：即使 runner 未启用，ConfirmToolCall 也应能明确拒绝过期凭据。
		AgentConfirms: agent.NewRedisConfirmBroker(redisClient, c.StateKeyPrefix),
		AgentRunner:   buildAgentRunner(c, memoryStore, watchStore, auditStore, userService, contentService),
		AgentQuota:    buildAgentQuota(redisClient, c),
		UserService:   userService,
	}
}

func buildMemoryStore(c config.Config, userService userservice.UserService) memory.Store {
	if strings.TrimSpace(c.DataSource) == "" {
		return nil
	}
	store := memory.NewSQLStore(sqlx.NewMysql(c.DataSource))
	if userService != nil {
		store.Personalization = func(ctx context.Context, userID int64) (bool, error) {
			pref, err := userService.GetPersonalizationPreference(ctx, &userservice.GetPersonalizationPreferenceReq{UserId: userID})
			if err != nil {
				return false, err
			}
			if pref == nil {
				return false, nil
			}
			return pref.Enabled, nil
		}
	}
	return store
}

func buildAuditStore(c config.Config) agent.AuditStore {
	if strings.TrimSpace(c.DataSource) == "" {
		return nil
	}
	return agent.NewSQLAuditStore(sqlx.NewMysql(c.DataSource))
}

func buildWatchStore(c config.Config) watch.Store {
	if strings.TrimSpace(c.DataSource) == "" {
		return nil
	}
	return watch.NewSQLStore(sqlx.NewMysql(c.DataSource))
}

// buildAgentTools 构造 Agent 工具注册表；失败时返回 nil（agent 模式不可用），
// 不阻断 enhanced_search 管线。
func buildAgentTools(
	c config.Config,
	searchService searchservice.SearchService,
	contentService contentservice.ContentService,
	mediaService mediaservice.MediaService,
	recommendService recommendservice.RecommendService,
	interactionService interactionservice.InteractionService,
	userService userservice.UserService,
	memoryStore memory.Store,
	watchStore watch.Store,
) *agent.ToolRegistry {
	if !c.Agent.Enabled {
		return nil
	}
	registry, err := agent.NewToolRegistry(agent.Clients{
		Search:      searchService,
		Content:     contentService,
		Media:       mediaService,
		Recommend:   recommendService,
		Interaction: interactionService,
		User:        userService,
		Memory:      memoryStore,
		Watch:       watchStore,
		Web: websearch.New(websearch.Config{
			APIKey:     c.Agent.WebSearch.APIKey,
			Endpoint:   c.Agent.WebSearch.Endpoint,
			Timeout:    time.Duration(c.Agent.WebSearch.TimeoutMs) * time.Millisecond,
			MaxResults: c.Agent.WebSearch.MaxResults,
		}),
	}, c.Agent.AllowedTools)
	if err != nil {
		logx.Errorw("assistant agent disabled", logx.Field("reason", err.Error()))
		return nil
	}
	return registry
}

// buildAgentRunner 装配 Agent 编排引擎；未启用或配置不完整时返回 nil 并记录原因，
// 不 panic——agent 关闭不应影响服务启动与既有管线。
func buildAgentRunner(c config.Config, memoryStore memory.Store, watchStore watch.Store, auditStore agent.AuditStore,
	userService userservice.UserService, contentService contentservice.ContentService) agent.Runner {
	if !c.Agent.Enabled || !c.LLM.Enabled || c.LLM.WireAPI == llm.WireAPIResponses {
		return nil
	}
	if len(c.Agent.AllowedTools) == 0 {
		logx.Infow("assistant agent enabled with default tool set")
	}
	runner, err := agent.NewOpenAIRunner(
		c.LLM.Endpoint, c.LLM.APIKey, c.LLM.Model,
		c.LLM.MaxContextRunes, c.LLM.MaxOutputTokens,
	)
	if err != nil {
		logx.Errorw("assistant agent disabled", logx.Field("reason", err.Error()))
		return nil
	}
	runtime := agent.NewRuntime(runner, memoryStore)
	runtime.Watch = watchStore
	runtime.Audit = auditStore
	runtime.User = userService
	runtime.Content = contentService
	runtime.Model = c.LLM.Model
	return runtime
}

// buildAgentQuota 为 Agent 模式构造独立固定窗口限流器（AGNT-032）。
func buildAgentQuota(redisClient *redis.Redis, c config.Config) store.QuotaLimiter {
	if !c.Agent.Enabled {
		return nil
	}
	state, err := store.NewRedisState(
		redisClient, c.StateKeyPrefix+":agent", c.ConversationTTLSeconds,
		c.ConversationMaxMessages, c.QuotaWindowSeconds, c.Agent.QuotaRequests,
	)
	if err != nil {
		logx.Errorw("assistant agent quota disabled", logx.Field("reason", err.Error()))
		return nil
	}
	return state
}
