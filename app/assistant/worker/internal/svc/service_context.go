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

	// SQL arguments may contain prompts and tool payloads. Metrics remain enabled,
	// but statement logging is disabled for this worker process.
	sqlx.DisableLog()
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

	client, routeIDs, err := buildLLMClient(c.LLM)
	if err != nil {
		return nil, err
	}
	if err := llm.Ready(client, c.LLM.Enabled); err != nil {
		return nil, err
	}
	auxClient, err := buildAuxiliaryClient(c.LLM, c.LLM.AuxModel, "aux")
	if err != nil {
		return nil, err
	}
	reviewClient, err := buildAuxiliaryClient(c.LLM, c.BackgroundReview.Model, "review")
	if err != nil {
		return nil, err
	}
	if c.LLM.Enabled && c.LLM.CanaryEnabled {
		canaryTimeout := minDuration(time.Duration(c.LLM.TimeoutMs)*time.Millisecond, 30*time.Second)
		for _, routeID := range routeIDs {
			routeClient, ok := llm.SelectExactRoute(client, routeID)
			if !ok {
				return nil, fmt.Errorf("assistant-agent: LLM route %q cannot be selected", routeID)
			}
			if err := runLLMCanary(canaryTimeout, routeClient); err != nil {
				return nil, fmt.Errorf("assistant-agent: LLM route %q readiness: %w", routeID, err)
			}
		}
		for _, auxiliaryRoute := range []struct {
			name   string
			client llm.Client
		}{{name: "aux", client: auxClient}, {name: "review", client: reviewClient}} {
			routeID, auxiliary := auxiliaryRoute.name, auxiliaryRoute.client
			if auxiliary != nil {
				if err := runLLMCanary(canaryTimeout, auxiliary); err != nil {
					return nil, fmt.Errorf("assistant-agent: LLM %s readiness: %w", routeID, err)
				}
			}
		}
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
		Store: st, Memory: mem, Watch: watchStore, Tools: registry, LLM: client, AuxLLM: auxClient, ReviewLLM: reviewClient, Notify: notify,
		Window: c.LLM.ContextWindowTokens, Provider: c.LLM.MaxOutputTokens,
	}
	return &ServiceContext{
		Config: c, Store: st, Memory: mem, Watch: watchStore,
		Lease:  &lease.Manager{Store: st, Owner: lease.NewOwner(c.Name), Lease: time.Duration(c.LeaseSeconds) * time.Second, Renew: time.Duration(c.RenewSeconds) * time.Second},
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

func buildLLMClient(c config.LLMConfig) (llm.Client, []string, error) {
	primary, err := llm.New(primaryLLMConfig(c, c.Model, c.RouteID))
	if err != nil || primary == nil {
		return primary, nil, err
	}
	routes := []llm.Route{{ID: primary.RouteID(), Boundary: c.Boundary, Client: primary}}
	routeIDs := []string{primary.RouteID()}
	for _, fallback := range c.Fallbacks {
		if !fallback.Enabled {
			continue
		}
		candidate, candidateErr := llm.New(llm.Config{
			Enabled: true, RouteID: fallback.RouteID, Boundary: fallback.Boundary,
			WireAPI: fallback.WireAPI, Endpoint: fallback.Endpoint, APIKey: fallback.APIKey, Model: fallback.Model,
			Timeout:         time.Duration(fallback.TimeoutMs) * time.Millisecond,
			MaxOutputTokens: fallback.MaxOutputTokens, ContextWindowTokens: fallback.ContextWindowTokens,
			PromptCostPerMillionTokens:     fallback.PromptCostPerMillionTokens,
			CompletionCostPerMillionTokens: fallback.CompletionCostPerMillionTokens,
			CacheReadCostPerMillionTokens:  fallback.CacheReadCostPerMillionTokens,
			CacheWriteCostPerMillionTokens: fallback.CacheWriteCostPerMillionTokens,
			ReasoningCostPerMillionTokens:  fallback.ReasoningCostPerMillionTokens,
		})
		if candidateErr != nil {
			return nil, nil, candidateErr
		}
		routes = append(routes, llm.Route{ID: candidate.RouteID(), Boundary: fallback.Boundary, Client: candidate})
		routeIDs = append(routeIDs, candidate.RouteID())
	}
	resilient, err := llm.NewResilient(routes, llm.RetryOptions{
		MaxAttempts: c.RetryMaxAttempts, BaseDelay: time.Duration(c.RetryBaseDelayMs) * time.Millisecond,
		MaxDelay: time.Duration(c.RetryMaxDelayMs) * time.Millisecond, MaxRetryAfter: time.Duration(c.RetryAfterMaxMs) * time.Millisecond,
	})
	return resilient, routeIDs, err
}

func buildAuxiliaryClient(c config.LLMConfig, model, suffix string) (llm.Client, error) {
	model = strings.TrimSpace(model)
	if !c.Enabled || model == "" {
		return nil, nil
	}
	routeID := strings.TrimSpace(c.RouteID)
	if routeID == "" {
		routeID = "primary"
	}
	httpClient, err := llm.New(primaryLLMConfig(c, model, routeID+"-"+suffix))
	if err != nil {
		return nil, err
	}
	return llm.NewResilient([]llm.Route{{ID: httpClient.RouteID(), Boundary: c.Boundary, Client: httpClient}}, llm.RetryOptions{
		MaxAttempts: c.RetryMaxAttempts, BaseDelay: time.Duration(c.RetryBaseDelayMs) * time.Millisecond,
		MaxDelay: time.Duration(c.RetryMaxDelayMs) * time.Millisecond, MaxRetryAfter: time.Duration(c.RetryAfterMaxMs) * time.Millisecond,
	})
}

func primaryLLMConfig(c config.LLMConfig, model, routeID string) llm.Config {
	return llm.Config{
		Enabled: c.Enabled, RouteID: routeID, Boundary: c.Boundary, WireAPI: c.WireAPI,
		Endpoint: c.Endpoint, APIKey: c.APIKey, Model: model, Timeout: time.Duration(c.TimeoutMs) * time.Millisecond,
		MaxOutputTokens: c.MaxOutputTokens, ContextWindowTokens: c.ContextWindowTokens,
		PromptCostPerMillionTokens:     c.PromptCostPerMillionTokens,
		CompletionCostPerMillionTokens: c.CompletionCostPerMillionTokens,
		CacheReadCostPerMillionTokens:  c.CacheReadCostPerMillionTokens,
		CacheWriteCostPerMillionTokens: c.CacheWriteCostPerMillionTokens,
		ReasoningCostPerMillionTokens:  c.ReasoningCostPerMillionTokens,
	}
}

func minDuration(a, b time.Duration) time.Duration {
	if a > 0 && a < b {
		return a
	}
	return b
}

func runLLMCanary(timeout time.Duration, client llm.Client) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return llm.Canary(ctx, client)
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
