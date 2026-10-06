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
	"esx/app/assistant/internal/retention"
	"esx/app/assistant/internal/runtime"
	"esx/app/assistant/internal/safety"
	"esx/app/assistant/internal/store"
	"esx/app/assistant/internal/tool"
	"esx/app/assistant/internal/websearch"
	"esx/app/assistant/worker/internal/config"
	"esx/app/content/rpc/contentservice"
	"esx/app/interaction/rpc/interactionservice"
	"esx/app/media/rpc/mediaservice"
	"esx/app/recommend/rpc/recommendservice"
	"esx/app/search/rpc/searchservice"
	"esx/app/user/rpc/userservice"

	redis "esx/pkg/redisstore"
	"esx/pkg/rpcx"
	sqlx "esx/pkg/sqlstore"
)

type ServiceContext struct {
	Config    config.Config
	Store     store.Store
	Memory    memory.Store
	Lease     *lease.Manager
	Engine    *runtime.Engine
	Index     *index.Client
	LLM       llm.Client
	Retention *retention.Cleaner
}

func NewServiceContext(c config.Config) (*ServiceContext, error) {
	if strings.TrimSpace(c.DataSource) == "" {
		return nil, fmt.Errorf("assistant-agent: DataSource is required")
	}
	if strings.TrimSpace(c.InternalSecret) == "" {
		return nil, fmt.Errorf("assistant-agent: InternalSecret is required")
	}
	newClient := authenticatedRPCClient(c.InternalSecret)

	searchService := searchservice.NewSearchService(newClient(c.SearchRpc))
	contentService := contentservice.NewContentService(newClient(c.ContentRpc))
	recommendService := recommendservice.NewRecommendService(newClient(c.RecommendRpc))
	mediaService := mediaservice.NewMediaService(newClient(c.MediaRpc))
	interactionService := interactionservice.NewInteractionService(newClient(c.InteractionRpc))
	userService := userservice.NewUserService(newClient(c.UserRpc))

	// SQL arguments contain prompts and tool payloads; suppress SQL logs (REL-022).
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
	if err := checkConfiguredRoutes(c.LLM, client, routeIDs, auxClient, reviewClient); err != nil {
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
		Interaction: interactionService, User: userService, Memory: mem, Store: st, History: historyTool,
		Web: websearch.New(websearch.Config{
			APIKey: c.WebSearch.APIKey, Endpoint: c.WebSearch.Endpoint,
			Timeout: time.Duration(c.WebSearch.TimeoutMs) * time.Millisecond, MaxResults: c.WebSearch.MaxResults,
		}),
	}, c.AllowedTools)
	if err != nil {
		return nil, err
	}
	engine := &runtime.Engine{
		Store: st, Memory: mem, Tools: registry, LLM: client, AuxLLM: auxClient, ReviewLLM: reviewClient, Notify: notify,
		Window: c.LLM.ContextWindowTokens, Provider: c.LLM.MaxOutputTokens,
	}
	return &ServiceContext{
		Config: c, Store: st, Memory: mem,
		Lease:  &lease.Manager{Store: st, Owner: lease.NewOwner(c.Name), Lease: time.Duration(c.LeaseSeconds) * time.Second, Renew: time.Duration(c.RenewSeconds) * time.Second},
		Engine: engine, Index: history, LLM: client,
		Retention: retention.New(st),
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

func checkConfiguredRoutes(c config.LLMConfig, client llm.Client, routeIDs []string, auxClient, reviewClient llm.Client) error {
	if c.Enabled && c.CanaryEnabled {
		canaryTimeout := minDuration(time.Duration(c.TimeoutMs)*time.Millisecond, 30*time.Second)
		for _, routeID := range routeIDs {
			routeClient, ok := llm.SelectExactRoute(client, routeID)
			if !ok {
				return fmt.Errorf("assistant-agent: LLM route %q cannot be selected", routeID)
			}
			if err := runLLMCanary(canaryTimeout, routeClient); err != nil {
				return fmt.Errorf("assistant-agent: LLM route %q readiness: %w", routeID, err)
			}
		}
		for _, auxiliaryRoute := range []struct {
			name   string
			client llm.Client
		}{{name: "aux", client: auxClient}, {name: "review", client: reviewClient}} {
			routeID, auxiliary := auxiliaryRoute.name, auxiliaryRoute.client
			if auxiliary != nil {
				if err := runLLMCanary(canaryTimeout, auxiliary); err != nil {
					return fmt.Errorf("assistant-agent: LLM %s readiness: %w", routeID, err)
				}
			}
		}
	}
	return nil
}

func authenticatedRPCClient(secret string) func(rpcx.RpcClientConf) rpcx.Client {

	internalAuthOption := rpcx.WithInternalAuth(secret)

	newClient := func(conf rpcx.RpcClientConf) rpcx.Client {
		return rpcx.MustNewClient(conf,

			internalAuthOption,
		)
	}
	return newClient
}
