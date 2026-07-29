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
)

type ServiceContext struct {
	Config             config.Config
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
		Config:             c,
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
