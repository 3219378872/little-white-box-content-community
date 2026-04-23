// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package svc

import (
	"esx/app/content/contentservice"
	"esx/app/interaction/interactionservice"
	"esx/app/media/mediaservice"
	"gateway/internal/config"
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
	OptionalAuth       rest.Middleware
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

	optionalAuth := middleware.NewOptionalAuthMiddleware(jwtx.JwtConfig{
		AccessSecret: c.Auth.AccessSecret,
		AccessExpire: c.Auth.AccessExpire,
	})

	return &ServiceContext{
		Config:             c,
		UserService:        userService,
		ContentService:     contentService,
		MediaService:       mediaService,
		InteractionService: interactionService,
		OptionalAuth:       optionalAuth.Handle,
	}
}
