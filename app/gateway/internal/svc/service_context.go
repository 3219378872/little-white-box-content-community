package svc

import (
	"esx/app/ad/rpc/adservice"
	"esx/app/assistant/rpc/assistantservice"
	"esx/app/behavior/rpc/behaviorservice"
	"esx/app/content/rpc/contentservice"
	"esx/app/feed/rpc/feedservice"
	"esx/app/gateway/internal/config"
	gatewaymiddleware "esx/app/gateway/internal/middleware"
	"esx/app/interaction/rpc/interactionservice"
	"esx/app/media/rpc/mediaservice"
	"esx/app/message/rpc/messageservice"
	"esx/app/review/rpc/reviewservice"
	"esx/app/search/rpc/searchservice"
	"esx/app/user/rpc/userservice"
	"esx/pkg/jwtx"
	"esx/pkg/middleware"

	"context"
	"esx/pkg/rpcx"
	"sync"

	"github.com/cloudwego/hertz/pkg/app"
)

// Dependency 描述一个就绪检查依赖（REL-053）。
type Dependency struct {
	Name     string
	Probe    func(context.Context) error
	Optional bool // 可选能力（如发现）故障只降级，不使整个 Gateway 下线
}

type ServiceContext struct {
	Config             config.Config
	Dependencies       []Dependency
	UserService        userservice.UserService
	ContentService     contentservice.ContentService
	MediaService       mediaservice.MediaService
	InteractionService interactionservice.InteractionService
	BehaviorService    behaviorservice.BehaviorService
	FeedService        feedservice.FeedService
	MessageService     messageservice.MessageService
	SearchService      searchservice.SearchService
	AssistantService   assistantservice.AssistantService
	AdService          adservice.AdService
	ReviewService      reviewservice.ReviewService
	OptionalAuth       app.HandlerFunc
	RequiredAuth       app.HandlerFunc
	BehaviorAccepted   app.HandlerFunc
}

func NewServiceContext(c config.Config) *ServiceContext {

	internalAuthOption := rpcx.WithInternalAuth(c.InternalSecret)

	withInternalAuth := func(opts ...rpcx.ClientOption) []rpcx.ClientOption {
		return append([]rpcx.ClientOption{

			internalAuthOption,
		}, opts...)
	}
	newClient := func(conf rpcx.RpcClientConf) rpcx.Client {
		return rpcx.MustNewClient(conf, withInternalAuth()...)
	}

	userClient := newClient(c.UserRpc)
	userService := userservice.NewUserService(userClient)
	contentClient := newClient(c.ContentRpc)
	contentService := contentservice.NewContentService(contentClient)
	mediaClient := newClient(c.MediaRpc)
	mediaService := mediaservice.NewMediaService(mediaClient)
	interactionClient := newClient(c.InteractionRpc)
	interactionService := interactionservice.NewInteractionService(interactionClient)
	behaviorClient := newClient(c.BehaviorRpc)
	behaviorService := behaviorservice.NewBehaviorService(behaviorClient)
	feedClient := newClient(c.FeedRpc)
	feedService := feedservice.NewFeedService(feedClient)
	messageClient := newClient(c.MessageRpc)
	messageService := messageservice.NewMessageService(messageClient)
	searchClient := newClient(c.SearchRpc)
	searchService := searchservice.NewSearchService(searchClient)
	assistantClient := newClient(c.AssistantRpc)
	assistantService := assistantservice.NewAssistantService(assistantClient)
	adClient := newClient(c.AdRpc)
	adService := adservice.NewAdService(adClient)
	reviewClient := newClient(c.ReviewRpc)
	reviewService := reviewservice.NewReviewService(reviewClient)

	optionalAuth := middleware.NewOptionalAuthMiddleware(jwtx.JwtConfig{
		AccessSecret: c.Auth.AccessSecret,
		AccessExpire: c.Auth.AccessExpire,
	})
	requiredAuth := middleware.NewRequiredAuthMiddleware(jwtx.JwtConfig{
		AccessSecret: c.Auth.AccessSecret,
		AccessExpire: c.Auth.AccessExpire,
	})
	behaviorAccepted := gatewaymiddleware.NewBehaviorAcceptedMiddleware()

	return &ServiceContext{
		Config: c,
		Dependencies: []Dependency{
			{Name: "user", Probe: userClient.Probe},
			{Name: "content", Probe: contentClient.Probe},
			{Name: "media", Probe: mediaClient.Probe},
			{Name: "interaction", Probe: interactionClient.Probe},
			{Name: "behavior", Probe: behaviorClient.Probe},
			{Name: "feed", Probe: feedClient.Probe},
			{Name: "message", Probe: messageClient.Probe},
			{Name: "search", Probe: searchClient.Probe, Optional: true},
			{Name: "assistant", Probe: assistantClient.Probe, Optional: true},
			// 广告与审核是可选能力：故障只降级，推荐流照常返回不含广告（ADS-022）。
			{Name: "ad", Probe: adClient.Probe, Optional: true},
			{Name: "review", Probe: reviewClient.Probe, Optional: true},
		},
		UserService:        userService,
		ContentService:     contentService,
		MediaService:       mediaService,
		InteractionService: interactionService,
		BehaviorService:    behaviorService,
		FeedService:        feedService,
		MessageService:     messageService,
		SearchService:      searchService,
		AssistantService:   assistantService,
		AdService:          adService,
		ReviewService:      reviewService,
		OptionalAuth:       optionalAuth.Hertz,
		RequiredAuth:       requiredAuth.Hertz,
		BehaviorAccepted:   behaviorAccepted.Hertz,
	}
}

// DependencyStatus 是一次就绪检查的结果。
type DependencyStatus struct {
	Name    string `json:"name"`
	Status  string `json:"status"` // ok | down
	Healthy bool
}

// Readiness 检查所有依赖的连接状态。可选能力故障只标记 down；
// 必需能力故障返回 unavailable。整体状态 ready/degraded/unavailable。
func (s *ServiceContext) Readiness(ctx context.Context) (string, []DependencyStatus) {
	statuses := make([]DependencyStatus, len(s.Dependencies))
	var wg sync.WaitGroup
	for i, d := range s.Dependencies {
		wg.Add(1)
		go func(i int, d Dependency) {
			defer wg.Done()
			healthy := d.Probe != nil && d.Probe(ctx) == nil
			state := "down"
			if healthy {
				state = "ok"
			}
			statuses[i] = DependencyStatus{Name: d.Name, Status: state, Healthy: healthy}
		}(i, d)
	}
	wg.Wait()
	requiredDown, optionalDown := false, false
	for i, status := range statuses {
		if !status.Healthy {
			if s.Dependencies[i].Optional {
				optionalDown = true
			} else {
				requiredDown = true
			}
		}
	}
	if requiredDown {
		return "unavailable", statuses
	}
	if optionalDown {
		return "degraded", statuses
	}
	return "ready", statuses
}
