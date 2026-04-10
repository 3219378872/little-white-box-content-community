// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package svc

import (
	"esx/app/content/contentservice"
	"gateway/internal/config"
	"user/userservice"

	"github.com/zeromicro/go-zero/zrpc"
)

type ServiceContext struct {
	Config         config.Config
	UserService    userservice.UserService
	ContentService contentservice.ContentService
}

func NewServiceContext(c config.Config) *ServiceContext {
	userClient := zrpc.MustNewClient(c.UserRpc)
	userService := userservice.NewUserService(userClient)
	contentClient := zrpc.MustNewClient(c.ContentRpc)
	contentService := contentservice.NewContentService(contentClient)

	return &ServiceContext{
		Config:         c,
		UserService:    userService,
		ContentService: contentService,
	}
}
