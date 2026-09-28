package svc

import (
	"fmt"
	"strings"

	"esx/app/assistant/mq/internal/config"
	"esx/app/assistant/watch"
	"esx/app/content/rpc/contentservice"

	"esx/pkg/rpcx"
	sqlx "esx/pkg/sqlstore"
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
	contentClient := rpcx.MustNewClient(c.ContentRpc,

		rpcx.WithInternalAuth(c.InternalSecret),
	)
	// Watch titles and summaries must not appear in normal, slow or failed SQL logs.
	return &ServiceContext{
		Config:  c,
		Watch:   watch.NewSQLStore(sqlx.NewMysql(c.DataSource)),
		Content: contentservice.NewContentService(contentClient),
	}, nil
}
