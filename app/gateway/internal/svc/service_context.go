// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package svc

import (
	"esx/app/content/contentservice"
	"esx/app/media/mediaservice"
	"gateway/internal/config"
	"user/userservice"

	"github.com/zeromicro/go-zero/zrpc"
)

type ServiceContext struct {
	Config         config.Config
	UserService    userservice.UserService
	ContentService contentservice.ContentService
	MediaService   mediaservice.MediaService
}

func NewServiceContext(c config.Config) *ServiceContext {
	userClient := zrpc.MustNewClient(c.UserRpc)
	userService := userservice.NewUserService(userClient)
	contentClient := zrpc.MustNewClient(c.ContentRpc)
	contentService := contentservice.NewContentService(contentClient)
	mediaClient := zrpc.MustNewClient(c.MediaRpc)
	mediaService := mediaservice.NewMediaService(mediaClient)

	return &ServiceContext{
		Config:         c,
		UserService:    userService,
		ContentService: contentService,
		MediaService:   mediaService,
	}
}
