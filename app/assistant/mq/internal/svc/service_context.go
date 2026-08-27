package svc

import (
	"fmt"
	"strings"

	"esx/app/assistant/mq/internal/config"
	"esx/app/assistant/watch"
	"esx/app/content/rpc/contentservice"
	"esx/pkg/interceptor"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
	"github.com/zeromicro/go-zero/zrpc"
)

type ServiceContext struct {
	Config  config.Config
	Watch   watch.Store
	Content contentservice.ContentService
}

func NewServiceContext(c config.Config) (*ServiceContext, error) {
	if strings.TrimSpace(c.DataSource) == "" {
		return nil, fmt.Errorf("assistant-watch-matcher: DataSource is required")
	}
	if strings.TrimSpace(c.InternalSecret) == "" {
		return nil, fmt.Errorf("assistant-watch-matcher: InternalSecret is required")
	}
	contentConf := c.ContentRpc
	contentConf.Middlewares.Duration = false
	contentClient := zrpc.MustNewClient(contentConf,
		zrpc.WithUnaryClientInterceptor(interceptor.BizErrorUnaryInterceptor()),
		zrpc.WithUnaryClientInterceptor(interceptor.InternalAuthUnaryClientInterceptor(c.InternalSecret)),
		zrpc.WithUnaryClientInterceptor(interceptor.SafeDurationUnaryClientInterceptor()),
	)
	return &ServiceContext{
		Config:  c,
		Watch:   watch.NewSQLStore(sqlx.NewMysql(c.DataSource)),
		Content: contentservice.NewContentService(contentClient),
	}, nil
}
