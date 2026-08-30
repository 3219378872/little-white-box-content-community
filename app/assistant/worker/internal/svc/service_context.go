package svc

import (
	"context"
	"fmt"
	"strings"
	"time"

	"esx/app/assistant/internal/index"
	"esx/app/assistant/internal/lease"
	"esx/app/assistant/internal/llm"
	"esx/app/assistant/internal/memory"
	"esx/app/assistant/internal/runtime"
	"esx/app/assistant/internal/safety"
	"esx/app/assistant/internal/store"
	"esx/app/assistant/internal/tool"
	"esx/app/assistant/internal/websearch"
	"esx/app/assistant/watch"
	"esx/app/assistant/worker/internal/config"
	"esx/app/content/rpc/contentservice"
	"esx/app/interaction/rpc/interactionservice"
	"esx/app/media/rpc/mediaservice"
	"esx/app/recommend/rpc/recommendservice"
	"esx/app/search/rpc/searchservice"
	"esx/app/user/rpc/userservice"
	"esx/pkg/interceptor"

	"github.com/zeromicro/go-zero/core/stores/redis"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
	"github.com/zeromicro/go-zero/zrpc"
)

type ServiceContext struct {
	Config  config.Config
	Store   store.Store
	Memory  memory.Store
	Watch   watch.Store
	Lease   *lease.Manager
	Engine  *runtime.Engine
	Index   *index.Client
	LLM     llm.Client
	Consent runtime.ConsentChecker
}

func NewServiceContext(c config.Config) (*ServiceContext, error) {
	if strings.TrimSpace(c.DataSource) == "" {
		return nil, fmt.Errorf("assistant-agent: DataSource is required")
	}
	if strings.TrimSpace(c.InternalSecret) == "" {
		return nil, fmt.Errorf("assistant-agent: InternalSecret is required")
	}
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

	conn := sqlx.NewMysql(c.DataSource)
	st := store.NewSQLStore(conn)
	var safetyFilter safety.Filter
	if c.Safety.Enabled {
		filter, err := safety.NewKeywordFilter(c.Safety.BlockedTerms, c.Safety.MaxScanRunes)
		if err != nil {
			return nil, err
		}
		safetyFilter = filter
	}
	mem := memory.NewSQLStore(conn, safetyFilter)
	watchStore := watch.NewSQLStore(conn)
	redisClient := redis.MustNewRedis(c.Redis.RedisConf)
	notify := store.NewRedisNotifier(redisClient)

	client, err := llm.New(llm.Config{
		Enabled: c.LLM.Enabled, WireAPI: c.LLM.WireAPI, Endpoint: c.LLM.Endpoint, APIKey: c.LLM.APIKey,
		Model: c.LLM.Model, Timeout: time.Duration(c.LLM.TimeoutMs) * time.Millisecond,
		MaxOutputTokens: c.LLM.MaxOutputTokens, ContextWindowTokens: c.LLM.ContextWindowTokens,
		PromptCostPerMillionTokens: c.LLM.PromptCostPerMillionTokens, CompletionCostPerMillionTokens: c.LLM.CompletionCostPerMillionTokens,
	})
	if err != nil {
		return nil, err
	}
	if err := llm.Ready(client, c.LLM.Enabled); err != nil {
		return nil, err
	}
	history, err := index.New(c.Elasticsearch.Addresses, c.Elasticsearch.Username, c.Elasticsearch.Password, st)
	if err != nil {
		return nil, err
	}
	var historyTool tool.History
	if history != nil {
		historyTool = history
	}
	registry, err := tool.NewRegistry(tool.Clients{
		Search: searchService, Content: contentService, Media: mediaService, Recommend: recommendService,
		Interaction: interactionService, User: userService, Memory: mem, Watch: watchStore, Store: st, History: historyTool,
		Web: websearch.New(websearch.Config{
			APIKey: c.WebSearch.APIKey, Endpoint: c.WebSearch.Endpoint,
			Timeout: time.Duration(c.WebSearch.TimeoutMs) * time.Millisecond, MaxResults: c.WebSearch.MaxResults,
		}),
	}, c.AllowedTools)
	if err != nil {
		return nil, err
	}
	engine := &runtime.Engine{
		Store: st, Memory: mem, Watch: watchStore, Tools: registry, LLM: client, Notify: notify,
		Window: c.LLM.ContextWindowTokens, Provider: c.LLM.MaxOutputTokens,
	}
	return &ServiceContext{
		Config: c, Store: st, Memory: mem, Watch: watchStore,
		Lease:  &lease.Manager{Store: st, Owner: c.Name, Lease: time.Duration(c.LeaseSeconds) * time.Second, Renew: time.Duration(c.RenewSeconds) * time.Second},
		Engine: engine, Index: history, LLM: client,
		Consent: func(ctx context.Context, userID int64) (bool, error) {
			consent, err := userService.GetAgentCapabilityConsent(ctx, &userservice.GetAgentCapabilityConsentReq{UserId: userID})
			if err != nil {
				return false, err
			}
			return currentConsentGranted(consent), nil
		},
	}, nil
}

func currentConsentGranted(consent *userservice.GetAgentCapabilityConsentResp) bool {
	if consent == nil || !consent.Granted {
		return false
	}
	requiredVersion := consent.CurrentVersion
	if requiredVersion < tool.CurrentConsentVersion {
		requiredVersion = tool.CurrentConsentVersion
	}
	return consent.ConsentVersion >= requiredVersion
}
