// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package svc

import (
	"gateway/internal/config"
	"user/userservice"

	"github.com/zeromicro/go-zero/zrpc"
)

type ServiceContext struct {
	Config      config.Config
	UserService userservice.UserService
}

func NewServiceContext(c config.Config) *ServiceContext {
	client := zrpc.MustNewClient(c.UserRpc)
	service := userservice.NewUserService(client)

	return &ServiceContext{
		Config:      c,
		UserService: service,
	}
}
