// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package svc

import (
	"esx/app/assistant/rpc/assistantservice"
	"esx/app/behavior/rpc/behaviorservice"
	"esx/app/content/rpc/contentservice"
	"esx/app/feed/rpc/feedservice"
	"esx/app/interaction/rpc/interactionservice"
	"esx/app/media/rpc/mediaservice"
	"esx/app/message/rpc/messageservice"
	"esx/app/search/rpc/searchservice"
	"gateway/internal/config"
	gatewaymiddleware "gateway/internal/middleware"
	"interceptor"
	"jwtx"
	"middleware"
	"user/userservice"

	"github.com/zeromicro/go-zero/rest"
	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc/connectivity"
)

// Dependency 描述一个就绪检查依赖（REL-053）。
type Dependency struct {
	Name      string
	ConnState func() connectivity.State
	Optional  bool // 可选能力（如发现）故障只降级，不使整个 Gateway 下线
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
	OptionalAuth       rest.Middleware
	BehaviorAccepted   rest.Middleware
}

func NewServiceContext(c config.Config) *ServiceContext {
	bizErrInterceptor := interceptor.BizErrorUnaryInterceptor()

	userClient := zrpc.MustNewClient(c.UserRpc, zrpc.WithUnaryClientInterceptor(bizErrInterceptor))
	userService := userservice.NewUserService(userClient)
	contentClient := zrpc.MustNewClient(c.ContentRpc, zrpc.WithUnaryClientInterceptor(bizErrInterceptor))
	contentService := contentservice.NewContentService(contentClient)
	mediaClient := zrpc.MustNewClient(c.MediaRpc, zrpc.WithUnaryClientInterceptor(bizErrInterceptor))
	mediaService := mediaservice.NewMediaService(mediaClient)
	interactionClient := zrpc.MustNewClient(c.InteractionRpc, zrpc.WithUnaryClientInterceptor(bizErrInterceptor))
	interactionService := interactionservice.NewInteractionService(interactionClient)
	behaviorClient := zrpc.MustNewClient(c.BehaviorRpc, zrpc.WithUnaryClientInterceptor(bizErrInterceptor))
	behaviorService := behaviorservice.NewBehaviorService(behaviorClient)
	feedClient := zrpc.MustNewClient(c.FeedRpc, zrpc.WithUnaryClientInterceptor(bizErrInterceptor))
	feedService := feedservice.NewFeedService(feedClient)
	messageClient := zrpc.MustNewClient(c.MessageRpc, zrpc.WithUnaryClientInterceptor(bizErrInterceptor))
	messageService := messageservice.NewMessageService(messageClient)
	searchClient := zrpc.MustNewClient(c.SearchRpc, zrpc.WithUnaryClientInterceptor(bizErrInterceptor))
	searchService := searchservice.NewSearchService(searchClient)
	assistantClient := zrpc.MustNewClient(c.AssistantRpc, zrpc.WithUnaryClientInterceptor(bizErrInterceptor))
	assistantService := assistantservice.NewAssistantService(assistantClient)

	optionalAuth := middleware.NewOptionalAuthMiddleware(jwtx.JwtConfig{
		AccessSecret: c.Auth.AccessSecret,
		AccessExpire: c.Auth.AccessExpire,
	})
	behaviorAccepted := gatewaymiddleware.NewBehaviorAcceptedMiddleware()

	return &ServiceContext{
		Config: c,
		Dependencies: []Dependency{
			{Name: "user", ConnState: func() connectivity.State { return userClient.Conn().GetState() }},
			{Name: "content", ConnState: func() connectivity.State { return contentClient.Conn().GetState() }},
			{Name: "media", ConnState: func() connectivity.State { return mediaClient.Conn().GetState() }},
			{Name: "interaction", ConnState: func() connectivity.State { return interactionClient.Conn().GetState() }},
			{Name: "behavior", ConnState: func() connectivity.State { return behaviorClient.Conn().GetState() }},
			{Name: "feed", ConnState: func() connectivity.State { return feedClient.Conn().GetState() }},
			{Name: "message", ConnState: func() connectivity.State { return messageClient.Conn().GetState() }},
			{Name: "search", ConnState: func() connectivity.State { return searchClient.Conn().GetState() }, Optional: true},
			{Name: "assistant", ConnState: func() connectivity.State { return assistantClient.Conn().GetState() }, Optional: true},
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
		OptionalAuth:       optionalAuth.Handle,
		BehaviorAccepted:   behaviorAccepted.Handle,
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
func (s *ServiceContext) Readiness() (string, []DependencyStatus) {
	statuses := make([]DependencyStatus, 0, len(s.Dependencies))
	requiredDown := false
	optionalDown := false
	for _, dependency := range s.Dependencies {
		healthy := dependencyHealthy(dependency)
		status := "ok"
		if !healthy {
			status = "down"
			if dependency.Optional {
				optionalDown = true
			} else {
				requiredDown = true
			}
		}
		statuses = append(statuses, DependencyStatus{Name: dependency.Name, Status: status, Healthy: healthy})
	}
	switch {
	case requiredDown:
		return "unavailable", statuses
	case optionalDown:
		return "degraded", statuses
	default:
		return "ready", statuses
	}
}

func dependencyHealthy(dependency Dependency) bool {
	if dependency.ConnState == nil {
		return false
	}
	state := dependency.ConnState()
	switch state {
	case connectivity.Ready, connectivity.Idle, connectivity.Connecting:
		return true
	default:
		return false
	}
}
